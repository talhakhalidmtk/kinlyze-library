// Package upload syncs generated reports to the Kinlyze Dashboard for
// logged-in users. Sync is additive: offline/local-only remains the default,
// and a failed or errored sync never affects local report generation.
package upload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
)

// endpoint is a var (not const) so tests can point it at an httptest.Server.
var endpoint = "https://gwhhwqvpfrhrbyltkotm.supabase.co/functions/v1/upload-report"

type requestBody struct {
	RepoName    string          `json:"repo_name"`
	ProjectName string          `json:"project_name,omitempty"`
	Report      *scoring.Result `json:"report"`
}

type responseBody struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// send POSTs the report and returns the HTTP status code and decoded
// response body. A non-nil error means the request itself never completed
// (marshal/network failure) — statusCode is meaningless in that case.
func send(token, repoName, projectName string, result *scoring.Result) (statusCode int, resp responseBody, err error) {
	body, err := json.Marshal(requestBody{RepoName: repoName, ProjectName: projectName, Report: result})
	if err != nil {
		return 0, responseBody{}, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, responseBody{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		return 0, responseBody{}, err
	}
	defer func() { _ = httpResp.Body.Close() }()

	data, _ := io.ReadAll(httpResp.Body)
	var parsed responseBody
	_ = json.Unmarshal(data, &parsed) // best-effort; a non-JSON body just leaves Error empty

	return httpResp.StatusCode, parsed, nil
}

// Report POSTs the analysis result to the Dashboard and prints the outcome
// to stderr. It never returns an error — callers should fire-and-forget so
// that a network failure can't block or fail local report generation.
func Report(token, repoName string, result *scoring.Result) {
	statusCode, _, err := send(token, repoName, "", result)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "  Dashboard sync skipped: %s\n", err)
		return
	}

	switch statusCode {
	case http.StatusOK, http.StatusCreated:
		_, _ = fmt.Fprintln(os.Stderr, "  ✓ Report synced to your Kinlyze Dashboard.")
	case http.StatusUnauthorized:
		_, _ = fmt.Fprintln(os.Stderr, "  ✖ Dashboard sync failed: 401 invalid or revoked token. Run 'kinlyze login --token <TOKEN>' again.")
	case http.StatusPaymentRequired:
		_, _ = fmt.Fprintln(os.Stderr, "  ✖ Dashboard sync failed: 402 upgrade to Pro/Team to enable automatic sync.")
	default:
		_, _ = fmt.Fprintf(os.Stderr, "  ✖ Dashboard sync failed: unexpected response (%d).\n", statusCode)
	}
}

// SendForAgent POSTs the analysis result on behalf of an unattended agent run
// and returns an error describing any failure (including the Dashboard's own
// "error" message on a non-2xx response) so the caller can relay it via
// agent-sync-status. Returns nil on success.
func SendForAgent(token, repoName, projectName string, result *scoring.Result) error {
	statusCode, resp, err := send(token, repoName, projectName, result)
	if err != nil {
		return err
	}

	switch statusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("401 invalid or revoked token")
	case http.StatusPaymentRequired:
		return fmt.Errorf("402 no active Pro/Team plan")
	default:
		if resp.Error != "" {
			return fmt.Errorf("%s", resp.Error)
		}
		return fmt.Errorf("unexpected response (%d)", statusCode)
	}
}
