package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dist/*
var assets embed.FS

const StubHTML = `<!doctype html><html><body><h1>Shed UI disabled</h1><p>This binary was configured to serve the API without the embedded operator UI.</p></body></html>`

func Handler(enabled bool) http.Handler {
	if !enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(StubHTML)) })
	}
	sub, _ := fs.Sub(assets, "dist")
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		name := strings.TrimPrefix(r.URL.Path, "/ui/")
		name = path.Clean("/" + name)[1:]
		if name == "." || name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(sub, name); err != nil || name == "index.html" {
			serveIndex(w, sub)
			return
		}
		r.URL.Path = "/" + name
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, filesystem fs.FS) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	b, err := fs.ReadFile(filesystem, "index.html")
	if err != nil {
		http.Error(w, "embedded UI index missing", http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}
