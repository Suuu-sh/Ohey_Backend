package httpapi

import (
	"net/http"

	"github.com/Suuu-sh/Ohey_Backend/docs/legal"
)

func (r *router) legalTerms(w http.ResponseWriter, req *http.Request) {
	writeLegalText(w, req, legal.TermsText)
}

func (r *router) legalPrivacy(w http.ResponseWriter, req *http.Request) {
	writeLegalText(w, req, legal.PrivacyPolicyText)
}

func writeLegalText(w http.ResponseWriter, _ *http.Request, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Last-Modified", legal.LastModified.Format(http.TimeFormat))
	_, _ = w.Write([]byte(body))
}
