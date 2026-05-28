package httpapi

import (
	"net/http"
	"testing"
)

func TestInvalidFCMPushTokenResponse(t *testing.T) {
	for _, body := range []string{
		`{"error":{"status":"UNREGISTERED"}}`,
		`{"error":{"status":"SENDER_ID_MISMATCH"}}`,
		`{"error":{"status":"INVALID_ARGUMENT","message":"The registration token is not a valid FCM registration token"}}`,
	} {
		if !isInvalidFCMPushTokenResponse(http.StatusBadRequest, body) {
			t.Fatalf("body should be invalid token: %s", body)
		}
	}
	if isInvalidFCMPushTokenResponse(http.StatusUnauthorized, `{"error":{"status":"THIRD_PARTY_AUTH_ERROR"}}`) {
		t.Fatal("auth/config errors should not disable user tokens")
	}
}
