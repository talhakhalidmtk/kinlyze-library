package agent

import "strings"

// cronMarkerPrefix tags every comment line kinlyze installs directly above
// one of its crontab entries, so a re-install can find and replace the
// whole managed block instead of appending duplicates. It must not go on
// the same line as the command: cron only treats '#' as a comment when
// it's the first character of the line, so trailing "cmd # comment" would
// be passed to the command literally.
const cronMarkerPrefix = "# kinlyze-agent"

const (
	dailyCronMarker  = cronMarkerPrefix + "-daily"  // the 9am weekday entry
	rebootCronMarker = cronMarkerPrefix + "-reboot" // catch-up entry: run once at boot
)

type cronJob struct {
	marker string
	entry  string
}

// mergeCronJobs returns current with any previously-installed kinlyze-agent
// marker+entry pairs removed, followed by the given fresh jobs. current may
// be empty (no crontab yet).
func mergeCronJobs(current string, jobs []cronJob) string {
	var kept []string
	if current != "" {
		lines := strings.Split(current, "\n")
		for i := 0; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], cronMarkerPrefix) {
				i++ // also drop the entry line right after the marker
				continue
			}
			if lines[i] == "" {
				continue
			}
			kept = append(kept, lines[i])
		}
	}
	for _, j := range jobs {
		kept = append(kept, j.marker, j.entry)
	}
	return strings.Join(kept, "\n") + "\n"
}
