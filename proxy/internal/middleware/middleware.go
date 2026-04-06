package middleware

import (
	"net/http"

	"github.com/hilthontt/wordle-go/proxy/internal/throttling"
)

const Weight = 1

func ThrottlingMiddleware(throttler *throttling.Throttler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the route key from the request
			route := r.Method + ":" + r.URL.Path

			// Check if the request is allowed
			if !throttler.IncrementAndCheck(route, Weight) {
				// Request is throttled
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte("Rate limit exceeded. Please try again later."))
				return
			}

			// Request is allowed, proceed to the next handler
			next.ServeHTTP(w, r)
		})
	}
}

func HierarchicalThrottlingMiddleware(ht *throttling.HierarchicalThrottler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !ht.CheckRequest(r) {
				http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
