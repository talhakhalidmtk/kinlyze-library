//go:build linux

package agent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// installScheduler registers two cron entries: the 9am weekday scan, and an
// @reboot entry that catches up a run missed because the machine was off.
// Both use this binary's absolute path (cron's PATH can't be relied on to
// resolve a bare "kinlyze"). Re-running replaces any previously installed
// entries rather than duplicating them; Run()'s own once-per-day check
// keeps the two triggers from double-scanning on a normal day.
func installScheduler() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not determine kinlyze's install path: %w", err)
	}

	if _, err := exec.LookPath("crontab"); err != nil {
		return "", fmt.Errorf("cron not found on this system — install cron, or add a scheduler entry manually to run '%s agent run' weekdays at 9am", exePath)
	}

	current, err := currentCrontab()
	if err != nil {
		return "", fmt.Errorf("could not read current crontab: %w", err)
	}

	jobs := []cronJob{
		{marker: dailyCronMarker, entry: fmt.Sprintf("0 9 * * 1-5 %s agent run", exePath)},
		{marker: rebootCronMarker, entry: fmt.Sprintf("@reboot %s agent run", exePath)},
	}
	updated := mergeCronJobs(current, jobs)

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(updated)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not install crontab: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return "crontab (weekdays 9am, plus catch-up on boot)", nil
}

// uninstallScheduler removes the cron entries installed by installScheduler,
// leaving any unrelated crontab entries untouched. If removing them empties
// the crontab entirely, it clears the crontab rather than installing a
// blank one (some crontab implementations reject empty stdin).
func uninstallScheduler() (string, error) {
	if _, err := exec.LookPath("crontab"); err != nil {
		return "no crontab entries to remove (crontab not found)", nil
	}

	current, err := currentCrontab()
	if err != nil {
		return "", fmt.Errorf("could not read current crontab: %w", err)
	}
	if !strings.Contains(current, cronMarkerPrefix) {
		return "no kinlyze agent entries were installed", nil
	}

	updated := mergeCronJobs(current, nil)
	if strings.TrimSpace(updated) == "" {
		if err := exec.Command("crontab", "-r").Run(); err != nil {
			return "", fmt.Errorf("could not clear crontab: %w", err)
		}
		return "removed crontab entries", nil
	}

	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(updated)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not update crontab: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return "removed crontab entries", nil
}

// currentCrontab returns the user's existing crontab, or "" if none exists
// yet (crontab -l exits non-zero with "no crontab for user" in that case).
func currentCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if strings.Contains(strings.ToLower(string(exitErr.Stderr)), "no crontab") {
				return "", nil
			}
		}
		return "", nil // treat any other failure to read as "no existing crontab"
	}
	return string(out), nil
}
