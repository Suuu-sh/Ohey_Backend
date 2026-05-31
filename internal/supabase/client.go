package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, http: httpClient}
}

func (c *Client) Get(ctx context.Context, authToken, table string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, authToken, table, query, nil, out, nil)
}

func (c *Client) Post(ctx context.Context, authToken, table string, query url.Values, body any, out any) error {
	headers := map[string]string{"Prefer": "return=representation"}
	return c.do(ctx, http.MethodPost, authToken, table, query, body, out, headers)
}

func (c *Client) Upsert(ctx context.Context, authToken, table string, query url.Values, body any, out any) error {
	headers := map[string]string{"Prefer": "return=representation,resolution=merge-duplicates"}
	return c.do(ctx, http.MethodPost, authToken, table, query, body, out, headers)
}

func (c *Client) Patch(ctx context.Context, authToken, table string, query url.Values, body any, out any) error {
	headers := map[string]string{"Prefer": "return=representation"}
	return c.do(ctx, http.MethodPatch, authToken, table, query, body, out, headers)
}

func (c *Client) Delete(ctx context.Context, authToken, table string, query url.Values, out any) error {
	headers := map[string]string{"Prefer": "return=representation"}
	return c.do(ctx, http.MethodDelete, authToken, table, query, nil, out, headers)
}

func (c *Client) do(ctx context.Context, method, authToken, table string, query url.Values, body any, out any, headers map[string]string) error {
	endpoint := fmt.Sprintf("%s/rest/v1/%s", c.baseURL, table)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	return c.doURL(ctx, method, endpoint, authToken, body, out, headers)
}

func (c *Client) doURL(ctx context.Context, method, endpoint, authToken string, body any, out any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) GetAuthUser(ctx context.Context, accessToken string, out any) error {
	endpoint := fmt.Sprintf("%s/auth/v1/user", c.baseURL)
	return c.doURL(ctx, http.MethodGet, endpoint, accessToken, nil, out, nil)
}

func (c *Client) GetAuthJWKS(ctx context.Context, out any) error {
	endpoint := fmt.Sprintf("%s/auth/v1/.well-known/jwks.json", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) AdminCreateUser(ctx context.Context, body any, out any) error {
	endpoint := fmt.Sprintf("%s/auth/v1/admin/users", c.baseURL)
	return c.doURL(ctx, http.MethodPost, endpoint, c.apiKey, body, out, nil)
}

func (c *Client) AdminUpdateUser(ctx context.Context, userID string, body any, out any) error {
	endpoint := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.baseURL, url.PathEscape(userID))
	return c.doURL(ctx, http.MethodPut, endpoint, c.apiKey, body, out, nil)
}

func (c *Client) AdminDeleteUser(ctx context.Context, userID string) error {
	endpoint := fmt.Sprintf("%s/auth/v1/admin/users/%s", c.baseURL, url.PathEscape(userID))
	return c.doURL(ctx, http.MethodDelete, endpoint, c.apiKey, nil, nil, nil)
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e APIError) Error() string {
	return fmt.Sprintf("supabase api error: status=%d body=%s", e.StatusCode, e.Body)
}
