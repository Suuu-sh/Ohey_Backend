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
	testMemoryID  = "11111111-2222-3333-4444-555555555555"
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
		{name: "uuid", method: http.MethodDelete, path: "/v1/memories/not-a-uuid"},
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

func TestAdminListUsersUsesRequestedStatusDate(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":           otherUserID,
				"user_id":      "friend",
				"display_name": "Friend",
			}})
		case "/rest/v1/daily_statuses":
			if got := req.URL.Query().Get("status_date"); got != "eq.2026-05-26" {
				t.Fatalf("status_date filter = %q", got)
			}
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"user_id": otherUserID,
				"status":  "maybe_available",
			}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodGet, "/v1/admin/users?date=2026-05-26", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := rows[0]["status"]; got != "maybe_available" {
		t.Fatalf("status = %#v", got)
	}
}

func TestAdminUpdateUserWritesRequestedStatusDate(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(
		w,
		authedRequest(http.MethodPatch, "/v1/admin/users/"+otherUserID, `{"status":"maybe_available","status_date":"2026-05-26"}`),
	)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/daily_statuses")
	if !ok {
		t.Fatal("daily_statuses request was not sent")
	}
	if !strings.Contains(request.Body, `"status_date":"2026-05-26"`) {
		t.Fatalf("daily status body missing requested date: %s", request.Body)
	}
	if !strings.Contains(request.Body, `"status":"maybe_available"`) {
		t.Fatalf("daily status body missing status: %s", request.Body)
	}
}

func TestDeleteMemoryIsScopedToAuthenticatedOwner(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/memories" && req.Method == http.MethodDelete {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/memories/"+testMemoryID, ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/memories")
	if !ok {
		t.Fatal("memories request was not sent")
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

func TestDeleteFriendshipIsScopedToAuthenticatedUserPair(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/friendships" && req.Method == http.MethodDelete {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": "friendship"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/friends/"+otherUserID, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/friendships")
	if !ok || request.Method != http.MethodDelete {
		t.Fatalf("friendship delete request = %#v", request)
	}
	filter := request.Query.Get("or")
	if !strings.Contains(filter, testUserID) || !strings.Contains(filter, otherUserID) {
		t.Fatalf("friendship delete filter = %q", filter)
	}
}

func TestGetFriendRequestStatusReturnsRequestID(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/friend_requests":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":           testRequestID,
				"from_user_id": testUserID,
				"to_user_id":   otherUserID,
			}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/friend-requests/status?friend_id="+otherUserID, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"request_id":"`+testRequestID+`"`) {
		t.Fatalf("body missing request_id: %s", w.Body.String())
	}
}

func TestListFriendRequestsScopesDirection(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/friend_requests" && req.Method == http.MethodGet {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":           testRequestID,
				"from_user_id": otherUserID,
				"to_user_id":   testUserID,
				"status":       "pending",
				"from_user": map[string]any{
					"id":           otherUserID,
					"user_id":      "friend_1",
					"display_name": "Friend",
				},
			}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/friend-requests?direction=incoming", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/friend_requests")
	if !ok {
		t.Fatal("friend_requests request was not sent")
	}
	if got := request.Query.Get("status"); got != "eq.pending" {
		t.Fatalf("status filter = %q", got)
	}
	if got := request.Query.Get("to_user_id"); got != "eq."+testUserID {
		t.Fatalf("to_user_id filter = %q", got)
	}
	if !strings.Contains(request.Query.Get("select"), "from_user:profiles") {
		t.Fatalf("select missing embedded profile: %q", request.Query.Get("select"))
	}
}

func TestListBlockedUsersReturnsTargetProfiles(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/user_blocks":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"blocked_user_id": otherUserID,
				"created_at":      "2026-05-28T00:00:00Z",
			}})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":           otherUserID,
				"user_id":      "friend_1",
				"display_name": "Friend",
			}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/user-blocks", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/user_blocks")
	if !ok || request.Query.Get("blocker_user_id") != "eq."+testUserID {
		t.Fatalf("user_blocks request = %#v", request)
	}
	if !strings.Contains(w.Body.String(), `"target_user_id":"`+otherUserID+`"`) {
		t.Fatalf("body missing target_user_id: %s", w.Body.String())
	}
}

