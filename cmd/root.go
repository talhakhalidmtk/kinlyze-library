// Package cmd defines the kinlyze command-line interface.
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/talhakhalidmtk/kinlyze-library/internal/auth"
	"github.com/talhakhalidmtk/kinlyze-library/internal/git"
	"github.com/talhakhalidmtk/kinlyze-library/internal/renderer"
	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
	"github.com/talhakhalidmtk/kinlyze-library/internal/upload"
)

var version = "0.3.0" // set by ldflags at build time

var (
	flagRepo          string   // single-repo commands (insights, heatmap, busfactor, developers, flows)
	flagRepos         []string // root/scan: comma-separated, supports multiple repos
	flagDiscoverPath  string   // root/scan: search this path's subdirectories for git repos
	flagDays          int
	flagTop           int
	flagMinCommits    int
	flagNoColor       bool
	flagJSON          bool
	flagExcludeBots   bool
	flagExcludeEmails []string
	flagLoginToken    string
)

// isLoggedIn reports whether a Dashboard token is saved locally.
func isLoggedIn() bool {
	_, err := auth.LoadToken()
	return err == nil
}

// maybeSyncReport uploads the report if a Dashboard token is saved. Sync is
// best-effort: a failed or missing token never blocks or fails local report
// generation, and status goes to stderr so it doesn't corrupt a redirected
// `kinlyze --json > report.json`.
func maybeSyncReport(result *scoring.Result) {
	if token, err := auth.LoadToken(); err == nil {
		upload.Report(token, result.RepoInfo.Name, result)
	}
}

// outputJSON prints the JSON report and syncs it to the Dashboard if logged in.
func outputJSON(result *scoring.Result) error {
	if err := renderer.RenderJSON(result); err != nil {
		return err
	}
	maybeSyncReport(result)
	return nil
}

// outputJSONMulti prints a JSON array of reports (one per repo) and syncs
// each to the Dashboard if logged in.
func outputJSONMulti(results []*scoring.Result) error {
	if err := renderer.RenderJSONMulti(results); err != nil {
		return err
	}
	for _, result := range results {
		maybeSyncReport(result)
	}
	return nil
}

// ── Shared analysis helper ────────────────────────────────────────────────────

// runAnalysis resolves repoInput, runs the full scoring pipeline, and returns
// the result. Used by every command/subcommand to avoid duplicating setup logic.
func runAnalysis(repoInput string) *scoring.Result {
	// Apply flags
	if flagNoColor || flagJSON {
		_ = os.Setenv("NO_COLOR", "1")
	}

	// Validate
	if flagDays < 1 {
		renderer.PrintError("--days must be a positive integer.")
		os.Exit(1)
	}
	if flagMinCommits < 1 {
		renderer.PrintError("--min-commits must be at least 1.")
		os.Exit(1)
	}
	if flagDays > 3650 {
		renderer.PrintWarning("--days > 3650 may be slow on large repositories.")
	}

	// Resolve repo path
	repoPath, err := filepath.Abs(repoInput)
	if err != nil {
		renderer.PrintError(fmt.Sprintf("Invalid path: %s", repoInput))
		os.Exit(1)
	}
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		renderer.PrintError(fmt.Sprintf("Path does not exist: %s", repoPath))
		os.Exit(1)
	}
	if !git.IsGitRepo(repoPath) {
		renderer.PrintError(fmt.Sprintf(
			"Not a git repository: %s\n  Run this command inside a git repo, or use --repo <path>.",
			repoPath,
		))
		os.Exit(1)
	}

	repoRoot, err := git.GetRepoRoot(repoPath)
	if err != nil {
		renderer.PrintError(fmt.Sprintf("Could not determine repo root: %s", err))
		os.Exit(1)
	}

	// Progress output (suppressed in JSON mode)
	var progressFn func(string)
	if !flagJSON {
		fmt.Printf("\n  Scanning %s...\n\n", repoRoot)
		progressFn = renderer.PrintProgress
	}

	result, err := scoring.AnalyzeRepo(
		repoRoot, flagDays, flagMinCommits,
		flagExcludeBots, flagExcludeEmails, progressFn,
	)
	if err != nil {
		renderer.PrintError(err.Error())
		os.Exit(1)
	}

	return result
}

// ── Root command (full scan) ──────────────────────────────────────────────────

