package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/talhakhalidmtk/kinlyze-library/internal/agent"
)

var flagAgentToken string

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Automated scheduled scans (Pro/Team)",
	Long: `Kinlyze Agent scans a Dashboard-registered set of repos on a fixed
schedule with no one present, and syncs the results automatically.

  kinlyze agent install --token <TOKEN>   One-time setup: saves the token,
                                           registers an OS scheduler entry
                                           (launchd on macOS, cron on Linux,
                                           Task Scheduler on Windows) that
                                           runs 'kinlyze agent run' weekdays
                                           at 9am, and runs an initial scan
                                           right away. A run missed because
                                           the machine was off catches up
                                           automatically at the next boot/
                                           login (at most once per weekday).
  kinlyze agent run                       Invoked by that schedule. Not meant
                                           to be run interactively.
  kinlyze agent uninstall                 Removes the scheduler entry. Run
                                           'agent install' again afterward to
                                           re-enable scheduled scans.`,
}

var agentInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Save a Dashboard token, register the scheduler, and run an initial scan",
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagAgentToken == "" {
			return fmt.Errorf("--token is required. Get one from your Kinlyze Dashboard")
		}
		location, err := agent.Install(flagAgentToken)
		if err != nil {
			return err
		}
		fmt.Printf("\n  ✓ Agent installed. Scheduled via %s.\n", location)
		fmt.Println("  Running an initial scan now — see ~/.kinlyze/agent.log for the result.")
		return nil
	},
}

var agentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run one scheduled scan pass (invoked by the OS scheduler, not meant for interactive use)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return agent.Run()
	},
}

var agentUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the scheduler entry installed by 'agent install'",
	Long: `Removes the OS scheduler entry (launchd/cron/Task Scheduler) so
'kinlyze agent run' no longer runs automatically. The saved Dashboard token
is left in place — run 'kinlyze logout' separately if you want to remove
that too. Run 'kinlyze agent install --token <TOKEN>' again at any time to
re-enable scheduled scans.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		location, err := agent.Uninstall()
		if err != nil {
			return err
		}
		fmt.Printf("\n  ✓ %s\n", location)
		fmt.Println("  Run 'kinlyze agent install --token <TOKEN>' again to re-enable scheduled scans.")
		return nil
	},
}

func init() {
	agentInstallCmd.Flags().StringVar(&flagAgentToken, "token", "", "Dashboard API token")

	agentCmd.AddCommand(agentInstallCmd)
	agentCmd.AddCommand(agentRunCmd)
	agentCmd.AddCommand(agentUninstallCmd)
	rootCmd.AddCommand(agentCmd)
}
