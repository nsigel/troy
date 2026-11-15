// Package web holds the built web app. `pnpm build` writes it to dist/app; without
// that, the Go build still works and the app answers 404.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// App serves the built app: its files as they are, and index.html for any other path,
// so the app can route. Paths under /api/, /sign-in/ and /sendblue/ belong to the
// server and get 404.
func App() http.Handler {
	app, _ := fs.Sub(dist, "dist/app")
	files := http.FileServerFS(app)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		for _, prefix := range []string{"/api/", "/s/", "/sign-in/", "/sendblue/"} {
			if strings.HasPrefix(p+"/", prefix) {
				http.NotFound(w, r)
				return
			}
		}
		if f, err := fs.Stat(app, strings.TrimPrefix(p, "/")); err == nil && !f.IsDir() {
			if strings.HasPrefix(p, "/assets/") { // names carry a content hash
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(app, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