var rootCmd = &cobra.Command{
	Use:   "kinlyze",
	Short: "Analyze the kin behind your code",
	Long: `Kinlyze maps knowledge concentration risk in any Git repository.
Find your bus factor before someone quits and takes it with them.

No source code is read. Only Git metadata (author emails, dates, file paths, lines changed).
Everything runs locally — nothing is sent anywhere.

Run 'kinlyze' for a full scan, or use subcommands for specific sections:
  kinlyze scan         Full scan (same as running kinlyze with no args)
  kinlyze insights     Key insights and risk alerts only
  kinlyze heatmap      Knowledge heat map
  kinlyze busfactor    Bus factor deep dive
  kinlyze developers   Developer departure impact profiles
  kinlyze flows        User flow risk analysis`,

	Example: `  kinlyze                            Full scan of current directory
  kinlyze --repo a,b,c               Full scan of multiple repos (requires login)
  kinlyze insights --repo ./myapp    Key insights only
  kinlyze heatmap --top 20           Top 20 riskiest modules
  kinlyze developers --days 180      Developer profiles (last 6 months)
  kinlyze flows                      User flow risk analysis
  kinlyze --json | jq '.insights'    JSON output for scripting
  kinlyze --json | jq '.maturity'    Check repo maturity classification`,

	SilenceUsage:  true,
	SilenceErrors: true,

	RunE: runScan,
}

// ── Subcommands ───────────────────────────────────────────────────────────────

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Full scan — all sections (same as running kinlyze with no args)",
	RunE:  runScan,
}

// runScan powers both the root command and 'kinlyze scan'. Unlike the other
// subcommands, it supports multiple repos: pass --repo a,b,c, or omit --repo
// entirely to get a numbered picker over git repos found in subdirectories
// of the current directory (when the current directory isn't a repo itself).
func runScan(cmd *cobra.Command, args []string) error {
	repoPaths, err := resolveRepoPaths()
	if err != nil {
		renderer.PrintError(err.Error())
		os.Exit(1)
	}

	if flagJSON {
		if len(repoPaths) == 1 {
			result := runAnalysis(repoPaths[0])
			return outputJSON(result)
		}
		var results []*scoring.Result
		for _, repoPath := range repoPaths {
			results = append(results, runAnalysis(repoPath))
		}
		return outputJSONMulti(results)
	}

	for i, repoPath := range repoPaths {
		result := runAnalysis(repoPath)
		renderer.Render(result, flagTop, isLoggedIn())
		maybeSyncReport(result)
		if i < len(repoPaths)-1 {
			fmt.Println()
		}
	}
	return nil
}

// errLoginRequired is returned when a Dashboard-only feature (repo discovery
// or multi-repo scanning) is used without a saved token.
func errLoginRequired(feature string) error {
	return fmt.Errorf(
		"%s requires a Kinlyze Dashboard account.\n  Run 'kinlyze login --token <TOKEN>' to enable it, or scan a single repo without this flag",
		feature,
	)
}

// resolveRepoPaths returns the repo paths to scan.
//   - --discover <path> always searches <path>'s immediate subdirectories
//     for git repos and prompts the user to pick which to scan. Requires login.
//   - Otherwise, --repo (comma-separated) is used as-is if given. Passing more
//     than one repo requires login.
//   - Otherwise, if the current directory is itself a git repo, that's the
//     only target — same as before.
//   - Otherwise, if git repos are found in immediate subdirectories of the
//     current directory, the user is prompted to pick which ones to scan.
//     This auto-discovery also requires login.
func resolveRepoPaths() ([]string, error) {
	if flagDiscoverPath != "" {
		if len(flagRepos) > 0 {
			return nil, fmt.Errorf("--discover cannot be combined with --repo")
		}
		if !isLoggedIn() {
			return nil, errLoginRequired("--discover")
		}
		root, err := filepath.Abs(flagDiscoverPath)
		if err != nil {
			return nil, err
		}
		repos, err := discoverRepos(root)
		if err != nil {
			return nil, err
		}
		if len(repos) == 0 {
			return nil, fmt.Errorf("no git repos found in subdirectories of %s", root)
		}
		return promptRepoSelection(repos)
	}

	if len(flagRepos) > 0 {
		if len(flagRepos) > 1 && !isLoggedIn() {
			return nil, errLoginRequired("scanning multiple repos (--repo a,b,c)")
		}
		return flagRepos, nil
	}

	cwd, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	if git.IsGitRepo(cwd) {
		return []string{"."}, nil
	}

	if !isLoggedIn() {
		// Fall through to "." so runAnalysis reports the usual
		// "not a git repository" error; repo auto-discovery is a
		// Dashboard-only feature.
		return []string{"."}, nil
	}

	repos, err := discoverRepos(cwd)
	if err != nil || len(repos) == 0 {
		// Fall through to "." so runAnalysis reports the usual
		// "not a git repository" error.
		return []string{"."}, nil
	}

	return promptRepoSelection(repos)
}