func TestCreateMemoryValidatesFriendIDsAndCreatesLinks(t *testing.T) {
	friendID := otherUserID
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": "friendship"}})
		case "/rest/v1/memories":
			if req.Method == http.MethodGet {
				writeFakeJSON(w, http.StatusOK, []map[string]any{})
				return
			}
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{
				"id":            testMemoryID,
				"happened_at":   "2026-05-23T10:00:00Z",
				"owner_user_id": testUserID,
				"place_name":    "Test Bar",
				"memo":          "memo",
				"photo_path":    "",
				"is_official":   false,
			}})
		case "/rest/v1/memory_tagged_users":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
		case "/rest/v1/notifications":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	body := `{"happened_at":"2026-05-23T10:00:00Z","place_name":" Test Bar ","memo":"memo","friend_ids":["` + friendID + `","` + friendID + `"]}`
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/memories", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/memory_tagged_users")
	if !ok {
		t.Fatal("memory_tagged_users request was not sent")
	}
	if strings.Count(request.Body, friendID) != 1 {
		t.Fatalf("friend links were not deduplicated: %s", request.Body)
	}
}

func TestCreateMemoryRejectsExistingLogOnSameLocalDay(t *testing.T) {
	insertSent := false
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": "friendship"}})
		case "/rest/v1/memories":
			if req.Method == http.MethodGet {
				query := req.URL.Query()
				if got := query.Get("owner_user_id"); got != "eq."+testUserID {
					t.Fatalf("owner_user_id filter = %q", got)
				}
				if got := query.Get("is_official"); got != "eq.false" {
					t.Fatalf("is_official filter = %q", got)
				}
				filters := query["happened_at"]
				if len(filters) != 2 ||
					filters[0] != "gte.2026-05-23T15:00:00Z" ||
					filters[1] != "lt.2026-05-24T15:00:00Z" {
					t.Fatalf("happened_at filters = %#v", filters)
				}
				writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": testMemoryID}})
				return
			}
			insertSent = true
			writeFakeJSON(w, http.StatusCreated, []map[string]any{})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	body := `{"happened_at":"2026-05-24T12:30:00Z","happened_on":"2026-05-24","timezone_offset_minutes":540,"friend_ids":["` + otherUserID + `"]}`
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/memories", body))

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if insertSent {
		t.Fatal("memory insert was sent despite same-day existing memory")
	}
	if !strings.Contains(w.Body.String(), "1日1つ") {
		t.Fatalf("body does not explain daily limit: %s", w.Body.String())
	}
}

