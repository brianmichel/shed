package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerServesIndexWithoutRedirect(t *testing.T) {
	for _, path := range []string{"/ui/", "/ui/index.html", "/ui/deep/link"} {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		Handler(true).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status=%d", path, rr.Code)
		}
		if location := rr.Header().Get("Location"); location != "" {
			t.Fatalf("%s unexpectedly redirected to %s", path, location)
		}
		if rr.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s missing csp", path)
		}
	}
}
