package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersAllowFnOSDesktopIframe(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if value := recorder.Header().Get("X-Frame-Options"); value != "" {
		t.Fatalf("X-Frame-Options must be omitted for the fnOS cross-port iframe, got %q", value)
	}
	policy := recorder.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "frame-ancestors *") {
		t.Fatalf("Content-Security-Policy must explicitly allow the fnOS desktop iframe, got %q", policy)
	}
}
