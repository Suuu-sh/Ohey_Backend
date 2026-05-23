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

func TestCreateDrinkLogValidatesFriendIDsAndCreatesLinks(t *testing.T) {
	friendID := otherUserID
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": "friendship"}})
		case "/rest/v1/drink_logs":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{
				"id":            testLogID,
				"drank_at":      "2026-05-23T10:00:00Z",
				"owner_user_id": testUserID,
				"place_name":    "Test Bar",
				"memo":          "memo",
				"photo_path":    "",
				"is_official":   false,
			}})
		case "/rest/v1/drink_log_friends":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
		case "/rest/v1/notifications":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	body := `{"drank_at":"2026-05-23T10:00:00Z","place_name":" Test Bar ","memo":"memo","friend_ids":["` + friendID + `","` + friendID + `"]}`
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/drink-logs", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/drink_log_friends")
	if !ok {
		t.Fatal("drink_log_friends request was not sent")
	}
	if strings.Count(request.Body, friendID) != 1 {
		t.Fatalf("friend links were not deduplicated: %s", request.Body)
	}
}

func TestCreateDrinkLogRejectsInvalidFriendID(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/drink-logs", `{"friend_ids":["bad"]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestCreateDrinkLogRejectsNonFriendTag(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/friendships" {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/drink-logs", `{"friend_ids":["`+otherUserID+`"]}`))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if _, ok := fake.lastRequest("/rest/v1/drink_logs"); ok {
		t.Fatal("drink log insert was sent for a non-friend tag")
	}
}

func TestCreateFriendRequestValidatesAndScopesFriendshipCheck(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/friend_requests":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{
				"id":           testRequestID,
				"from_user_id": testUserID,
				"to_user_id":   otherUserID,
				"status":       "pending",
			}})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
		case "/rest/v1/notifications":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/friend-requests", `{"to_user_id":"`+otherUserID+`"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/friendships")
	if !ok || !strings.Contains(request.Query.Get("or"), testUserID) || !strings.Contains(request.Query.Get("or"), otherUserID) {
		t.Fatalf("friendship check query = %#v", request.Query)
	}
}

func TestCreateFriendRequestRejectsInvalidRecipient(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/friend-requests", `{"to_user_id":"bad"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestCreateDrinkInviteValidatesDateAndCreatesInvite(t *testing.T) {
	inviteID := "33333333-4444-5555-6666-777777777777"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/daily_statuses":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/drink_invites":
			if req.Method == http.MethodGet {
				writeFakeJSON(w, http.StatusOK, []map[string]any{})
				return
			}
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{
				"id":           inviteID,
				"from_user_id": testUserID,
				"to_user_id":   otherUserID,
				"invite_date":  "2026-05-23",
				"status":       "pending",
			}})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
		case "/rest/v1/notifications":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/drink-invites", `{"to_user_id":"`+otherUserID+`","invite_date":"2026-05-23"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/drink_invites")
	if !ok || request.Method != http.MethodPost || !strings.Contains(request.Body, "2026-05-23") {
		t.Fatalf("invite create request = %#v", request)
	}
}

func TestCreateDrinkInviteRejectsInvalidDate(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/drink-invites", `{"to_user_id":"`+otherUserID+`","invite_date":"2026/05/23"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestAdminCreateNotificationValidatesRecipientsAndDeduplicates(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/auth/v1/user" {
			writeFakeJSON(w, http.StatusOK, map[string]any{"id": testUserID, "email": "user@example.com"})
			return
		}
		if req.URL.Path == "/rest/v1/notifications" && req.Method == http.MethodPost {
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()
	body := `{"title":"Title","message":"Message","recipient_user_ids":["` + otherUserID + `","` + otherUserID + `"]}`

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/notifications", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"recipient_count":1`) || !strings.Contains(w.Body.String(), `"created_count":1`) {
		t.Fatalf("unexpected notification result: %s", w.Body.String())
	}
}

