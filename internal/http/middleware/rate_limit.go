package middleware

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RateLimitConfig はレート制限の設定を保持する。
type RateLimitConfig struct {
	PerMinuteLimit int
	VerifyLimit    int
}

type rateCounter struct {
	Count     int
	WindowEnd time.Time
}

// InMemoryRateLimiter はMVP向けの固定窓レート制限実装。
type InMemoryRateLimiter struct {
	mu       sync.Mutex
	counters map[string]rateCounter
	now      func() time.Time
}

func NewInMemoryRateLimiter() *InMemoryRateLimiter {
	return &InMemoryRateLimiter{counters: map[string]rateCounter{}, now: time.Now}
}

func (l *InMemoryRateLimiter) allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	now := l.now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.counters[key]
	if !ok || now.After(entry.WindowEnd) {
		l.counters[key] = rateCounter{Count: 1, WindowEnd: now.Add(window)}
		return true
	}
	if entry.Count >= limit {
		return false
	}
	entry.Count++
	l.counters[key] = entry
	return true
}

// RateLimit はIP+エンドポイント単位で制御する。
func RateLimit(limiter *InMemoryRateLimiter, cfg RateLimitConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := cfg.PerMinuteLimit
			endpoint := r.Method + " " + r.URL.Path
			if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/verify") && strings.Contains(r.URL.Path, "/v1/access/") {
				limit = cfg.VerifyLimit
				endpoint = "POST /v1/access/{token}/verify"
			}
			ip := clientIP(r)
			key := ip + "|" + endpoint
			if !limiter.allow(key, limit, time.Minute) {
				reqID := chimw.GetReqID(r.Context())
				log.Printf("event=rate_limit_hit request_id=%s ip=%s endpoint=%s limit=%d", reqID, ip, endpoint, limit)
				w.Header().Set("Retry-After", "60")
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
