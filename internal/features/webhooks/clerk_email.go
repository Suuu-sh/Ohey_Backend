package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Suuu-sh/Ohey_Backend/internal/config"
)

const (
	clerkWebhookTolerance = 5 * time.Minute
	resendEmailsEndpoint  = "https://api.resend.com/emails"
)

type ClerkWebhookEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ClerkEmailEventData struct {
	ToEmailAddress string          `json:"to_email_address"`
	Subject        string          `json:"subject"`
	Body           string          `json:"body"`
	BodyHTML       string          `json:"body_html"`
	HTML           string          `json:"html"`
	BodyPlain      string          `json:"body_plain"`
	Text           string          `json:"text"`
	Slug           string          `json:"slug"`
	Data           json.RawMessage `json:"data"`
}

type ResendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
	ReplyTo string   `json:"reply_to,omitempty"`
}

type ClerkEmailWebhookResult struct {
	Ignored bool
}

func ProcessClerkEmailWebhook(headers http.Header, body []byte, cfg *config.Config, now time.Time) (ClerkEmailWebhookResult, error) {
	if cfg == nil {
		return ClerkEmailWebhookResult{}, errors.New("config is required")
	}
	if strings.TrimSpace(cfg.ClerkWebhookSecret) == "" {
		return ClerkEmailWebhookResult{}, errors.New("CLERK_WEBHOOK_SECRET is required for Clerk email webhooks")
	}
	if strings.TrimSpace(cfg.ResendAPIKey) == "" || strings.TrimSpace(cfg.ResendFromEmail) == "" {
		return ClerkEmailWebhookResult{}, errors.New("RESEND_API_KEY and RESEND_FROM_EMAIL are required for Clerk email delivery")
	}
	if err := verifyClerkWebhookSignature(headers, body, cfg.ClerkWebhookSecret, now); err != nil {
		return ClerkEmailWebhookResult{}, err
	}

	var envelope ClerkWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ClerkEmailWebhookResult{}, fmt.Errorf("invalid webhook json: %w", err)
	}
	if envelope.Type != "email.created" && envelope.Type != "emails.created" {
		return ClerkEmailWebhookResult{Ignored: true}, nil
	}

	var email ClerkEmailEventData
	if err := json.Unmarshal(envelope.Data, &email); err != nil {
		return ClerkEmailWebhookResult{}, fmt.Errorf("invalid email event data: %w", err)
	}
	if err := sendClerkEmailWithResend(cfg, email); err != nil {
		return ClerkEmailWebhookResult{}, err
	}
	return ClerkEmailWebhookResult{}, nil
}

func sendClerkEmailWithResend(cfg *config.Config, email ClerkEmailEventData) error {
	to := strings.TrimSpace(email.ToEmailAddress)
	subject := strings.TrimSpace(email.Subject)
	htmlBody := firstNonEmptyString(email.BodyHTML, email.HTML, email.Body)
	textBody := firstNonEmptyString(email.BodyPlain, email.Text)
	if to == "" {
		return errors.New("clerk email event is missing to_email_address")
	}
	if subject == "" {
		return errors.New("clerk email event is missing subject")
	}
	if strings.TrimSpace(htmlBody) == "" && strings.TrimSpace(textBody) == "" {
		return errors.New("clerk email event is missing email body")
	}

	return sendResendEmail(cfg, ResendEmailRequest{
		From:    strings.TrimSpace(cfg.ResendFromEmail),
		To:      []string{to},
		Subject: subject,
		HTML:    htmlBody,
		Text:    textBody,
		ReplyTo: strings.TrimSpace(cfg.ResendReplyToEmail),
	})
}

func sendResendEmail(cfg *config.Config, payload ResendEmailRequest) error {
	if strings.TrimSpace(cfg.ResendAPIKey) == "" || strings.TrimSpace(payload.From) == "" {
		return errors.New("RESEND_API_KEY and from email are required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode resend email: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, resendEmailsEndpoint, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("failed to create resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.ResendAPIKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email with resend: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("resend email failed: %s", msg)
	}
	return nil
}

func verifyClerkWebhookSignature(headers http.Header, body []byte, secret string, now time.Time) error {
	id := strings.TrimSpace(headers.Get("svix-id"))
	timestamp := strings.TrimSpace(headers.Get("svix-timestamp"))
	signature := strings.TrimSpace(headers.Get("svix-signature"))
	if id == "" || timestamp == "" || signature == "" {
		return errors.New("missing svix signature headers")
	}

	signedAt, err := parseUnixTimestamp(timestamp)
	if err != nil {
		return err
	}
	if now.Sub(signedAt) > clerkWebhookTolerance || signedAt.Sub(now) > clerkWebhookTolerance {
		return errors.New("webhook timestamp is outside tolerance")
	}

	key, err := decodeSvixSecret(secret)
	if err != nil {
		return err
	}
	msg := []byte(id + "." + timestamp + ".")
	msg = append(msg, body...)
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	expected := mac.Sum(nil)

	for _, part := range strings.Split(signature, " ") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "v1,")
		decoded, err := base64.StdEncoding.DecodeString(part)
		if err == nil && hmac.Equal(decoded, expected) {
			return nil
		}
	}
	return errors.New("signature mismatch")
}

func parseUnixTimestamp(raw string) (time.Time, error) {
	var seconds int64
	if _, err := fmt.Sscanf(raw, "%d", &seconds); err != nil {
		return time.Time{}, errors.New("invalid svix timestamp")
	}
	return time.Unix(seconds, 0), nil
}

func decodeSvixSecret(secret string) ([]byte, error) {
	secret = strings.TrimSpace(secret)
	secret = strings.TrimPrefix(secret, "whsec_")
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, errors.New("invalid clerk webhook secret")
	}
	return decoded, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