// discoverRepos returns immediate subdirectories of root that are git repos.
func discoverRepos(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var repos []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(root, e.Name())
		if git.IsGitRepo(path) {
			repos = append(repos, path)
		}
	}
	sort.Strings(repos)
	return repos, nil
}

// promptRepoSelection lists discovered repos and asks the user to choose.
func promptRepoSelection(repos []string) ([]string, error) {
	fmt.Println("\n  No git repository here. Found these repos in subdirectories:")
	for i, r := range repos {
		fmt.Printf("    %d) %s\n", i+1, filepath.Base(r))
	}
	fmt.Print("\n  Select repos to scan (e.g. 1,3 or 'all'): ")

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("no repos selected")
	}
	if strings.EqualFold(line, "all") {
		return repos, nil
	}

	var selected []string
	for _, tok := range strings.Split(line, ",") {
		tok = strings.TrimSpace(tok)
		n, err := strconv.Atoi(tok)
		if err != nil || n < 1 || n > len(repos) {
			return nil, fmt.Errorf("invalid selection: %q", tok)
		}
		selected = append(selected, repos[n-1])
	}
	return selected, nil
}

var insightsCmd = &cobra.Command{
	Use:   "insights",
	Short: "Key insights and risk alerts — the executive summary",
	Long: `Shows repository-wide patterns and actionable risk alerts.
This is the highest-signal view: maturity classification, monoculture
detection, noise vs runtime risk breakdown, and grouped alerts.

Best for: quick health checks, CI pipeline reports, sharing with leadership.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(flagRepo)

		if flagJSON {
			return outputJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderInsights(result)
		renderer.RenderAlerts(result)
		renderer.RenderFooter(isLoggedIn())
		return nil
	},
}

var heatmapCmd = &cobra.Command{
	Use:   "heatmap",
	Short: "Knowledge heat map — every module ranked by concentration risk",
	Long: `Shows every module in the codebase ranked by knowledge concentration risk.
Modules are color-coded: red = critical (BF=1), orange = high, yellow = medium, green = healthy.
Low-impact modules (examples, templates, docs) are dimmed to reduce noise.

Best for: identifying which specific modules need attention.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(flagRepo)

		if flagJSON {
			return outputJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderHeatmap(result, flagTop)
		renderer.RenderFooter(isLoggedIn())
		return nil
	},
}

var busfactorCmd = &cobra.Command{
	Use:     "busfactor",
	Aliases: []string{"bf"},
	Short:   "Bus factor deep dive — at-risk modules grouped by owner",
	Long: `Shows modules with bus factor ≤ 2 grouped by their primary owner.
Reveals who is a single point of failure and which modules break if they leave.

Best for: identifying which people to prioritize for knowledge transfer.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(flagRepo)

		if flagJSON {
			return outputJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderBusFactor(result)
		renderer.RenderFooter(isLoggedIn())
		return nil
	},
}

var developersCmd = &cobra.Command{
	Use:     "developers",
	Aliases: []string{"devs"},
	Short:   "Developer profiles — knowledge departure impact per engineer",
	Long: `Shows what percentage of total codebase knowledge each developer holds,
what happens if they leave, which modules they solely own, and which user flows
they control end-to-end.

Best for: team planning, hiring decisions, onboarding prioritization.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(flagRepo)

		if flagJSON {
			return outputJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderDeveloperProfiles(result)
		renderer.RenderFooter(isLoggedIn())
		return nil
	},
}

