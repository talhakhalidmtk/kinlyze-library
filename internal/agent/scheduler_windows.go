//go:build windows

package agent

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const taskName = "KinlyzeAgent"

// installScheduler registers a Task Scheduler task with two triggers: the
// 9am weekday scan, and an at-startup trigger (plus StartWhenAvailable) so
// a run missed because the machine was off catches up as soon as it's back
// on. Run()'s own once-per-day check keeps the two triggers from
// double-scanning on a normal day.
//
// schtasks.exe's plain CLI flags only support one trigger per task, so this
// shells out to PowerShell's Register-ScheduledTask, which supports
// multiple triggers and StartWhenAvailable directly. /F-equivalent (-Force)
// means re-running Install replaces any previously installed task.
func installScheduler() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not determine kinlyze's install path: %w", err)
	}

	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$action = New-ScheduledTaskAction -Execute %s -Argument 'agent run'
$weekly = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday,Tuesday,Wednesday,Thursday,Friday -At 9:00am
$onStart = New-ScheduledTaskTrigger -AtStartup
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable
Register-ScheduledTask -TaskName %s -Action $action -Trigger @($weekly,$onStart) -Settings $settings -Force | Out-Null
`, psQuote(exePath), psQuote(taskName))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not create Task Scheduler task: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return fmt.Sprintf("Task Scheduler task %q — weekdays 9am, plus catch-up at startup", taskName), nil
}

// uninstallScheduler removes the Task Scheduler task installed by
// installScheduler, if present.
func uninstallScheduler() (string, error) {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
if (Get-ScheduledTask -TaskName %s -ErrorAction SilentlyContinue) {
	Unregister-ScheduledTask -TaskName %s -Confirm:$false
	Write-Output 'removed'
} else {
	Write-Output 'not-found'
}
`, psQuote(taskName), psQuote(taskName))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not remove Task Scheduler task: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if strings.Contains(stdout.String(), "not-found") {
		return "no kinlyze agent task was installed", nil
	}
	return fmt.Sprintf("removed Task Scheduler task %q", taskName), nil
}

// psQuote wraps s in PowerShell single quotes, doubling any embedded quotes
// (the PowerShell escaping convention for single-quoted strings).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
