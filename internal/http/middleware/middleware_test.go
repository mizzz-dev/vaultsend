package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitBlocksAfterLimit(t *testing.T) {
	limiter := NewInMemoryRateLimiter()
	h := RateLimit(limiter, RateLimitConfig{PerMinuteLimit: 2, VerifyLimit: 1})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/shipments/x", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 got=%d", w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/shipments/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429 got=%d", w.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("unexpected header: %s", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("unexpected header: %s", got)
	}
}