func TestCreateMemoryRejectsInvalidFriendID(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/memories", `{"friend_ids":["bad"]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestCreateMemoryRejectsNonFriendTag(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/friendships" {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/memories", `{"friend_ids":["`+otherUserID+`"]}`))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if _, ok := fake.lastRequest("/rest/v1/memories"); ok {
		t.Fatal("memory insert was sent for a non-friend tag")
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

func TestCreateInviteValidatesDateAndCreatesInvite(t *testing.T) {
	inviteID := "33333333-4444-5555-6666-777777777777"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/daily_statuses":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/invites":
			if req.Method == http.MethodGet {
				writeFakeJSON(w, http.StatusOK, []map[string]any{})
				return
			}
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{
				"id":              inviteID,
				"inviter_user_id": testUserID,
				"invitee_user_id": otherUserID,
				"scheduled_date":  "2026-05-23",
				"status":          "pending",
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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/invites", `{"invitee_user_id":"`+otherUserID+`","scheduled_date":"2026-05-23"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/invites")
	if !ok || request.Method != http.MethodPost || !strings.Contains(request.Body, "2026-05-23") {
		t.Fatalf("invite create request = %#v", request)
	}
}

func TestCreateInviteRejectsInvalidDate(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/invites", `{"invitee_user_id":"`+otherUserID+`","scheduled_date":"2026/05/23"}`))

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

func TestCreateMediaUploadURLCreatesUserScopedMemoryPhotoPath(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/storage/v1/object/upload/sign/nomo-photos/users/"+testUserID+"/memories/") {
			writeFakeJSON(w, http.StatusOK, map[string]any{
				"url":   "/object/upload/sign/nomo-photos/users/" + testUserID + "/memories/photo.jpg?token=upload-token",
				"token": "upload-token",
			})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, "/v1/media/upload-url", `{"kind":"memory_photo","file_extension":".jpg","content_type":"image/jpeg"}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["bucket"] != "nomo-photos" || body["token"] != "upload-token" || body["content_type"] != "image/jpeg" {
		t.Fatalf("response = %#v", body)
	}
	path, _ := body["path"].(string)
	if !strings.HasPrefix(path, "users/"+testUserID+"/memories/") || !strings.HasSuffix(path, ".jpg") {
		t.Fatalf("path = %q", path)
	}
	request, ok := fake.lastRequest("/storage/v1/object/upload/sign/nomo-photos/" + path)
	if !ok {
		t.Fatalf("storage signed upload request for %q was not sent", path)
	}
	if request.Method != http.MethodPost {
		t.Fatalf("storage method = %s, want POST", request.Method)
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

func TestUpdateInviteIsScopedToAuthenticatedRecipientAndPending(t *testing.T) {
	inviteID := "33333333-4444-5555-6666-777777777777"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/invites" && req.Method == http.MethodPatch {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/invites/"+inviteID, `{"status":"accepted"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/invites")
	if !ok {
		t.Fatal("invites request was not sent")
	}
	if got := request.Query.Get("invitee_user_id"); got != "eq."+testUserID {
		t.Fatalf("invitee_user_id filter = %q", got)
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

func TestUnregisterPushTokenScopesToAuthenticatedUser(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/push_tokens" && req.Method == http.MethodDelete {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"token": "device-token"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/me/push-token", `{"token":"device-token"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/push_tokens")
	if !ok {
		t.Fatal("push_tokens delete request was not sent")
	}
	if got := request.Query.Get("token"); got != "eq.device-token" {
		t.Fatalf("token filter = %q", got)
	}
	if got := request.Query.Get("user_id"); got != "eq."+testUserID {
		t.Fatalf("user_id filter = %q", got)
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

func TestUpsertProfileNormalizesAndScopesToAuthUser(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/profiles" && req.Method == http.MethodPost {
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, "/v1/me/profile", `{"user_id":" valid_user ","display_name":" Name ","gender":"","character_key":"","avatar_url":" avatar.png "}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/profiles")
	if !ok {
		t.Fatal("profiles upsert request was not sent")
	}
	if got := request.Query.Get("on_conflict"); got != "id" {
		t.Fatalf("on_conflict = %q", got)
	}
	for _, want := range []string{
		`"id":"` + testUserID + `"`,
		`"user_id":"valid_user"`,
		`"display_name":"Name"`,
		`"gender":"unspecified"`,
		`"character_key":"avatar"`,
		`"avatar_url":"avatar.png"`,
	} {
		if !strings.Contains(request.Body, want) {
			t.Fatalf("profile body %s does not contain %s", request.Body, want)
		}
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

func TestDeleteOwnAccountUsesAdminAuthDelete(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/auth/v1/admin/users/"+testUserID && req.Method == http.MethodDelete {
			writeFakeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, "/v1/me/account", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/auth/v1/admin/users/" + testUserID)
	if !ok || request.Method != http.MethodDelete {
		t.Fatalf("admin delete request = %#v", request)
	}
}

func TestUpdateFriendRequestAcceptedCreatesFriendship(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friend_requests":
			if req.Method == http.MethodPatch {
				writeFakeJSON(w, http.StatusOK, []map[string]any{{
					"id":           testRequestID,
					"from_user_id": otherUserID,
					"to_user_id":   testUserID,
					"status":       "accepted",
				}})
				return
			}
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "friendship"}})
			return
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
			return
		case "/rest/v1/notifications":
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/friend-requests/"+testRequestID, `{"status":"accepted"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/friendships")
	if !ok {
		t.Fatal("friendship upsert request was not sent")
	}
	if !strings.Contains(request.Body, testUserID) || !strings.Contains(request.Body, otherUserID) {
		t.Fatalf("friendship body does not include both users: %s", request.Body)
	}
}

func TestUpdateFriendRequestRejectedDoesNotCreateFriendship(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/friend_requests" && req.Method == http.MethodPatch {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":           testRequestID,
				"from_user_id": otherUserID,
				"to_user_id":   testUserID,
				"status":       "rejected",
			}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/friend-requests/"+testRequestID, `{"status":"rejected"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if _, ok := fake.lastRequest("/rest/v1/friendships"); ok {
		t.Fatal("friendship was created for rejected request")
	}
}

func TestUpdateInviteAcceptedOnlyCreatesAcceptedNotification(t *testing.T) {
	inviteID := "33333333-4444-5555-6666-777777777777"
	for _, tc := range []struct {
		status                 string
		expectNotificationPost bool
	}{
		{status: "accepted", expectNotificationPost: true},
		{status: "rejected", expectNotificationPost: false},
	} {
		t.Run(tc.status, func(t *testing.T) {
			fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
				switch req.URL.Path {
				case "/rest/v1/invites":
					if req.Method == http.MethodPatch {
						writeFakeJSON(w, http.StatusOK, []map[string]any{{
							"id":              inviteID,
							"inviter_user_id": otherUserID,
							"invitee_user_id": testUserID,
							"status":          tc.status,
						}})
						return
					}
				case "/rest/v1/profiles":
					writeFakeJSON(w, http.StatusOK, []map[string]any{{"display_name": "Actor", "user_id": "actor"}})
					return
				case "/rest/v1/notifications":
					writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
					return
				}
				writeFakeJSON(w, http.StatusOK, []map[string]any{})
			})
			w := httptest.NewRecorder()

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/invites/"+inviteID, `{"status":"`+tc.status+`"}`))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
			_, notificationPosted := fake.lastRequest("/rest/v1/notifications")
			if notificationPosted != tc.expectNotificationPost {
				t.Fatalf("notification posted = %v, want %v", notificationPosted, tc.expectNotificationPost)
			}
		})
	}
}

