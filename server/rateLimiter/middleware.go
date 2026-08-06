package rateLimiter

import (
	"net/http"
	"peerfinder/kauth"
	"time"
)

func WithRateLimiter(h func(w http.ResponseWriter, r *http.Request, s *kauth.AuthenticationInfo),
	resetTime time.Duration, countLimit int) func(w http.ResponseWriter, r *http.Request, s *kauth.AuthenticationInfo) {
	limiter := NewRateLimiter[string](resetTime, 1000, countLimit)
	return func(w http.ResponseWriter, r *http.Request, s *kauth.AuthenticationInfo) {
		if !limiter.RateLimitOK(s.ASN) {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		h(w, r, s)
	}
}
