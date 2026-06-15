package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCleanUUID(t *testing.T) {
	id, errMessage := cleanUUID("  AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE  ", "user id")
	if errMessage != "" {
		t.Fatalf("cleanUUID returned error: %s", errMessage)
	}
	if id != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("cleanUUID normalized id = %q", id)
	}

	if _, errMessage := cleanUUID("not-a-uuid", "user id"); errMessage == "" {
		t.Fatal("cleanUUID accepted invalid uuid")
	}
}

func TestCleanUUIDsDeduplicatesAndRejectsInvalidIDs(t *testing.T) {
	ids, errMessage := cleanUUIDs([]string{
		"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"",
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE",
		"11111111-2222-3333-4444-555555555555",
	}, "friend id")
	if errMessage != "" {
		t.Fatalf("cleanUUIDs returned error: %s", errMessage)
	}
	if joined := strings.Join(ids, ","); joined != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee,11111111-2222-3333-4444-555555555555" {
		t.Fatalf("cleanUUIDs returned %q", joined)
	}

	if _, errMessage := cleanUUIDs([]string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "bad"}, "friend id"); errMessage == "" {
		t.Fatal("cleanUUIDs accepted an invalid id")
	}
}

func TestCleanDateOnlyOrToday(t *testing.T) {
	date, errMessage := cleanDateOnlyOrToday("2026-05-23", "date")
	if errMessage != "" {
		t.Fatalf("cleanDateOnlyOrToday returned error: %s", errMessage)
	}
	if date != "2026-05-23" {
		t.Fatalf("cleanDateOnlyOrToday returned %q", date)
	}
	if _, errMessage := cleanDateOnlyOrToday("2026/05/23", "date"); errMessage == "" {
		t.Fatal("cleanDateOnlyOrToday accepted invalid date")
	}
}

func TestDateOnlyParamRejectsInvalidDateInsteadOfFallingBackToToday(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/friends?date=2026/05/23", nil)
	w := httptest.NewRecorder()

	if got, ok := dateOnlyParam(w, req, "date"); ok || got != "" {
		t.Fatalf("dateOnlyParam() = (%q, %v), want rejection", got, ok)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDecodeJSONBodyRejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}{"extra":true}`))
	w := httptest.NewRecorder()
	var body map[string]bool

	if decodeJSONBody(w, req, &body) {
		t.Fatal("decodeJSONBody accepted multiple JSON values")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("decodeJSONBody status = %d", w.Code)
	}
}

func TestSanitizePostgRESTSearch(t *testing.T) {
	got := sanitizePostgRESTSearch(`  abc*(),.'"\\def  `)
	if got != "abcdef" {
		t.Fatalf("sanitizePostgRESTSearch returned %q", got)
	}
}
