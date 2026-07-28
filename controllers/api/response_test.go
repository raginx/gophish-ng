package api

import (
	"net/http/httptest"
	"testing"
)

// TestJSONResponseNoStore verifies that API responses tell the browser not
// to cache per-user data - otherwise a stale response can be served after
// switching accounts in the same browser (ref gophish/gophish#2022).
func TestJSONResponseNoStore(t *testing.T) {
	w := httptest.NewRecorder()
	JSONResponse(w, map[string]string{"foo": "bar"}, 200)
	got := w.Header().Get("Cache-Control")
	want := "no-store"
	if got != want {
		t.Fatalf("unexpected Cache-Control header: got %q, want %q", got, want)
	}
}
