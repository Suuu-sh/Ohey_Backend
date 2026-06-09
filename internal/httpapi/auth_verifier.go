package httpapi

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	authTokenCacheTTL         = 2 * time.Minute
	authServerTokenCacheTTL   = 30 * time.Second
	authJWKSCacheTTL          = 10 * time.Minute
	authJWTClockSkew          = 30 * time.Second
	maxAuthTokenCacheEntries  = 4096
	maxAuthTokenLengthForAuth = 64 * 1024
)

var (
	errInvalidAuthToken = errors.New("invalid auth token")
	errJWKKeyNotFound   = errors.New("jwk key not found")
)

type authVerifier struct {
	httpClient *http.Client
	issuer     string
	jwksURL    string
	audience   string
	now        func() time.Time

	mu         sync.Mutex
	tokenCache map[[32]byte]cachedAuthUser
	jwksCache  cachedAuthJWKS
}

type cachedAuthUser struct {
	user      AuthUser
	expiresAt time.Time
}

type cachedAuthJWKS struct {
	keys      []jwk
	expiresAt time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	KID string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Iss          string         `json:"iss"`
	Sub          string         `json:"sub"`
	Aud          any            `json:"aud"`
	Exp          int64          `json:"exp"`
	Nbf          int64          `json:"nbf"`
	Iat          int64          `json:"iat"`
	Role         string         `json:"role"`
	Email        string         `json:"email"`
	AppMetadata  map[string]any `json:"app_metadata"`
	UserMetadata map[string]any `json:"user_metadata"`
}

type parsedJWT struct {
	header       jwtHeader
	claims       jwtClaims
	signingInput []byte
	signature    []byte
	expiresAt    time.Time
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty    string   `json:"kty"`
	KID    string   `json:"kid"`
	Alg    string   `json:"alg"`
	Use    string   `json:"use"`
	KeyOps []string `json:"key_ops"`
	Crv    string   `json:"crv"`
	N      string   `json:"n"`
	E      string   `json:"e"`
	X      string   `json:"x"`
	Y      string   `json:"y"`
}

func newClerkAuthVerifier(issuer, jwksURL, audience string, httpClient *http.Client, now func() time.Time) *authVerifier {
	return newAuthVerifier(authVerifierConfig{
		httpClient: httpClient,
		issuer:     strings.TrimRight(issuer, "/"),
		jwksURL:    strings.TrimSpace(jwksURL),
		audience:   strings.TrimSpace(audience),
		now:        now,
	})
}

type authVerifierConfig struct {
	httpClient *http.Client
	issuer     string
	jwksURL    string
	audience   string
	now        func() time.Time
}

func newAuthVerifier(cfg authVerifierConfig) *authVerifier {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.httpClient == nil {
		cfg.httpClient = http.DefaultClient
	}
	return &authVerifier{
		httpClient: cfg.httpClient,
		issuer:     strings.TrimRight(cfg.issuer, "/"),
		jwksURL:    strings.TrimSpace(cfg.jwksURL),
		audience:   strings.TrimSpace(cfg.audience),
		now:        cfg.now,
		tokenCache: make(map[[32]byte]cachedAuthUser),
	}
}

func bearerTokenFromRequest(req *http.Request) (string, bool) {
	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return token, token != ""
}

func (r *router) verifyAuthToken(ctx context.Context, token string) (AuthUser, error) {
	if r.authVerifier == nil {
		return AuthUser{}, errors.New("auth verifier is not configured")
	}
	return r.authVerifier.Verify(ctx, token)
}

func writeAuthVerificationError(w http.ResponseWriter, err error) {
	if errors.Is(err, errInvalidAuthToken) {
		writeError(w, http.StatusUnauthorized, "authentication failed")
		return
	}
	writeUpstreamError(w, err)
}

