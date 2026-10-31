package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shradiphylleia/rival/internal/config"
)

func TestRouterErrorsUseJSONEnvelope(t *testing.T) {
	h := NewServer(config.Config{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()

	cases := []struct {
		method, path string
		status       int
		code         string
		allow        string
	}{
		{http.MethodGet, "/nope", http.StatusNotFound, "not_found", ""},
		{http.MethodDelete, "/events", http.StatusMethodNotAllowed, "method_not_allowed", "POST"},
		{http.MethodDelete, "/webhooks/abc", http.StatusMethodNotAllowed, "method_not_allowed", "GET, PATCH"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

		if rec.Code != tc.status {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.status)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", tc.method, tc.path, ct)
		}
		if allow := strings.Join(rec.Header().Values("Allow"), ", "); allow != tc.allow {
			t.Errorf("%s %s: Allow = %q, want %q", tc.method, tc.path, allow, tc.allow)
		}
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: body not JSON: %v (%q)", tc.method, tc.path, err, rec.Body.String())
		}
		if body.Error.Code != tc.code {
			t.Errorf("%s %s: code = %q, want %q", tc.method, tc.path, body.Error.Code, tc.code)
		}
	}
}
