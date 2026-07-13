// Package legal owns the documents served by the public Ohey legal endpoints.
package legal

import (
	_ "embed"
	"time"
)

// TermsText is the current Japanese terms of service.
//
//go:embed terms.txt
var TermsText string

// PrivacyPolicyText is the current Japanese privacy policy.
//
//go:embed privacy-policy.txt
var PrivacyPolicyText string

// LastModified is updated whenever either public legal document changes.
var LastModified = time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
