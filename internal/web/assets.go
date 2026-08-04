package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// serves the built React app, falling back to index.html for any path that
// isn't a real static file -- client-side routing needs this
func staticHandler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("internal/web: embedded dist directory is missing: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if _, err := fs.Stat(sub, path); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
