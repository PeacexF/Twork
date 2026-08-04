package web

import (
	"crypto/subtle"
	"net/http"
)

// wraps h with HTTP Basic Auth, constant-time compared against username/password.
// The dashboard can add/remove chats and trigger sends, so every route (including
// static assets) goes through this -- config.go's validation already refuses to
// enable the web server without a username and password set.
func basicAuth(username, password string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="twork"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
