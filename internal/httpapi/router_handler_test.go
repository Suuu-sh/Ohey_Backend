package httpapi

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yota/ohey/backend/internal/config"
	"github.com/yota/ohey/backend/internal/contracts"
	"github.com/yota/ohey/backend/internal/supabase"
)

const (
	testUserID    = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	otherUserID   = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	testRequestID = "22222222-3333-4444-5555-666666666666"
)

func contractPath(pattern string, replacements ...string) string {
	if len(replacements)%2 != 0 {
		panic("contractPath replacements must be key/value pairs")
	}
	path := pattern
	for i := 0; i < len(replacements); i += 2 {
		path = strings.ReplaceAll(path, "{"+replacements[i]+"}", replacements[i+1])
	}
	return path
}

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

func (f *fakeSupabase) countRequests(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, request := range f.requests {
		if request.Path == path {
			count++
		}
	}
	return count
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
	req.Header.Set("X-Ohey-User-ID", testUserID)
	return req
}

func writeFakeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func signedSupabaseJWT(t *testing.T, privateKey *rsa.PrivateKey, issuer, kid, subject, email string, expiresAt time.Time) string {
	t.Helper()
	header := map[string]any{"typ": "JWT", "alg": "RS256", "kid": kid}
	claims := map[string]any{
		"iss":   issuer,
		"sub":   subject,
		"aud":   "authenticated",
		"role":  "authenticated",
		"email": email,
		"exp":   expiresAt.Unix(),
		"iat":   time.Now().Add(-time.Minute).Unix(),
	}
	headerPart := encodeJWTPart(t, header)
	claimsPart := encodeJWTPart(t, claims)
	signingInput := headerPart + "." + claimsPart
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func encodeJWTPart(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal jwt part: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func rsaPublicJWK(key *rsa.PublicKey, kid string) map[string]any {
	return map[string]any{
		"kty":     "RSA",
		"kid":     kid,
		"alg":     "RS256",
		"key_ops": []string{"verify"},
		"n":       base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":       base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func TestAuthUsesJWKSVerifiedSubjectWithoutAuthUserRoundTrip(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	const kid = "test-key"
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/auth/v1/.well-known/jwks.json":
			writeFakeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{rsaPublicJWK(&privateKey.PublicKey, kid)}})
		case "/rest/v1/profiles":
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":            testUserID,
				"user_id":       "valid_user",
				"display_name":  "Valid User",
				"character_key": "avatar",
				"is_plus":       false,
			}})
		default:
			writeFakeJSON(w, http.StatusOK, []map[string]any{})
		}
	})
	token := signedSupabaseJWT(t, privateKey, fake.server.URL+"/auth/v1", kid, testUserID, "user@example.com", time.Now().Add(time.Hour))
	req := authedRequest(http.MethodGet, contracts.APIPathMeProfile, "")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if got := fake.countRequests("/auth/v1/user"); got != 0 {
		t.Fatalf("auth user round trips = %d, want 0", got)
	}
	if got := fake.countRequests("/auth/v1/.well-known/jwks.json"); got != 1 {
		t.Fatalf("jwks requests = %d, want 1", got)
	}
}

func TestAuthCachesAuthServerUserForOpaqueToken(t *testing.T) {
	fake := newFakeSupabase(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/rest/v1/profiles" {
			writeFakeJSON(w, http.StatusOK, []map[string]any{{
				"id":            testUserID,
				"user_id":       "valid_user",
				"display_name":  "Valid User",
				"character_key": "avatar",
				"is_plus":       false,
			}})
			return
		}
		writeFakeJSON(w, http.StatusOK, []map[string]any{})
	})
	router := testRouter(fake)

	for range 2 {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathMeProfile, ""))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
		}
	}

	if got := fake.countRequests("/auth/v1/user"); got != 1 {
		t.Fatalf("auth user round trips = %d, want 1", got)
	}
}

func TestAuthRejectsExpiredJWKSJWTWithoutAuthUserFallback(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	fake := newFakeSupabase(t, nil)
	token := signedSupabaseJWT(t, privateKey, fake.server.URL+"/auth/v1", "test-key", testUserID, "user@example.com", time.Now().Add(-time.Hour))
	req := authedRequest(http.MethodGet, contracts.APIPathMeProfile, "")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if got := fake.countRequests("/auth/v1/user"); got != 0 {
		t.Fatalf("auth user fallback requests = %d, want 0", got)
	}
	if got := fake.countRequests("/auth/v1/.well-known/jwks.json"); got != 0 {
		t.Fatalf("jwks requests = %d, want 0", got)
	}
}

