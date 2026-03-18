// Package cmd defines the kinlyze command-line interface.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kinlyze/kinlyze/internal/git"
	"github.com/kinlyze/kinlyze/internal/renderer"
	"github.com/kinlyze/kinlyze/internal/scoring"
	"github.com/spf13/cobra"
)

var version = "0.1.0" // set by ldflags at build time

var (
	flagRepo       string
	flagDays       int
	flagTop        int
	flagMinCommits int
	flagNoColor    bool
	flagJSON       bool
)

var rootCmd = &cobra.Command{
	Use:   "kinlyze",
	Short: "Analyze the kin behind your code",
	Long: `Kinlyze maps knowledge concentration risk in any Git repository.
Find your bus factor before someone quits and takes it with them.

No source code is read. Only Git metadata (author emails, dates, file paths).
Everything runs locally — nothing is sent anywhere.`,

	Example: `  kinlyze                            Analyze current directory
  kinlyze --repo /path/to/repo        Analyze a specific repo
  kinlyze --days 180                  Last 6 months only
  kinlyze --top 20                    Show top 20 riskiest modules
  kinlyze --min-commits 5             Filter noise (files touched <5 times)
  kinlyze --no-color > report.txt     Plain text output
  kinlyze --json | jq '.summary'      Raw JSON for scripting`,

	SilenceUsage:  true,
	SilenceErrors: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		// Apply flags
		if flagNoColor || flagJSON {
			os.Setenv("NO_COLOR", "1")
		}

		// Validate flags
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

		// Check path exists
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			renderer.PrintError(fmt.Sprintf("Path does not exist: %s", repoPath))
			os.Exit(1)
		}

		// Check it's a git repo
		if !git.IsGitRepo(repoPath) {
			renderer.PrintError(fmt.Sprintf(
				"Not a git repository: %s\n  Run this command inside a git repo, or use --repo <path>.",
				repoPath,
			))
			os.Exit(1)
		}

		// Get repo root (handles running from a subdirectory)
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

		// Run analysis
		result, err := scoring.AnalyzeRepo(repoRoot, flagDays, flagMinCommits, progressFn)
		if err != nil {
			renderer.PrintError(err.Error())
			os.Exit(1)
		}

		// Output
		if flagJSON {
			if err := renderer.RenderJSON(result); err != nil {
				renderer.PrintError(fmt.Sprintf("JSON encoding failed: %s", err))
				os.Exit(1)
			}
		} else {
			renderer.Render(result, flagTop)
		}

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

func init() {
	rootCmd.Flags().StringVarP(&flagRepo,       "repo",         "r", ".",   "Path to git repository (default: current directory)")
	rootCmd.Flags().IntVarP   (&flagDays,        "days",         "d", 365,   "Days of history to analyze")
	rootCmd.Flags().IntVarP   (&flagTop,         "top",          "t", 0,     "Show only top N riskiest modules (0 = all)")
	rootCmd.Flags().IntVar    (&flagMinCommits,   "min-commits",      2,     "Minimum commits for a file to be included (filters noise)")
	rootCmd.Flags().BoolVar   (&flagNoColor,     "no-color",         false,  "Disable colored output")
	rootCmd.Flags().BoolVar   (&flagJSON,        "json",             false,  "Output raw JSON")

	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		renderer.PrintError(err.Error())
		os.Exit(1)
	}
}
