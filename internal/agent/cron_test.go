package agent

import "testing"

func TestMergeCronJobs(t *testing.T) {
	jobs := []cronJob{
		{marker: dailyCronMarker, entry: "0 9 * * 1-5 /usr/local/bin/kinlyze agent run"},
		{marker: rebootCronMarker, entry: "@reboot /usr/local/bin/kinlyze agent run"},
	}

	t.Run("empty crontab", func(t *testing.T) {
		got := mergeCronJobs("", jobs)
		want := "# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /usr/local/bin/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /usr/local/bin/kinlyze agent run\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("preserves unrelated entries", func(t *testing.T) {
		current := "0 3 * * * /usr/bin/backup.sh\n"
		got := mergeCronJobs(current, jobs)
		want := "0 3 * * * /usr/bin/backup.sh\n" +
			"# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /usr/local/bin/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /usr/local/bin/kinlyze agent run\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("re-install replaces old entries, not duplicates", func(t *testing.T) {
		current := "0 3 * * * /usr/bin/backup.sh\n" +
			"# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /old/path/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /old/path/kinlyze agent run\n"
		newJobs := []cronJob{
			{marker: dailyCronMarker, entry: "0 9 * * 1-5 /new/path/kinlyze agent run"},
			{marker: rebootCronMarker, entry: "@reboot /new/path/kinlyze agent run"},
		}
		got := mergeCronJobs(current, newJobs)
		want := "0 3 * * * /usr/bin/backup.sh\n" +
			"# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /new/path/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /new/path/kinlyze agent run\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("uninstall (nil jobs) strips kinlyze entries but keeps others", func(t *testing.T) {
		current := "0 3 * * * /usr/bin/backup.sh\n" +
			"# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /usr/local/bin/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /usr/local/bin/kinlyze agent run\n"
		got := mergeCronJobs(current, nil)
		want := "0 3 * * * /usr/bin/backup.sh\n"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("uninstall (nil jobs) on a crontab with only kinlyze entries leaves it blank", func(t *testing.T) {
		current := "# kinlyze-agent-daily\n" +
			"0 9 * * 1-5 /usr/local/bin/kinlyze agent run\n" +
			"# kinlyze-agent-reboot\n" +
			"@reboot /usr/local/bin/kinlyze agent run\n"
		got := mergeCronJobs(current, nil)
		if got != "\n" {
			t.Fatalf("got %q, want a blank crontab", got)
		}
	})
}
