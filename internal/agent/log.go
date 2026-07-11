package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// logPath returns ~/.kinlyze/agent.log, the local record of unattended run
// outcomes — the only feedback loop available since agent run has no user
// present to read stdout/stderr.
func logPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kinlyze", "agent.log"), nil
}

// Logf appends a timestamped line to the agent log. Best-effort: a logging
// failure must never abort a scan, so errors are silently dropped.
func Logf(format string, args ...any) {
	path, err := logPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}
