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

func (r *SupabaseStorageRepository) DeleteObject(ctx context.Context, bucket, objectPath string) error {
	if r.supabaseURL == "" || r.serviceRoleKey == "" {
		return errors.New("supabase storage is not configured")
	}
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(objectPath) == "" {
		return nil
	}
	endpoint := r.supabaseURL + "/storage/v1/object/" + escapedStoragePath(bucket, objectPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.serviceRoleKey)
	req.Header.Set("apikey", r.serviceRoleKey)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supabase storage object delete failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (r *SupabaseStorageRepository) CreateSignedDisplayURL(ctx context.Context, bucket, objectPath string, expiresInSeconds int) (string, error) {
	if r.supabaseURL == "" || r.serviceRoleKey == "" {
		return "", errors.New("supabase storage is not configured")
	}
	if expiresInSeconds <= 0 {
		expiresInSeconds = DisplayURLTTLSeconds
	}
	endpoint := r.supabaseURL + "/storage/v1/object/sign/" + escapedStoragePath(bucket, objectPath)
	body, err := json.Marshal(map[string]any{"expiresIn": expiresInSeconds})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.serviceRoleKey)
	req.Header.Set("apikey", r.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("supabase storage signed display url failed: status=%d body=%s", resp.StatusCode, string(data))
	}
	var out struct {
		SignedURL string `json:"signedURL"`
		SignedUrl string `json:"signedUrl"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	signedURL := strings.TrimSpace(out.SignedURL)
	if signedURL == "" {
		signedURL = strings.TrimSpace(out.SignedUrl)
	}
	if signedURL == "" {
		signedURL = strings.TrimSpace(out.URL)
	}
	if signedURL == "" {
		return "", UserError{Kind: ErrorKindUpstream, Message: "signed display url response missing url"}
	}
	if strings.HasPrefix(signedURL, "/") {
		signedURL = r.supabaseURL + "/storage/v1" + signedURL
	}
	return signedURL, nil
}

func (r *SupabaseStorageRepository) ListObjects(ctx context.Context, bucket, prefix string, limit int) ([]StorageObject, error) {
	if r.supabaseURL == "" || r.serviceRoleKey == "" {
		return nil, errors.New("supabase storage is not configured")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	endpoint := r.supabaseURL + "/storage/v1/object/list/" + url.PathEscape(bucket)
	body, err := json.Marshal(map[string]any{"prefix": prefix, "limit": limit})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.serviceRoleKey)
	req.Header.Set("apikey", r.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("supabase storage object list failed: status=%d body=%s", resp.StatusCode, string(data))
	}
	var rows []struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	objects := make([]StorageObject, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" {
			continue
		}
		path := name
		if prefix != "" && !strings.HasPrefix(name, prefix+"/") {
			path = prefix + "/" + name
		}
		objects = append(objects, StorageObject{Name: name, Path: path})
	}
	return objects, nil
}

func escapedStoragePath(bucket, objectPath string) string {
	parts := append([]string{bucket}, strings.Split(objectPath, "/")...)
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
