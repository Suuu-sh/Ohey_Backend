package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type SupabaseStorageRepository struct {
	supabaseURL    string
	serviceRoleKey string
	httpClient     *http.Client
}

func NewSupabaseStorageRepository(supabaseURL, serviceRoleKey string, httpClient *http.Client) *SupabaseStorageRepository {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &SupabaseStorageRepository{
		supabaseURL:    strings.TrimRight(strings.TrimSpace(supabaseURL), "/"),
		serviceRoleKey: strings.TrimSpace(serviceRoleKey),
		httpClient:     httpClient,
	}
}

func (r *SupabaseStorageRepository) CreateSignedUploadURL(ctx context.Context, target UploadTarget) (UploadURL, error) {
	if r.supabaseURL == "" || r.serviceRoleKey == "" {
		return UploadURL{}, errors.New("supabase storage is not configured")
	}
	endpoint := r.supabaseURL + "/storage/v1/object/upload/sign/" + escapedStoragePath(target.Bucket, target.Path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return UploadURL{}, err
	}
	req.Header.Set("Authorization", "Bearer "+r.serviceRoleKey)
	req.Header.Set("apikey", r.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return UploadURL{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return UploadURL{}, fmt.Errorf("supabase storage signed upload url failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var out struct {
		URL       string `json:"url"`
		SignedURL string `json:"signedURL"`
		SignedUrl string `json:"signedUrl"`
		Token     string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return UploadURL{}, err
	}
	signedURL := strings.TrimSpace(out.URL)
	if signedURL == "" {
		signedURL = strings.TrimSpace(out.SignedURL)
	}
	if signedURL == "" {
		signedURL = strings.TrimSpace(out.SignedUrl)
	}
	if signedURL == "" {
		return UploadURL{}, UserError{Kind: ErrorKindUpstream, Message: "signed upload url response missing url"}
	}
	if strings.HasPrefix(signedURL, "/") {
		signedURL = r.supabaseURL + "/storage/v1" + signedURL
	}
	token := strings.TrimSpace(out.Token)
	if token == "" {
		if parsed, err := url.Parse(signedURL); err == nil {
			token = parsed.Query().Get("token")
		}
	}
	if token == "" {
		return UploadURL{}, UserError{Kind: ErrorKindUpstream, Message: "signed upload url response missing token"}
	}
	return UploadURL{Bucket: target.Bucket, Path: target.Path, Token: token, SignedURL: signedURL, ContentType: target.ContentType}, nil
}

func escapedStoragePath(bucket, objectPath string) string {
	parts := append([]string{bucket}, strings.Split(objectPath, "/")...)
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
