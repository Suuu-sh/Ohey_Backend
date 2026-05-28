package httpapi

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type fcmServiceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	ProjectID   string `json:"project_id"`
	TokenURI    string `json:"token_uri"`
}

type fcmSender struct {
	account fcmServiceAccount
	http    *http.Client
	mu      sync.Mutex
	token   string
	expires time.Time
}

type fcmSendError struct {
	status       int
	body         string
	invalidToken bool
}

func (e fcmSendError) Error() string {
	return fmt.Sprintf("fcm send failed: status=%d body=%s", e.status, e.body)
}

func (e fcmSendError) InvalidPushToken() bool {
	return e.invalidToken
}

func newFCMSender(raw string, httpClient *http.Client) (*fcmSender, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var account fcmServiceAccount
	if err := json.Unmarshal([]byte(raw), &account); err != nil {
		decoded, decErr := base64.StdEncoding.DecodeString(raw)
		if decErr != nil {
			return nil, err
		}
		if err := json.Unmarshal(decoded, &account); err != nil {
			return nil, err
		}
	}
	if account.ClientEmail == "" || account.PrivateKey == "" || account.ProjectID == "" {
		return nil, errors.New("FCM service account must include client_email, private_key, and project_id")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &fcmSender{account: account, http: httpClient}, nil
}

func (s *fcmSender) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	accessToken, err := s.accessToken(ctx)
	if err != nil {
		return err
	}
	messageData := map[string]string{}
	for k, v := range data {
		if v != "" {
			messageData[k] = v
		}
	}
	payload := map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": title, "body": body},
		"data":         messageData,
		"apns":         map[string]any{"payload": map[string]any{"aps": map[string]any{"sound": "default"}}},
	}}
	encoded, _ := json.Marshal(payload)
	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", url.PathEscape(s.account.ProjectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := string(respBody)
		return fcmSendError{status: resp.StatusCode, body: body, invalidToken: isInvalidFCMPushTokenResponse(resp.StatusCode, body)}
	}
	return nil
}

func isInvalidFCMPushTokenResponse(status int, body string) bool {
	normalized := strings.ToUpper(body)
	if strings.Contains(normalized, "UNREGISTERED") ||
		strings.Contains(normalized, "SENDER_ID_MISMATCH") {
		return true
	}
	return status == http.StatusBadRequest &&
		strings.Contains(normalized, "INVALID_ARGUMENT") &&
		strings.Contains(normalized, "TOKEN")
}

func (s *fcmSender) accessToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.token != "" && time.Now().Before(s.expires.Add(-time.Minute)) {
		token := s.token
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	assertion, err := s.jwtAssertion()
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fcm token failed: status=%d body=%s", resp.StatusCode, string(data))
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", errors.New("fcm token response missing access_token")
	}
	s.mu.Lock()
	s.token = out.AccessToken
	s.expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	s.mu.Unlock()
	return out.AccessToken, nil
}

func (s *fcmSender) jwtAssertion() (string, error) {
	now := time.Now().Unix()
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iss":   s.account.ClientEmail,
		"scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud":   s.account.TokenURI,
		"iat":   now,
		"exp":   now + 3600,
	}
	unsigned := b64JSON(header) + "." + b64JSON(claims)
	block, _ := pem.Decode([]byte(s.account.PrivateKey))
	if block == nil {
		return "", errors.New("invalid fcm private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	privateKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", errors.New("fcm private key is not RSA")
	}
	digest := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func b64JSON(value any) string {
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}

func NewFCMSender(raw string, httpClient *http.Client) (*fcmSender, error) {
	return newFCMSender(raw, httpClient)
}
