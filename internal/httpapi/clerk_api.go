package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const clerkAPIBaseURL = "https://api.clerk.com/v1"

type clerkAPIClient struct {
	secretKey  string
	httpClient *http.Client
}

func NewClerkAPIClientForDependencies(secretKey string, httpClient *http.Client) *clerkAPIClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &clerkAPIClient{secretKey: strings.TrimSpace(secretKey), httpClient: httpClient}
}

func (c *clerkAPIClient) configured() bool { return c != nil && c.secretKey != "" }

func (c *clerkAPIClient) DeleteUser(ctx context.Context, clerkUserID string) error {
	if !c.configured() {
		return fmt.Errorf("clerk api is not configured")
	}
	clerkUserID = strings.TrimSpace(clerkUserID)
	if clerkUserID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, clerkAPIBaseURL+"/users/"+clerkUserID, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("clerk delete user failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func (c *clerkAPIClient) CreateUser(ctx context.Context, email, password, userID, displayName, avatarURL string) (map[string]any, error) {
	if !c.configured() {
		return nil, fmt.Errorf("clerk api is not configured")
	}
	payload := map[string]any{
		"email_address":             []string{email},
		"password":                  password,
		"skip_password_checks":      false,
		"skip_password_requirement": false,
		"public_metadata":           map[string]any{"user_id": userID, "display_name": displayName, "avatar_url": avatarURL},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, clerkAPIBaseURL+"/users", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return out, nil
	}
	return nil, fmt.Errorf("clerk create user failed: status=%d body=%v", resp.StatusCode, out)
}

func (c *clerkAPIClient) UpdateUser(ctx context.Context, clerkUserID string, payload map[string]any) error {
	if !c.configured() {
		return fmt.Errorf("clerk api is not configured")
	}
	if strings.TrimSpace(clerkUserID) == "" || len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, clerkAPIBaseURL+"/users/"+strings.TrimSpace(clerkUserID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("clerk update user failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
}

func (c *clerkAPIClient) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
}