func (v *authVerifier) Verify(ctx context.Context, token string) (AuthUser, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxAuthTokenLengthForAuth {
		return AuthUser{}, errInvalidAuthToken
	}
	if user, ok := v.cachedToken(token); ok {
		return user, nil
	}

	parsed, parseErr := parseJWT(token)
	if parseErr == nil {
		if err := parsed.validate(v.issuer, v.audience, v.now()); err != nil {
			return AuthUser{}, err
		}
		if isAsymmetricAlg(parsed.header.Alg) {
			if err := v.verifyAsymmetric(ctx, parsed); err == nil {
				user := parsed.authUser()
				v.cacheToken(token, user, parsed.expiresAt, authTokenCacheTTL)
				return user, nil
			} else if !errors.Is(err, errJWKKeyNotFound) {
				return AuthUser{}, err
			}
		}
		return AuthUser{}, errInvalidAuthToken
	}

	return AuthUser{}, errInvalidAuthToken
}

func (v *authVerifier) cachedToken(token string) (AuthUser, bool) {
	key := sha256.Sum256([]byte(token))
	now := v.now()

	v.mu.Lock()
	defer v.mu.Unlock()
	cached, ok := v.tokenCache[key]
	if !ok {
		return AuthUser{}, false
	}
	if !cached.expiresAt.After(now) {
		delete(v.tokenCache, key)
		return AuthUser{}, false
	}
	return cached.user, true
}

func (v *authVerifier) cacheToken(token string, user AuthUser, tokenExpiresAt time.Time, maxTTL time.Duration) {
	if strings.TrimSpace(user.ID) == "" || maxTTL <= 0 {
		return
	}
	now := v.now()
	expiresAt := now.Add(maxTTL)
	if !tokenExpiresAt.IsZero() && tokenExpiresAt.Before(expiresAt) {
		expiresAt = tokenExpiresAt
	}
	if !expiresAt.After(now) {
		return
	}

	key := sha256.Sum256([]byte(token))
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.tokenCache) >= maxAuthTokenCacheEntries {
		for cacheKey, cached := range v.tokenCache {
			if !cached.expiresAt.After(now) {
				delete(v.tokenCache, cacheKey)
			}
		}
		if len(v.tokenCache) >= maxAuthTokenCacheEntries {
			v.tokenCache = make(map[[32]byte]cachedAuthUser)
		}
	}
	v.tokenCache[key] = cachedAuthUser{user: user, expiresAt: expiresAt}
}

func parseJWT(token string) (parsedJWT, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return parsedJWT{}, errInvalidAuthToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return parsedJWT{}, errInvalidAuthToken
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return parsedJWT{}, errInvalidAuthToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return parsedJWT{}, errInvalidAuthToken
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return parsedJWT{}, errInvalidAuthToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return parsedJWT{}, errInvalidAuthToken
	}

	return parsedJWT{
		header:       header,
		claims:       claims,
		signingInput: []byte(parts[0] + "." + parts[1]),
		signature:    signature,
		expiresAt:    time.Unix(claims.Exp, 0),
	}, nil
}

func (p parsedJWT) validate(issuer, audience string, now time.Time) error {
	if strings.TrimSpace(p.header.Alg) == "" || strings.EqualFold(p.header.Alg, "none") {
		return errInvalidAuthToken
	}
	if p.claims.Iss != issuer {
		return errInvalidAuthToken
	}
	if strings.TrimSpace(p.claims.Sub) == "" {
		return errInvalidAuthToken
	}
	if p.claims.Exp == 0 {
		return errInvalidAuthToken
	}
	if !now.Before(time.Unix(p.claims.Exp, 0).Add(authJWTClockSkew)) {
		return errInvalidAuthToken
	}
	if p.claims.Nbf != 0 && now.Add(authJWTClockSkew).Before(time.Unix(p.claims.Nbf, 0)) {
		return errInvalidAuthToken
	}
	if strings.TrimSpace(audience) != "" && !jwtAudienceContains(p.claims.Aud, audience) {
		return errInvalidAuthToken
	}
	return nil
}

func jwtAudienceContains(value any, expected string) bool {
	expected = strings.TrimSpace(expected)
	switch aud := value.(type) {
	case string:
		return aud == expected
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	}
	return false
}

func (p parsedJWT) authUser() AuthUser {
	return AuthUser{
		ID:           strings.TrimSpace(p.claims.Sub),
		Email:        strings.TrimSpace(p.claims.Email),
		AppMetadata:  p.claims.AppMetadata,
		UserMetadata: p.claims.UserMetadata,
	}
}

func isAsymmetricAlg(alg string) bool {
	switch alg {
	case "RS256", "ES256", "EdDSA":
		return true
	default:
		return false
	}
}

