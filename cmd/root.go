// Package cmd defines the kinlyze command-line interface.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/talhakhalidmtk/kinlyze-library/internal/git"
	"github.com/talhakhalidmtk/kinlyze-library/internal/renderer"
	"github.com/talhakhalidmtk/kinlyze-library/internal/scoring"
)

var version = "0.3.0" // set by ldflags at build time

var (
	flagRepo          string
	flagDays          int
	flagTop           int
	flagMinCommits    int
	flagNoColor       bool
	flagJSON          bool
	flagExcludeBots   bool
	flagExcludeEmails []string
)

// ── Shared analysis helper ────────────────────────────────────────────────────

// runAnalysis resolves the repo, runs the full scoring pipeline, and returns
// the result. Used by every command/subcommand to avoid duplicating setup logic.
func runAnalysis(cmd *cobra.Command) *scoring.Result {
	// Apply flags
	if flagNoColor || flagJSON {
		os.Setenv("NO_COLOR", "1")
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
	repoPath, err := filepath.Abs(flagRepo)
	if err != nil {
		renderer.PrintError(fmt.Sprintf("Invalid path: %s", flagRepo))
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
  kinlyze insights --repo ./myapp    Key insights only
  kinlyze heatmap --top 20           Top 20 riskiest modules
  kinlyze developers --days 180      Developer profiles (last 6 months)
  kinlyze flows                      User flow risk analysis
  kinlyze --json | jq '.insights'    JSON output for scripting
  kinlyze --json | jq '.maturity'    Check repo maturity classification`,

	SilenceUsage:  true,
	SilenceErrors: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.Render(result, flagTop)
		return nil
	},
}

// ── Subcommands ───────────────────────────────────────────────────────────────

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Full scan — all sections (same as running kinlyze with no args)",
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.Render(result, flagTop)
		return nil
	},
}

var insightsCmd = &cobra.Command{
	Use:   "insights",
	Short: "Key insights and risk alerts — the executive summary",
	Long: `Shows repository-wide patterns and actionable risk alerts.
This is the highest-signal view: maturity classification, monoculture
detection, noise vs runtime risk breakdown, and grouped alerts.

Best for: quick health checks, CI pipeline reports, sharing with leadership.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderInsights(result)
		renderer.RenderAlerts(result)
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
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderHeatmap(result, flagTop)
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
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderBusFactor(result)
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
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderDeveloperProfiles(result)
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
		result := runAnalysis(cmd)

		if flagJSON {
			return renderer.RenderJSON(result)
		}
		renderer.RenderSummaryOnly(result)
		renderer.RenderFlowRisk(result)
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

// ── Flag registration ─────────────────────────────────────────────────────────

func addSharedFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flagRepo, "repo", "r", ".", "Path to git repository")
	cmd.Flags().IntVarP(&flagDays, "days", "d", 365, "Days of history to analyze")
	cmd.Flags().IntVarP(&flagTop, "top", "t", 0, "Show only top N riskiest modules (0 = all)")
	cmd.Flags().IntVar(&flagMinCommits, "min-commits", 2, "Minimum commits for a file to be included")
	cmd.Flags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "Output raw JSON (full result for all subcommands)")
	cmd.Flags().BoolVar(&flagExcludeBots, "no-bots", true, "Exclude bot/CI commits")
	cmd.Flags().StringSliceVar(&flagExcludeEmails, "exclude-emails", nil, "Comma-separated emails to exclude")
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

	// Register subcommands
	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(insightsCmd)
	rootCmd.AddCommand(heatmapCmd)
	rootCmd.AddCommand(busfactorCmd)
	rootCmd.AddCommand(developersCmd)
	rootCmd.AddCommand(flowsCmd)
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		renderer.PrintError(err.Error())
		os.Exit(1)
	}
}
