package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/yota/nomo/backend/internal/config"
	"github.com/yota/nomo/backend/internal/supabase"
)

const (
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	otherUserID   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	testLogID     = "11111111-2222-3333-4444-555555555555"
	testRequestID = "22222222-3333-4444-5555-666666666666"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

type fakeSupabase struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
	handler  func(http.ResponseWriter, *http.Request)
}

func newFakeSupabase(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *fakeSupabase {
	t.Helper()
	fake := &fakeSupabase{t: t, handler: handler}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		fake.mu.Lock()
		fake.requests = append(fake.requests, recordedRequest{
			Method: req.Method,
			Path:   req.URL.Path,
			Query:  req.URL.Query(),
			Body:   string(body),
		})
		fake.mu.Unlock()

		if req.URL.Path == "/auth/v1/user" {
			writeFakeJSON(w, http.StatusOK, map[string]any{"id": testUserID, "email": "user@example.com"})
			return
		}
		if handler != nil {
			handler(w, req)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeSupabase) client() *supabase.Client {
	return supabase.NewClient(f.server.URL, "test-key", f.server.Client())
}

func (f *fakeSupabase) lastRequest(path string) (recordedRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.requests) - 1; i >= 0; i-- {
		if f.requests[i].Path == path {
			return f.requests[i], true
		}
	}
	return recordedRequest{}, false
}

func testRouter(fake *fakeSupabase, adminEmails ...string) http.Handler {
	return NewRouter(Dependencies{
		Config: config.Config{
			SupabaseURL:            fake.server.URL,
			SupabaseAnonKey:        "anon-key",
			SupabaseServiceRoleKey: "service-role-key",
			AllowedOrigins:         []string{"*"},
			AdminEmails:            adminEmails,
		},
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Supabase:      fake.client(),
		AdminSupabase: fake.client(),
	})
}

func authedRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer access-token")
	req.Header.Set("X-Nomo-User-ID", testUserID)
	return req
}

func writeFakeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func TestAuthRejectsUserIDMismatch(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	req := authedRequest(http.MethodGet, "/v1/me/profile", "")
	req.Header.Set("X-Nomo-User-ID", otherUserID)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerRejectsOversizedJSONBody(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	largeToken := strings.Repeat("x", int(maxJSONBodyBytes)+1)
	req := authedRequest(http.MethodPut, "/v1/me/push-token", `{"token":"`+largeToken+`","platform":"ios"}`)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "JSON body is too large") {
		t.Fatalf("body does not mention JSON size: %s", w.Body.String())
	}
}

func TestHandlerRejectsInvalidUUIDAndDate(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	router := testRouter(fake)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "uuid", method: http.MethodDelete, path: "/v1/drink-logs/not-a-uuid"},
		{name: "date", method: http.MethodGet, path: "/v1/daily-status?date=2026/05/23"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, authedRequest(tc.method, tc.path, tc.body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSupabaseErrorsAreMasked(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/profiles" {
			http.Error(w, `{"secret":"service-role-leak","message":"raw upstream detail"}`, http.StatusInternalServerError)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/me/profile", ""))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "service-role-leak") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
	if !strings.Contains(body, "upstream service error") {
		t.Fatalf("unexpected masked error: %s", body)
	}
}

func TestAdminAccessRequiresConfiguredAdminEmail(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "admin@example.com").ServeHTTP(w, authedRequest(http.MethodGet, "/v1/admin/me", ""))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestDeleteDrinkLogIsScopedToAuthenticatedOwner(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/drink_logs" && req.Method == http.MethodDelete {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/drink-logs/"+testLogID, ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/drink_logs")
	if !ok {
		t.Fatal("drink_logs request was not sent")
	}
	if got := request.Query.Get("owner_user_id"); got != "eq."+testUserID {
		t.Fatalf("owner_user_id filter = %q", got)
	}
}

func TestUpdateFriendRequestIsScopedToAuthenticatedParticipant(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         string
		expectedFilter string
	}{
		{name: "accept recipient", status: "accepted", expectedFilter: "to_user_id"},
		{name: "cancel sender", status: "cancelled", expectedFilter: "from_user_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/rest/v1/friend_requests" && req.Method == http.MethodPatch {
					writeFakeJSON(w, http.StatusOK, []map[string]any{})
					return
				}
				writeFakeJSON(w, http.StatusOK, []map[string]any{})
			})
			w := httptest.NewRecorder()
			body := `{"status":"` + tc.status + `"}`

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/friend-requests/"+testRequestID, body))

			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
			request, ok := fake.lastRequest("/rest/v1/friend_requests")
			if !ok {
				t.Fatal("friend_requests request was not sent")
			}
			if got := request.Query.Get(tc.expectedFilter); got != "eq."+testUserID {
				t.Fatalf("%s filter = %q", tc.expectedFilter, got)
			}
		})
	}
}
