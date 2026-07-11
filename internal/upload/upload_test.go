package upload

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	original := endpoint
	endpoint = server.URL
	t.Cleanup(func() { endpoint = original })
}

func TestSendForAgent(t *testing.T) {
	result := &scoring.Result{}

	t.Run("success", func(t *testing.T) {
		var gotProjectName string
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			var body requestBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("could not decode request body: %v", err)
			}
			gotProjectName = body.ProjectName
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"report_id":"1"}`))
		})
		if err := SendForAgent("tok", "api", "default", result); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotProjectName != "default" {
			t.Fatalf("got project_name %q, want %q", gotProjectName, "default")
		}
	})

	t.Run("non-2xx surfaces the response's error field", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"ok":false,"error":"plan cap reached"}`))
		})
		err := SendForAgent("tok", "api", "default", result)
		if err == nil || err.Error() != "plan cap reached" {
			t.Fatalf("got %v, want \"plan cap reached\"", err)
		}
	})

	t.Run("401", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		err := SendForAgent("tok", "api", "default", result)
		if err == nil {
			t.Fatalf("expected an error")
		}
	})
}

