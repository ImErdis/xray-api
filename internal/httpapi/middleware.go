package httpapi

import (
	"net/http"
	"strings"
)

// authMiddleware enforces admin API-key auth via Authorization: Bearer <key>
// or X-API-Key. The subscription routes are mounted outside this middleware.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := extractKey(r)
		if !s.apiKeys.Verify(r.Context(), key, s.bootstrapKey) {
			writeJSON(w, http.StatusUnauthorized, errorBody{
				Error: errorDetail{Code: "unauthorized", Message: "missing or invalid API key"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractKey(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if after, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}