var flowsCmd = &cobra.Command{
	Use:   "flows",
	Short: "User flow risk — does one person own an entire feature end-to-end?",
	Long: `Groups related modules into user flows (authentication, payments, API, etc.)
and computes a flow-level bus factor. Shows whether a single person controls
an entire user journey from endpoint to database.

Best for: detecting feature-level dependency risk that module-level analysis misses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(flagRepo)

		if flagJSON {
			return outputJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderFlowRisk(result)
		renderer.RenderFooter(isLoggedIn())
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("kinlyze %s\n", version)
	},
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save a Dashboard token so scans sync automatically",
	Long: `Log in with a token via 'kinlyze login --token <TOKEN>'.
Get a token from your Kinlyze Dashboard.

The token is saved to ~/.kinlyze/credentials (mode 0600). Once logged in,
just run 'kinlyze scan' (or any command with --json) and the report lands
on your Dashboard automatically. Run 'kinlyze logout' to remove the saved
token.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagLoginToken == "" {
			renderer.PrintError("--token is required. Get one from your Kinlyze Dashboard.")
			os.Exit(1)
		}
		if err := auth.SaveToken(flagLoginToken); err != nil {
			renderer.PrintError(fmt.Sprintf("Could not save credentials: %s", err))
			os.Exit(1)
		}
		fmt.Println("\n  ✓ Logged in. Future scans will sync to your Kinlyze Dashboard.")
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove the saved Dashboard token",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := auth.DeleteToken(); err != nil {
			renderer.PrintError(fmt.Sprintf("Could not remove credentials: %s", err))
			os.Exit(1)
		}
		fmt.Println("\n  ✓ Logged out. Scans are fully local again.")
		return nil
	},
}

// ── Flag registration ─────────────────────────────────────────────────────────

func addSharedFlags(cmd *cobra.Command) {
	cmd.Flags().IntVarP(&flagDays, "days", "d", 365, "Days of history to analyze")
	cmd.Flags().IntVarP(&flagTop, "top", "t", 0, "Show only top N riskiest modules (0 = all)")
	cmd.Flags().IntVar(&flagMinCommits, "min-commits", 2, "Minimum commits for a file to be included")
	cmd.Flags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output raw JSON (full result for all subcommands)")
	cmd.Flags().BoolVar(&flagExcludeBots, "no-bots", true, "Exclude bot/CI commits")
	cmd.Flags().StringSliceVar(&flagExcludeEmails, "exclude-emails", nil, "Comma-separated emails to exclude")
}

// addSingleRepoFlag registers --repo for commands that only ever operate on
// one repository at a time.
func addSingleRepoFlag(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flagRepo, "repo", "r", ".", "Path to git repository")
}

// addMultiRepoFlag registers --repo and --discover for root/scan.
// --repo can take multiple comma-separated paths; --discover searches a
// given path's subdirectories for git repos and prompts the user to pick.
func addMultiRepoFlag(cmd *cobra.Command) {
	cmd.Flags().StringSliceVarP(&flagRepos, "repo", "r", nil, "Path(s) to git repositories, comma-separated. Multiple repos requires being logged in. If omitted and the current directory isn't a repo, you'll be prompted to pick from repos found in subdirectories (requires login).")
	cmd.Flags().StringVar(&flagDiscoverPath, "discover", "", "Search this path's subdirectories for git repos and prompt to select which to scan (requires login)")
}

func init() {
	// Shared flags on root and all subcommands
	addSharedFlags(rootCmd)
	addSharedFlags(scanCmd)
	addSharedFlags(insightsCmd)
	addSharedFlags(heatmapCmd)
	addSharedFlags(busfactorCmd)
	addSharedFlags(developersCmd)
	addSharedFlags(flowsCmd)

	addMultiRepoFlag(rootCmd)
	addMultiRepoFlag(scanCmd)
	addSingleRepoFlag(insightsCmd)
	addSingleRepoFlag(heatmapCmd)
	addSingleRepoFlag(busfactorCmd)
	addSingleRepoFlag(developersCmd)
	addSingleRepoFlag(flowsCmd)

	loginCmd.Flags().StringVar(&flagLoginToken, "token", "", "Dashboard API token")

	// Register subcommands
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(insightsCmd)
	rootCmd.AddCommand(heatmapCmd)
	rootCmd.AddCommand(busfactorCmd)
	rootCmd.AddCommand(developersCmd)
	rootCmd.AddCommand(flowsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		renderer.PrintError(err.Error())
		os.Exit(1)
	}
}
