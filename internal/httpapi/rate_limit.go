package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type rateLimitPolicy struct {
	Action string
	Limit  int
	Window time.Duration
}

type rateLimitBucket struct {
	Count     int
	ResetAt   time.Time
	UpdatedAt time.Time
}

type actionRateLimiter struct {
	mu          sync.Mutex
	now         func() time.Time
	buckets     map[string]rateLimitBucket
	lastCleanup time.Time
}

var timeNow = time.Now

var (
	rateLimitReportDrinkLog = rateLimitPolicy{
		Action: "drink_log_report",
		Limit:  10,
		Window: time.Hour,
	}
	rateLimitCreateDrinkInvite = rateLimitPolicy{
		Action: "drink_invite_create",
		Limit:  20,
		Window: time.Hour,
	}
	rateLimitCreateFriendRequest = rateLimitPolicy{
		Action: "friend_request_create",
		Limit:  20,
		Window: time.Hour,
	}
	rateLimitCreateUploadURL = rateLimitPolicy{
		Action: "media_upload_url_create",
		Limit:  30,
		Window: time.Hour,
	}
	rateLimitBlockUser = rateLimitPolicy{
		Action: "user_block_create",
		Limit:  30,
		Window: time.Hour,
	}
	rateLimitMuteUser = rateLimitPolicy{
		Action: "user_mute_create",
		Limit:  60,
		Window: time.Hour,
	}
)

func newActionRateLimiter(now func() time.Time) *actionRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &actionRateLimiter{
		now:     now,
		buckets: map[string]rateLimitBucket{},
	}
}

func (l *actionRateLimiter) Allow(userID string, policy rateLimitPolicy) (bool, time.Duration) {
	if l == nil || policy.Limit <= 0 || policy.Window <= 0 {
		return true, 0
	}
	now := l.now().UTC()
	key := userID + ":" + policy.Action

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) > 10*time.Minute {
		l.cleanup(now)
		l.lastCleanup = now
	}

	bucket := l.buckets[key]
	if bucket.ResetAt.IsZero() || !now.Before(bucket.ResetAt) {
		bucket = rateLimitBucket{
			Count:     0,
			ResetAt:   now.Add(policy.Window),
			UpdatedAt: now,
		}
	}
	if bucket.Count >= policy.Limit {
		retryAfter := bucket.ResetAt.Sub(now)
		if retryAfter <= 0 {
			retryAfter = time.Second
		}
		return false, retryAfter
	}
	bucket.Count++
	bucket.UpdatedAt = now
	l.buckets[key] = bucket
	return true, 0
}

func (l *actionRateLimiter) cleanup(now time.Time) {
	for key, bucket := range l.buckets {
		if !bucket.ResetAt.IsZero() && !now.Before(bucket.ResetAt.Add(time.Minute)) {
			delete(l.buckets, key)
		}
	}
}

func (r *router) enforceRateLimit(w http.ResponseWriter, req *http.Request, policy rateLimitPolicy) bool {
	userID := req.Header.Get("X-Nomo-User-ID")
	allowed, retryAfter := r.rateLimiter.Allow(userID, policy)
	if allowed {
		return true
	}
	seconds := int(retryAfter.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]string{
		"error":             "rate limit exceeded",
		"rate_limit_action": policy.Action,
		"retry_after":       fmt.Sprintf("%ds", seconds),
	})
	return false
}