func TestAdminCreateNotificationSendToAllAndConflictPartialResult(t *testing.T) {
	postCount := 0
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{"id": otherUserID}, {"id": testUserID}})
		case "/rest/v1/notifications":
			postCount++
			if postCount == 1 {
				http.Error(w, `{"code":"23505","message":"duplicate"}`, http.StatusConflict)
				return
			}
			writeFakeJSON(w, http.StatusCreated, []map[string]any{{"id": "notification"}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()
	body := `{"title":"Title","message":"Message","send_to_all":true,"recipient_user_ids":["` + otherUserID + `"]}`

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/notifications", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"recipient_count":2`) || !strings.Contains(w.Body.String(), `"created_count":1`) {
		t.Fatalf("unexpected partial notification result: %s", w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/profiles")
	if !ok {
		t.Fatal("send_to_all profiles query was not sent")
	}
	if request.Query.Get("limit") != "10000" || request.Query.Get("select") != "id" {
		t.Fatalf("unexpected send_to_all query: %#v", request.Query)
	}
}

func TestUpdateProfileRejectsImmutableAndOversizedFields(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	router := testRouter(fake)

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "gender", body: `{"gender":"male"}`, want: "gender cannot be changed"},
		{name: "display name length", body: `{"display_name":"` + strings.Repeat("名", 41) + `"}`, want: "display_name must be 1-40 characters"},
		{name: "avatar length", body: `{"avatar_url":"` + strings.Repeat("x", 4097) + `"}`, want: "avatar_url is too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/me/profile", tc.body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Fatalf("body %q does not contain %q", w.Body.String(), tc.want)
			}
		})
	}
	if _, ok := fake.lastRequest("/rest/v1/profiles"); ok {
		t.Fatal("invalid profile payload should not reach Supabase")
	}
}