func (v *authVerifier) verifyAsymmetric(ctx context.Context, parsed parsedJWT) error {
	keys, err := v.cachedJWKs(ctx, false)
	if err != nil {
		return errJWKKeyNotFound
	}
	if err := verifyJWTSignature(parsed, keys); err == nil {
		return nil
	} else if !errors.Is(err, errJWKKeyNotFound) {
		return err
	}

	keys, err = v.cachedJWKs(ctx, true)
	if err != nil {
		return errJWKKeyNotFound
	}
	return verifyJWTSignature(parsed, keys)
}

func (v *authVerifier) cachedJWKs(ctx context.Context, forceRefresh bool) ([]jwk, error) {
	now := v.now()
	v.mu.Lock()
	defer v.mu.Unlock()
	if !forceRefresh && v.jwksCache.expiresAt.After(now) {
		return v.jwksCache.keys, nil
	}
	var set jwks
	if v.jwksURL != "" {
		if err := v.getJWKSURL(ctx, &set); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("jwks url is not configured")
	}
	v.jwksCache = cachedAuthJWKS{
		keys:      set.Keys,
		expiresAt: now.Add(authJWKSCacheTTL),
	}
	return set.Keys, nil
}

func (v *authVerifier) getJWKSURL(ctx context.Context, out any) error {
	parsed, err := url.Parse(v.jwksURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errInvalidAuthToken
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errInvalidAuthToken
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func verifyJWTSignature(parsed parsedJWT, keys []jwk) error {
	var found bool
	for _, key := range keys {
		if strings.TrimSpace(parsed.header.KID) != "" && key.KID != parsed.header.KID {
			continue
		}
		if strings.TrimSpace(key.Alg) != "" && key.Alg != parsed.header.Alg {
			continue
		}
		found = true
		if err := verifyJWTSignatureWithKey(parsed, key); err != nil {
			return err
		}
		return nil
	}
	if !found {
		return errJWKKeyNotFound
	}
	return errInvalidAuthToken
}

func verifyJWTSignatureWithKey(parsed parsedJWT, key jwk) error {
	switch parsed.header.Alg {
	case "RS256":
		publicKey, err := rsaPublicKeyFromJWK(key)
		if err != nil {
			return errInvalidAuthToken
		}
		digest := sha256.Sum256(parsed.signingInput)
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], parsed.signature); err != nil {
			return errInvalidAuthToken
		}
		return nil
	case "ES256":
		publicKey, err := ecdsaPublicKeyFromJWK(key)
		if err != nil {
			return errInvalidAuthToken
		}
		if len(parsed.signature) != 64 {
			return errInvalidAuthToken
		}
		digest := sha256.Sum256(parsed.signingInput)
		r := new(big.Int).SetBytes(parsed.signature[:32])
		s := new(big.Int).SetBytes(parsed.signature[32:])
		if !ecdsa.Verify(publicKey, digest[:], r, s) {
			return errInvalidAuthToken
		}
		return nil
	case "EdDSA":
		publicKey, err := ed25519PublicKeyFromJWK(key)
		if err != nil {
			return errInvalidAuthToken
		}
		if !ed25519.Verify(publicKey, parsed.signingInput, parsed.signature) {
			return errInvalidAuthToken
		}
		return nil
	default:
		return errInvalidAuthToken
	}
}

func rsaPublicKeyFromJWK(key jwk) (*rsa.PublicKey, error) {
	if key.Kty != "RSA" {
		return nil, errInvalidAuthToken
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}
	if e == 0 {
		return nil, errInvalidAuthToken
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func ecdsaPublicKeyFromJWK(key jwk) (*ecdsa.PublicKey, error) {
	if key.Kty != "EC" || key.Crv != "P-256" {
		return nil, errInvalidAuthToken
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(key.Y)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

func ed25519PublicKeyFromJWK(key jwk) (ed25519.PublicKey, error) {
	if key.Kty != "OKP" || key.Crv != "Ed25519" {
		return nil, errInvalidAuthToken
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		return nil, err
	}
	if len(xBytes) != ed25519.PublicKeySize {
		return nil, errInvalidAuthToken
	}
	return ed25519.PublicKey(xBytes), nil
}
