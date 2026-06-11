package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipLimiter is a per-client-IP token-bucket limiter for the public endpoints.
// Buckets are pruned after an idle period so memory stays bounded.
type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rate    rate.Limit
	burst   int
}

type ipBucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

const ipBucketTTL = 10 * time.Minute

// newIPLimiter allows perMinute requests/min per IP (burst = perMinute, so a
// client can fetch its full minute allowance at once). nil when disabled.
func newIPLimiter(perMinute int) *ipLimiter {
	if perMinute <= 0 {
		return nil
	}
	return &ipLimiter{
		buckets: make(map[string]*ipBucket),
		rate:    rate.Limit(float64(perMinute) / 60.0),
		burst:   perMinute,
	}
}

func (l *ipLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[ip]
	if !ok {
		b = &ipBucket{lim: rate.NewLimiter(l.rate, l.burst)}
		l.buckets[ip] = b
	}
	b.lastSeen = time.Now()
	if len(l.buckets) > 10000 {
		l.pruneLocked()
	}
	return b.lim.Allow()
}

func (l *ipLimiter) pruneLocked() {
	cutoff := time.Now().Add(-ipBucketTTL)
	for ip, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}

// rateLimitMiddleware enforces the public-endpoint limit. With RealIP applied
// earlier in the chain, RemoteAddr reflects X-Forwarded-For behind a proxy.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	if s.limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !s.limiter.allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{
				Error: errorDetail{Code: "rate_limited", Message: "too many requests"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
