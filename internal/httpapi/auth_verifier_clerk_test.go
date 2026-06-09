package httpapi

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClerkAuthVerifierVerifiesJWKSJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-key"
	issuer := "https://ohey-test.clerk.accounts.dev"
	now := time.Unix(1_800_000_000, 0)
	set := jwks{Keys: []jwk{rsaJWK(kid, &key.PublicKey)}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(set)
	}))
	defer server.Close()

	token := signRS256JWT(t, key, kid, jwtClaims{
		Iss:   issuer,
		Sub:   "user_2abc",
		Aud:   []any{"ohey-mobile"},
		Exp:   now.Add(time.Hour).Unix(),
		Iat:   now.Unix(),
		Email: "user@example.com",
	})
	verifier := newClerkAuthVerifier(issuer, server.URL+"/.well-known/jwks.json", "ohey-mobile", server.Client(), func() time.Time { return now })

	user, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if user.ID != "user_2abc" || user.Email != "user@example.com" {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestClerkAuthVerifierRejectsWrongAudience(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := "https://ohey-test.clerk.accounts.dev"
	now := time.Unix(1_800_000_000, 0)
	token := signRS256JWT(t, key, "test-key", jwtClaims{
		Iss: issuer,
		Sub: "user_2abc",
		Aud: "other-audience",
		Exp: now.Add(time.Hour).Unix(),
	})
	verifier := newClerkAuthVerifier(issuer, "https://example.com/.well-known/jwks.json", "ohey-mobile", nil, func() time.Time { return now })

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify() succeeded, want audience error")
	}
}

func rsaJWK(kid string, key *rsa.PublicKey) jwk {
	return jwk{
		Kty: "RSA",
		KID: kid,
		Alg: "RS256",
		Use: "sig",
		N:   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func signRS256JWT(t *testing.T, key *rsa.PrivateKey, kid string, claims jwtClaims) string {
	t.Helper()
	headerBytes, err := json.Marshal(jwtHeader{Alg: "RS256", KID: kid, Typ: "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(claimsBytes)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}
