// Package upload syncs generated reports to the Kinlyze Dashboard for
// logged-in users. Sync is additive: offline/local-only remains the default,
// and a failed or errored sync never affects local report generation.
package upload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
)

const endpoint = "https://gefmuqbuhegttunnyynv.supabase.co/functions/v1/upload-report"

type requestBody struct {
	RepoName string          `json:"repo_name"`
	Report   *scoring.Result `json:"report"`
}

// Report POSTs the analysis result to the Dashboard and prints the outcome
// to stderr. It never returns an error — callers should fire-and-forget so
// that a network failure can't block or fail local report generation.
func Report(token, repoName string, result *scoring.Result) {
	body, err := json.Marshal(requestBody{RepoName: repoName, Report: result})
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Dashboard sync skipped: %s\n", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Dashboard sync skipped: %s\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Dashboard sync skipped: %s\n", err)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		fmt.Fprintln(os.Stderr, "  ✓ Report synced to your Kinlyze Dashboard.")
	case http.StatusUnauthorized:
		fmt.Fprintln(os.Stderr, "  ✖ Dashboard sync failed: 401 invalid or revoked token. Run 'kinlyze login --token <TOKEN>' again.")
	case http.StatusPaymentRequired:
		fmt.Fprintln(os.Stderr, "  ✖ Dashboard sync failed: 402 upgrade to Pro/Team to enable automatic sync.")
	default:
		fmt.Fprintf(os.Stderr, "  ✖ Dashboard sync failed: unexpected response (%d).\n", resp.StatusCode)
	}
}
