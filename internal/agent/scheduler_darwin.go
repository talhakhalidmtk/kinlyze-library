//go:build darwin

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchdLabel identifies the LaunchAgent job. cron is not used on macOS:
// it has no reliable way to catch up a run missed while the machine was
// off/asleep (no @reboot-equivalent guarantee), which launchd's RunAtLoad
// gives natively.
const launchdLabel = "com.kinlyze.agent"

// installScheduler writes a LaunchAgent plist that runs `kinlyze agent run`
// weekdays at 9am (StartCalendarInterval) and also once whenever the agent
// is (re)loaded — at login or right after this install (RunAtLoad) — so a
// run missed because the machine was off catches up as soon as it's back
// on. Run()'s own once-per-day check keeps the two triggers from
// double-scanning on a normal day.
func installScheduler() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not determine kinlyze's install path: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	logDir := filepath.Join(home, ".kinlyze")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return "", fmt.Errorf("could not create %s: %w", logDir, err)
	}

	plistDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(plistDir, 0755); err != nil {
		return "", fmt.Errorf("could not create %s: %w", plistDir, err)
	}
	plistPath := filepath.Join(plistDir, launchdLabel+".plist")

	plist := fmt.Sprintf(plistTemplate,
		launchdLabel, exePath,
		filepath.Join(logDir, "agent.stdout.log"), filepath.Join(logDir, "agent.stderr.log"))
	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return "", fmt.Errorf("could not write launch agent plist: %w", err)
	}

	uid := fmt.Sprintf("gui/%d", os.Getuid())
	// Unload any previously installed version first; ignore errors, since
	// it's fine (and expected on a first install) if nothing was loaded.
	exec.Command("launchctl", "bootout", uid, plistPath).Run()

	var stderr strings.Builder
	cmd := exec.Command("launchctl", "bootstrap", uid, plistPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not load launch agent: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return fmt.Sprintf("launchd (%s) — weekdays 9am, plus catch-up at login", plistPath), nil
}

// uninstallScheduler unloads and removes the LaunchAgent installed by
// installScheduler.
func uninstallScheduler() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	plistPath := filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")

	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return "no kinlyze agent launch agent was installed", nil
	}

	uid := fmt.Sprintf("gui/%d", os.Getuid())
	// Best-effort: it's fine if this fails because the job isn't loaded.
	exec.Command("launchctl", "bootout", uid, plistPath).Run()

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("could not remove %s: %w", plistPath, err)
	}
	return fmt.Sprintf("unloaded and removed %s", plistPath), nil
}

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>agent</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StartCalendarInterval</key>
	<array>
		<dict><key>Weekday</key><integer>1</integer><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
		<dict><key>Weekday</key><integer>2</integer><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
		<dict><key>Weekday</key><integer>3</integer><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
		<dict><key>Weekday</key><integer>4</integer><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
		<dict><key>Weekday</key><integer>5</integer><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
	</array>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`