func TestAdminCreateNotificationMasksNonConflictInsertError(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/notifications" && req.Method == http.MethodPost {
			http.Error(w, `{"secret":"service-role-leak","message":"raw upstream detail"}`, http.StatusInternalServerError)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, "/v1/admin/notifications", `{"title":"お知らせ","message":"本文","recipient_user_ids":["`+testUserID+`"]}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "service-role-leak") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
}

func TestRegisterPushTokenMasksSupabaseError(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/push_tokens" && req.Method == http.MethodPost {
			http.Error(w, `{"secret":"push-token-secret","message":"raw upstream detail"}`, http.StatusInternalServerError)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, "/v1/me/push-token", `{"token":"device-token","platform":"ios"}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "push-token-secret") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
}

func TestMarkNotificationsReadMasksSupabaseError(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/notifications" && req.Method == http.MethodPatch {
			http.Error(w, `{"secret":"notification-secret","message":"raw upstream detail"}`, http.StatusInternalServerError)
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/notifications/read-all", `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "notification-secret") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
}

func TestListFriendsAttachesMemoryStats(t *testing.T) {
	friendID := otherUserID
	older := "2026-05-20T10:00:00Z"
	newer := "2026-05-22T12:30:00Z"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"user_a_id":   testUserID,
				"user_b_id":   friendID,
				"is_favorite": true,
				"user_a":      map[string]any{"id": testUserID, "user_id": "me", "display_name": "Me"},
				"user_b":      map[string]any{"id": friendID, "user_id": "friend", "display_name": "Friend"},
			}})
		case "/rest/v1/daily_statuses":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/memory_tagged_users":
			if req.URL.Query().Get("tagged_user_id") == "eq."+testUserID {
				writeFakeJSON(w, http.StatusOK, []map[string]any{{
					"tagged_user_id": testUserID,
					"memories":       map[string]any{"owner_user_id": friendID, "happened_at": newer},
				}})
				return
			}
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"tagged_user_id": friendID,
				"memories":       map[string]any{"owner_user_id": testUserID, "happened_at": older},
			}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/friends", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d body = %s", len(rows), w.Body.String())
	}
	friend, ok := rows[0]["user_b"].(map[string]any)
	if !ok {
		t.Fatalf("user_b missing: %#v", rows[0])
	}
	if got := friend["total_memory_count"]; got != float64(2) {
		t.Fatalf("total_memory_count = %#v", got)
	}
	if got := friend["last_memory_at"]; got != newer {
		t.Fatalf("last_memory_at = %#v", got)
	}

	ownedReq, ok := fake.lastRequest("/rest/v1/memory_tagged_users")
	if !ok {
		t.Fatal("memory_tagged_users request was not sent")
	}
	if got := ownedReq.Query.Get("select"); got != "tagged_user_id,memories!inner(owner_user_id,happened_at)" {
		t.Fatalf("select = %q", got)
	}
}

func TestListFriendsIgnoresMemoryStatsSupabaseError(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/rest/v1/friendships":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"user_a_id": testUserID,
				"user_b_id": otherUserID,
				"user_a":    map[string]any{"id": testUserID, "user_id": "me", "display_name": "Me"},
				"user_b":    map[string]any{"id": otherUserID, "user_id": "friend", "display_name": "Friend"},
			}})
		case "/rest/v1/daily_statuses":
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		case "/rest/v1/memory_tagged_users":
			http.Error(w, `{"secret":"memory-stats-secret","message":"raw upstream detail"}`, http.StatusInternalServerError)
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, "/v1/friends", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "memory-stats-secret") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d body = %s", len(rows), body)
	}
}
