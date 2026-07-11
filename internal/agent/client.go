// Package agent implements the Kinlyze Agent: an unattended scheduler that
// scans a Dashboard-registered set of repos on a cron/Task Scheduler trigger
// and syncs the results back, with no user present to answer prompts.
package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// baseURL is a var (not const) so tests can point it at an httptest.Server.
var baseURL = "https://gwhhwqvpfrhrbyltkotm.supabase.co/functions/v1"

// ErrUnauthorized means the token was rejected (missing/revoked).
var ErrUnauthorized = errors.New("invalid or revoked token")

// ErrPaymentRequired means the account has no active Pro/Team plan.
var ErrPaymentRequired = errors.New("no active Pro/Team plan")

// Repo describes one repository registered for scheduled scanning on the
// Dashboard, as returned by agent-repos-list.
type Repo struct {
	ID          string `json:"id"`
	RepoPath    string `json:"repo_path"`
	RepoName    string `json:"repo_name"`
	ProjectName string `json:"project_name"`
	Schedule    string `json:"schedule"`
}

type apiResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

type reposListResponse struct {
	OK    bool   `json:"ok"`
	Repos []Repo `json:"repos"`
	Error string `json:"error"`
}

type syncStatusRequest struct {
	RepoName     string `json:"repo_name"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func post(token, path string, payload any) (statusCode int, body []byte, err error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/"+path, bytes.NewReader(data))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, buf.Bytes(), nil
}

// ListRepos fetches the repos registered for scheduled scanning. Returns
// ErrUnauthorized or ErrPaymentRequired for 401/402 respectively.
func ListRepos(token string) ([]Repo, error) {
	statusCode, body, err := post(token, "agent-repos-list", struct{}{})
	if err != nil {
		return nil, err
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusPaymentRequired:
		return nil, ErrPaymentRequired
	}

	var parsed reposListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("could not parse agent-repos-list response: %w", err)
	}
	if statusCode != http.StatusOK || !parsed.OK {
		if parsed.Error != "" {
			return nil, fmt.Errorf("%s", parsed.Error)
		}
		return nil, fmt.Errorf("unexpected response (%d)", statusCode)
	}
	return parsed.Repos, nil
}

// SyncStatus reports the outcome of a repo scan back to the Dashboard.
// status is "success" or "error"; errorMessage is only meaningful for "error".
func SyncStatus(token, repoName, status, errorMessage string) error {
	statusCode, body, err := post(token, "agent-sync-status", syncStatusRequest{
		RepoName:     repoName,
		Status:       status,
		ErrorMessage: errorMessage,
	})
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		var parsed apiResponse
		_ = json.Unmarshal(body, &parsed) // best-effort; a non-JSON body just leaves Error empty
		if parsed.Error != "" {
			return fmt.Errorf("%s", parsed.Error)
		}
		return fmt.Errorf("unexpected response (%d)", statusCode)
	}
	return nil
}
