package server_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestGuard(t *testing.T) {
	t.Parallel()
	const jsonType = "application/json"
	tests := []struct {
		name        string
		method      string
		host        string // empty: the test server's own 127.0.0.1 address
		contentType string
		origin      string
		sameOrigin  bool // send Origin: http://<host>
		want        int
	}{
		{name: "foreign host", method: "GET", host: "evil.example:8790", want: http.StatusForbidden},
		{name: "foreign host without port", method: "GET", host: "evil.example", want: http.StatusForbidden},
		{name: "foreign host on JSON POST", method: "POST", host: "evil.example:8790", contentType: jsonType, want: http.StatusForbidden},
		{name: "foreign origin", method: "POST", contentType: jsonType, origin: "http://evil.example", want: http.StatusForbidden},
		{name: "loopback origin on another port", method: "POST", contentType: jsonType, origin: "http://127.0.0.1:1", want: http.StatusForbidden},
		{name: "text/plain POST", method: "POST", contentType: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "POST without content type", method: "POST", want: http.StatusUnsupportedMediaType},
		{name: "loopback JSON POST", method: "POST", contentType: jsonType, want: http.StatusOK},
		{name: "same-origin JSON POST with charset", method: "POST", contentType: jsonType + "; charset=utf-8", sameOrigin: true, want: http.StatusOK},
		{name: "localhost GET", method: "GET", host: "localhost:8790", want: http.StatusOK},
		{name: "localhost GET without port", method: "GET", host: "localhost", want: http.StatusOK},
		{name: "IPv6 loopback GET", method: "GET", host: "[::1]:8790", want: http.StatusOK},
		{name: "GET ignores a foreign origin", method: "GET", origin: "http://evil.example", want: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := kitchenSinkServer(t)
			path, body := "/api/state", ""
			if tc.method == http.MethodPost {
				body = `{"items":{}}`
			}
			req := s.request(t, tc.method, path, body)
			if tc.host != "" {
				req.Host = tc.host
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.sameOrigin {
				req.Header.Set("Origin", "http://"+req.URL.Host)
			}
			resp, got := s.send(t, req)
			wantStatus(t, resp, got, tc.want)
			if tc.method != http.MethodPost || tc.want == http.StatusOK {
				return
			}
			if b, err := s.fb.State(); err != nil || strings.TrimSpace(string(b)) != "{}" {
				t.Errorf("State() after a rejected POST = %q, %v, want {} (nothing saved)", b, err)
			}
		})
	}
}
