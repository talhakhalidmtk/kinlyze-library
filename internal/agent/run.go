package agent

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/talhakhalidmtk/kinlyze-library/internal/auth"
	"github.com/talhakhalidmtk/kinlyze-library/internal/git"
	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
	"github.com/talhakhalidmtk/kinlyze-library/internal/upload"
)

// Scan defaults for unattended runs — same as the CLI's own flag defaults
// (see addSharedFlags in cmd/root.go).
const (
	scanDays        = 365
	scanMinCommits  = 2
	scanExcludeBots = true
)

// Install validates token against the Dashboard, saves it locally (reusing
// the same credentials file as `kinlyze login`), registers the OS scheduler
// entry that will invoke `kinlyze agent run` weekdays at 9am (plus a
// boot/login catch-up trigger — see installScheduler), and runs an initial
// scan immediately rather than waiting for the first scheduled trigger.
func Install(token string) (string, error) {
	if _, err := ListRepos(token); err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			return "", fmt.Errorf("invalid token — get a new one from your Kinlyze Dashboard")
		case errors.Is(err, ErrPaymentRequired):
			return "", fmt.Errorf("no active Pro/Team plan — see https://kinlyze.com/pricing")
		default:
			return "", fmt.Errorf("could not reach the Kinlyze Dashboard: %w", err)
		}
	}

	if err := auth.SaveToken(token); err != nil {
		return "", fmt.Errorf("could not save credentials: %w", err)
	}

	location, err := installScheduler()
	if err != nil {
		return "", fmt.Errorf("could not register scheduler: %w", err)
	}

	// Best-effort: Run() logs its own outcome, so a failure here doesn't
	// fail the install — the next scheduled/catch-up trigger will retry.
	_ = Run()

	return location, nil
}

// Uninstall removes the OS scheduler entry registered by Install and clears
// the local run-tracking state, so a future Install runs its initial scan
// again instead of thinking today's run already happened. The saved
// Dashboard token is left alone — it's shared with `kinlyze login`, and
// `kinlyze logout` is the way to remove it.
func Uninstall() (string, error) {
	location, err := uninstallScheduler()
	if err != nil {
		return "", fmt.Errorf("could not remove scheduler entry: %w", err)
	}
	clearState()
	return location, nil
}

// Run fetches the registered repos from the Dashboard and scans each one,
// uploading and reporting the outcome back per repo. It never lets one
// repo's failure stop the others. Errors returned here are the ones that
// abort the entire run (no token, revoked token, lapsed plan) — everything
// else is logged and synced per-repo instead.
//
// Run can now be triggered more than once on a given day: besides the 9am
// timer, a boot/login trigger fires so a run missed because the machine
// was off catches up as soon as it's back on (see installScheduler for each
// OS). The weekday + once-per-day checks below keep that from turning into
// duplicate scans or weekend runs.
func Run() error {
	if !isScheduledWeekday(time.Now()) {
		Logf("skipped: not a scheduled weekday")
		return nil
	}
	if alreadyRanToday() {
		Logf("skipped: already ran today")
		return nil
	}

	token, err := auth.LoadToken()
	if err != nil {
		Logf("no saved token — run 'kinlyze agent install --token <TOKEN>' first")
		return fmt.Errorf("not logged in")
	}

	repos, err := ListRepos(token)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			Logf("token was revoked; run 'kinlyze agent install --token <TOKEN>' again")
			return fmt.Errorf("invalid or revoked token")
		case errors.Is(err, ErrPaymentRequired):
			Logf("plan lapsed; see https://kinlyze.com/pricing")
			return fmt.Errorf("no active Pro/Team plan")
		default:
			// Not marked as done today: a network hiccup here should let a
			// later trigger the same day retry the whole batch.
			Logf("could not fetch repo list: %s", err)
			return err
		}
	}

	// Marked now, before the per-repo loop: individual repo failures below
	// aren't retried by a same-day catch-up trigger, only a top-level
	// Dashboard failure (handled above) is.
	markRanToday()

	for _, repo := range repos {
		runOneRepo(token, repo)
	}
	return nil
}

// isScheduledWeekday reports whether t falls on a day the agent is meant to
// scan (Monday-Friday). The boot/login catch-up trigger can fire on any
// day, so Run() needs this guard even though the timed trigger never fires
// outside weekdays.
func isScheduledWeekday(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	default:
		return true
	}
}

// runOneRepo scans a single repo and reports the outcome. A panic (from
// scoring or upload) is recovered and reported as an error rather than
// aborting the rest of the batch — this loop runs unattended.
func runOneRepo(token string, repo Repo) {
	defer func() {
		if r := recover(); r != nil {
			reportOutcome(token, repo, "error", fmt.Sprintf("panic during scan: %v", r))
		}
	}()

	repoRoot, err := resolveRepoRoot(repo.RepoPath)
	if err != nil {
		reportOutcome(token, repo, "error", err.Error())
		return
	}

	result, err := scoring.AnalyzeRepo(repoRoot, scanDays, scanMinCommits, scanExcludeBots, nil, nil)
	if err != nil {
		reportOutcome(token, repo, "error", err.Error())
		return
	}

	if err := upload.SendForAgent(token, repo.RepoName, repo.ProjectName, result); err != nil {
		reportOutcome(token, repo, "error", err.Error())
		return
	}

	reportOutcome(token, repo, "success", "")
}

// reportOutcome syncs a repo's scan outcome to the Dashboard and logs it
// locally. If the sync itself fails, that's logged too — otherwise a
// Dashboard hiccup would leave the only unattended feedback loop silent.
func reportOutcome(token string, repo Repo, status, message string) {
	if err := SyncStatus(token, repo.RepoName, status, message); err != nil {
		Logf("%s: could not report status to Dashboard: %s", repo.RepoName, err)
	}
	if status == "success" {
		Logf("%s: success", repo.RepoName)
		return
	}
	Logf("%s: error - %s", repo.RepoName, message)
}

// resolveRepoRoot validates repoPath the same way the interactive CLI does
// (cmd.runAnalysis) but returns an error instead of exiting the process.
func resolveRepoRoot(repoPath string) (string, error) {
	if repoPath == "" {
		return "", fmt.Errorf("repo path is empty")
	}
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("repo path does not exist: %s", repoPath)
	}
	if !git.IsGitRepo(repoPath) {
		return "", fmt.Errorf("not a git repository: %s", repoPath)
	}
	repoRoot, err := git.GetRepoRoot(repoPath)
	if err != nil {
		return "", fmt.Errorf("could not determine repo root: %w", err)
	}
	return repoRoot, nil
}