func TestAuthRejectsUserIDMismatch(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	req := authedRequest(http.MethodGet, contracts.APIPathMeProfile, "")
	req.Header.Set("X-Ohey-User-ID", otherUserID)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestHandlerRejectsOversizedJSONBody(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	largeToken := strings.Repeat("x", int(maxJSONBodyBytes)+1)
	req := authedRequest(http.MethodPut, contracts.APIPathMePushToken, `{"token":"`+largeToken+`","platform":"ios"}`)
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
		{name: "date", method: http.MethodGet, path: contracts.APIPathDailyStatus + "?date=2026/05/23"},
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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathMeProfile, ""))

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

	testRouter(fake, "admin@example.com").ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathAdminMe, ""))

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

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathAdminUsers+"?date=2026-05-26", ""))

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
		authedRequest(http.MethodPatch, contractPath(contracts.APIPathAdminUser, "id", otherUserID), `{"status":"maybe_available","status_date":"2026-05-26"}`),
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

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contractPath(contracts.APIPathFriendRequest, "id", testRequestID), body))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, contractPath(contracts.APIPathFriend, "id", otherUserID), ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathFriendReqStatus+"?friend_id="+otherUserID, ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathFriendRequests+"?direction=incoming", ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathUserBlocks, ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathFriendRequests, `{"to_user_id":"`+otherUserID+`"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathFriendRequests, `{"to_user_id":"bad"}`))

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
				"activity_label":  "焼肉",
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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathInvites, `{"invitee_user_id":"`+otherUserID+`","scheduled_date":"2026-05-23","activity_label":" 焼肉 "}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	request, ok := fake.lastRequest("/rest/v1/invites")
	if !ok || request.Method != http.MethodPost || !strings.Contains(request.Body, "2026-05-23") || !strings.Contains(request.Body, `"activity_label":"焼肉"`) {
		t.Fatalf("invite create request = %#v", request)
	}
}

func TestCreateInviteRejectsInvalidDate(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathInvites, `{"invitee_user_id":"`+otherUserID+`","scheduled_date":"2026/05/23"}`))

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

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathAdminNotifications, body))

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

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathAdminNotifications, `{"title":"Title","message":"Message","recipient_user_ids":["bad"]}`))

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

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, contracts.APIPathMePushToken, tc.body))

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestAdminAccessAllowsConfiguredAdminEmailCaseInsensitive(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "USER@EXAMPLE.COM").ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathAdminMe, ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contracts.APIPathMeProfile, `{"display_name":"Name"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contractPath(contracts.APIPathInvite, "id", inviteID), `{"status":"accepted"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, contracts.APIPathMePushToken, `{"token":"device-token","platform":"ios"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, contracts.APIPathMePushToken, `{"token":"device-token"}`))

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
	router.ServeHTTP(invalid, authedRequest(http.MethodPatch, contracts.APIPathMeProfile, `{"user_id":"bad user id"}`))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d body = %s", invalid.Code, invalid.Body.String())
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedRequest(http.MethodPatch, contracts.APIPathMeProfile, `{"user_id":"valid_user","display_name":"Name"}`))
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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, contracts.APIPathMeProfile, `{"user_id":" valid_user ","display_name":" Name ","character_key":"","avatar_url":" avatar.png "}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contracts.APIPathNotificationsReadAll, `{}`))

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

	router.ServeHTTP(w, authedRequest(http.MethodGet, contracts.APIPathAdminMe, ""))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestAdminDeleteUserRejectsSelfDelete(t *testing.T) {
	fake := newFakeSupabase(t, nil)
	w := httptest.NewRecorder()

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodDelete, contractPath(contracts.APIPathAdminUser, "id", testUserID), ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodDelete, contracts.APIPathMeAccount, ""))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contractPath(contracts.APIPathFriendRequest, "id", testRequestID), `{"status":"accepted"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contractPath(contracts.APIPathFriendRequest, "id", testRequestID), `{"status":"rejected"}`))

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

			testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contractPath(contracts.APIPathInvite, "id", inviteID), `{"status":"`+tc.status+`"}`))

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

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathAdminNotifications, body))

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
		{name: "display name length", body: `{"display_name":"` + strings.Repeat("名", 41) + `"}`, want: "display_name must be 1-40 characters"},
		{name: "avatar length", body: `{"avatar_url":"` + strings.Repeat("x", 4097) + `"}`, want: "avatar_url is too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, authedRequest(http.MethodPatch, contracts.APIPathMeProfile, tc.body))
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

	testRouter(fake, "user@example.com").ServeHTTP(w, authedRequest(http.MethodPost, contracts.APIPathAdminNotifications, `{"title":"お知らせ","message":"本文","recipient_user_ids":["`+testUserID+`"]}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPut, contracts.APIPathMePushToken, `{"token":"device-token","platform":"ios"}`))

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

	testRouter(fake).ServeHTTP(w, authedRequest(http.MethodPatch, contracts.APIPathNotificationsReadAll, `{}`))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "notification-secret") || strings.Contains(body, "raw upstream detail") {
		t.Fatalf("raw upstream body leaked: %s", body)
	}
}
