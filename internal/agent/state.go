package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// The agent can now be triggered more than once on the same day — the
// regular 9am timer, plus a boot/login trigger added so a run missed
// because the machine was off catches up as soon as it's back on. This
// state file is how Run() tells those triggers apart from a genuine
// same-day re-run: it records the local date of the last attempt that
// successfully reached the Dashboard, so only the first trigger each
// weekday actually scans.
func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kinlyze", "agent-state.json"), nil
}

type agentState struct {
	LastRunDate string `json:"last_run_date"` // local date, YYYY-MM-DD
}

func today() string {
	return time.Now().Format("2006-01-02")
}

// alreadyRanToday reports whether a run has already reached the Dashboard
// today (local date). A missing/unreadable state file counts as "no".
func alreadyRanToday() bool {
	path, err := statePath()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var s agentState
	if err := json.Unmarshal(data, &s); err != nil {
		return false
	}
	return s.LastRunDate == today()
}

// markRanToday records that a run reached the Dashboard today, so any
// later trigger this same weekday is skipped. Best-effort: a failure to
// write must never abort the scan that's already in progress.
func markRanToday() {
	path, err := statePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.Marshal(agentState{LastRunDate: today()})
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0600)
}

// clearState removes the run-tracking state file, if present, so a future
// `agent install` doesn't think today's run already happened and skip its
// initial scan. Best-effort: uninstalling the scheduler should succeed even
// if this fails.
func clearState() {
	path, err := statePath()
	if err != nil {
		return
	}
	os.Remove(path)
}
