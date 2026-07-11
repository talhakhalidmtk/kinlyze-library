package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	original := baseURL
	baseURL = server.URL
	t.Cleanup(func() { baseURL = original })
}

func TestListRepos(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"repos":[{"id":"1","repo_path":"/x/api","repo_name":"api","project_name":"default","schedule":"weekdays"}]}`))
		})
		repos, err := ListRepos("tok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repos) != 1 || repos[0].RepoName != "api" {
			t.Fatalf("unexpected repos: %+v", repos)
		}
	})

	t.Run("401 maps to ErrUnauthorized", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		_, err := ListRepos("bad-tok")
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("got %v, want ErrUnauthorized", err)
		}
	})

	t.Run("402 maps to ErrPaymentRequired", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusPaymentRequired)
		})
		_, err := ListRepos("tok")
		if !errors.Is(err, ErrPaymentRequired) {
			t.Fatalf("got %v, want ErrPaymentRequired", err)
		}
	})

	t.Run("no apikey header is sent", func(t *testing.T) {
		var sawAPIKey bool
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("apikey") != "" {
				sawAPIKey = true
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true,"repos":[]}`))
		})
		if _, err := ListRepos("tok"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sawAPIKey {
			t.Fatalf("expected no apikey header to be sent")
		}
	})
}

func TestSyncStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		})
		if err := SyncStatus("tok", "api", "success", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("non-2xx surfaces the response's error field", func(t *testing.T) {
		withTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"ok":false,"error":"plan cap reached"}`))
		})
		err := SyncStatus("tok", "api", "error", "boom")
		if err == nil || err.Error() != "plan cap reached" {
			t.Fatalf("got %v, want \"plan cap reached\"", err)
		}
	})
}