func TestAdminCreateNotificationRejectsInvalidRecipient(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/notifications", `{"title":"Title","message":"Message","recipient_user_ids":["bad"]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestRegisterPushTokenValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing token", body: `{"platform":"ios"}`},
		{name: "invalid platform", body: `{"token":"token","platform":"web"}`},
		{name: "too long", body: `{"token":"` + strings.Repeat("x", 4097) + `","platform":"ios"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeSupabase(t, nil)
			w := httptest.NewRecorder()

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, "/v1/me/push-token", tc.body))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdminAccessAllowsConfiguredAdminEmailCaseInsensitive(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "USER@EXAMPLE.COM").ServeHTTP(w, authedRequest(http.MethodGet, "/v1/admin/me", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"is_admin":true`) {
		t.Fatalf("unexpected admin response: %s", w.Body.String())
	}
}

func TestSupabaseClientErrorsAreMasked(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/profiles" {
			http.Error(w, `{"message":"duplicate key secret raw sql detail","token":"secret-token"}`, http.StatusBadRequest)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/me/profile", `{"display_name":"Name"}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "duplicate key") || strings.Contains(body, "secret-token") || strings.Contains(body, "raw sql") {
		t.Fatalf("raw upstream client error leaked: %s", body)
	}
	if !strings.Contains(body, "request rejected by upstream service") {
		t.Fatalf("unexpected masked client error: %s", body)
	}
}

func TestUpdateDrinkInviteIsScopedToAuthenticatedRecipientAndPending(t *testing.T) {
	inviteID := "33333333-4444-5555-6666-777777777777"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/drink_invites" && req.Method == http.MethodPatch {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/drink-invites/"+inviteID, `{"status":"accepted"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/drink_invites")
	if !ok {
		t.Fatal("drink_invites request was not sent")
	}
	if got := request.Query.Get("to_user_id"); got != "eq."+testUserID {
		t.Fatalf("to_user_id filter = %q", got)
	}
	if got := request.Query.Get("status"); got != "eq.pending" {
		t.Fatalf("status filter = %q", got)
	}
}

func TestRegisterPushTokenScopesToAuthenticatedUser(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/push_tokens" && req.Method == http.MethodPost {
			writeFakeJSON(w, http.StatusCreated, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, "/v1/me/push-token", `{"token":"device-token","platform":"ios"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/push_tokens")
	if !ok {
		t.Fatal("push_tokens request was not sent")
	}
	if !strings.Contains(request.Body, `"user_id":"`+testUserID+`"`) {
		t.Fatalf("push token body is not scoped to auth user: %s", request.Body)
	}
}

func TestUpdateProfileValidatesUserIDAndScopesToAuthUser(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/profiles" && req.Method == http.MethodPatch {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": testUserID}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	router := testRouter(fake)

	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, authedRequest(http.MethodPatch, "/v1/me/profile", `{"user_id":"bad user id"}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d body = %s", invalid.Code, invalid.Body.String())
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/me/profile", `{"user_id":"valid_user","display_name":"Name"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/profiles")
	if !ok {
		t.Fatal("profiles patch request was not sent")
	}
	if got := request.Query.Get("id"); got != "eq."+testUserID {
		t.Fatalf("profile id filter = %q", got)
	}
}

func TestMarkNotificationsReadIsScopedToAuthenticatedRecipient(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/notifications" && req.Method == http.MethodPatch {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": "notification"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/notifications/read-all", `{}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/notifications")
	if !ok {
		t.Fatal("notifications patch request was not sent")
	}
	if got := request.Query.Get("recipient_user_id"); got != "eq."+testUserID {
		t.Fatalf("recipient_user_id filter = %q", got)
	}
	if got := request.Query.Get("read_at"); got != "is.null" {
		t.Fatalf("read_at filter = %q", got)
	}
}

func TestAdminBackendRequiresServiceRole(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	router := NewRouter(Dependencies{
		Config: config.Config{
			SupabaseURL:     fake.server.URL,
			SupabaseAnonKey: "anon-key",
			AllowedOrigins:  []string{"*"},
			AdminEmails:     []string{"user@example.com"},
		},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Supabase: fake.client(),
	})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/admin/me", ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteUserRejectsSelfDelete(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/admin/users/"+testUserID, ""))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if _, ok := fake.lastRequest("/auth/v1/admin/users/" + testUserID); ok {
		t.Fatal("admin delete user request was sent for self-delete")
	}
}
