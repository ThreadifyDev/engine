// Package dashboard serves the CI-built UI without a JavaScript runtime.
package dashboard

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed dist
var assets embed.FS

// Wrap keeps public application assets outside API authentication and metering.
// Data, authentication, and WebSocket requests always pass through unchanged.
func Wrap(api http.Handler) http.Handler {
	root, _ := fs.Sub(assets, "dist")
	return serve(root, api)
}

func appPath(path string) bool {
	switch path {
	case "/", "/login", "/signup", "/cli-login", "/pricing", "/u",
		"/auth/forgot-password", "/auth/reset-password", "/auth/verify-otp":
		return true
	}
	return strings.HasPrefix(path, "/u/")
}

func serve(files fs.FS, api http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		shell := appPath(path)
		asset := strings.HasPrefix(path, "/assets/") || path == "/favicon-black.svg" || path == "/favicon-white.svg" || path == "/favicon-black.png" || path == "/favicon-white.png" || path == "/robots.txt"
		if !shell && !asset {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path, "/")
		if shell {
			name = "index.html"
		}
		if !fs.ValidPath(name) || strings.Contains(path, "\\") {
			http.NotFound(w, r)
			return
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			if shell {
				http.Error(w, "Dashboard assets are unavailable in this development build. Use a release binary or the dashboard dev server.", http.StatusServiceUnavailable)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}
