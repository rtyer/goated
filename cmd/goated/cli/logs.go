package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"goated/internal/app"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon and system logs",
	Long: `View daemon and system logs.

Without a subcommand, shows the last 50 lines of the daemon log (signal only, no Slack socket noise).

Examples:
  goated logs                    # last 50 lines of daemon signal
  goated logs -f                 # tail -f daemon signal (live)
  goated logs -n 200             # last 200 lines of daemon signal
  goated logs raw                # last 100 lines unfiltered
  goated logs raw -f             # tail -f unfiltered (everything)
  goated logs restarts           # recent restart history
  goated logs cron               # recent cron run log
  goated logs watchdog           # watchdog log`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogsSignal(cmd)
	},
}

var logsRawCmd = &cobra.Command{
	Use:   "raw",
	Short: "Show unfiltered daemon log (includes Slack socket noise)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := app.LoadConfig()
		logPath := filepath.Join(cfg.LogDir, "goated_daemon.log")

		follow, _ := cmd.Flags().GetBool("follow")
		n, _ := cmd.Flags().GetInt("lines")

		if follow {
			return tailFollow(logPath)
		}
		return tailLines(logPath, n)
	},
}

var logsRestartsCmd = &cobra.Command{
	Use:   "restarts",
	Short: "Show restart history",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := app.LoadConfig()
		logPath := filepath.Join(cfg.LogDir, "restarts.jsonl")
		n, _ := cmd.Flags().GetInt("lines")
		return tailLines(logPath, n)
	},
}

var logsCronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Show cron run log",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := app.LoadConfig()
		logPath := filepath.Join(cfg.LogDir, "cron", "runs.jsonl")
		n, _ := cmd.Flags().GetInt("lines")
		return tailLines(logPath, n)
	},
}

var logsWatchdogCmd = &cobra.Command{
	Use:   "watchdog",
	Short: "Show watchdog log",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := app.LoadConfig()
		logPath := filepath.Join(cfg.LogDir, "watchdog.log")
		n, _ := cmd.Flags().GetInt("lines")
		return tailLines(logPath, n)
	},
}

// signalPattern matches useful daemon log lines while excluding Slack socket
// chatter and raw JSON payloads. Lines must start at column 0 (no leading
// whitespace) to avoid matching inside indented JSON blobs.
const signalPattern = `^\[|^slack:|^slack attachments|^slack-api:.*Sending|^goated |^ERROR|^WARN`

func runLogsSignal(cmd *cobra.Command) error {
	cfg := app.LoadConfig()
	logPath := filepath.Join(cfg.LogDir, "goated_daemon.log")

	if _, err := os.Stat(logPath); err != nil {
		return fmt.Errorf("daemon log not found: %s", logPath)
	}

	follow, _ := cmd.Flags().GetBool("follow")
	n, _ := cmd.Flags().GetInt("lines")

	if follow {
		// tail -f | grep --line-buffered
		c := exec.Command("bash", "-c",
			fmt.Sprintf("tail -f %q | grep --line-buffered -E %q", logPath, signalPattern))
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}

	// grep + tail -n
	c := exec.Command("bash", "-c",
		fmt.Sprintf("grep -E %q %q | tail -n %d", signalPattern, logPath, n))
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func tailLines(path string, n int) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("log not found: %s", path)
	}
	c := exec.Command("tail", "-n", fmt.Sprintf("%d", n), path)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func tailFollow(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("log not found: %s", path)
	}
	c := exec.Command("tail", "-f", path)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func init() {
	logsCmd.Flags().BoolP("follow", "f", false, "follow log output (tail -f)")
	logsCmd.Flags().IntP("lines", "n", 50, "number of lines to show")

	logsRawCmd.Flags().BoolP("follow", "f", false, "follow log output (tail -f)")
	logsRawCmd.Flags().IntP("lines", "n", 100, "number of lines to show")

	logsRestartsCmd.Flags().IntP("lines", "n", 20, "number of lines to show")
	logsCronCmd.Flags().IntP("lines", "n", 30, "number of lines to show")
	logsWatchdogCmd.Flags().IntP("lines", "n", 30, "number of lines to show")

	logsCmd.AddCommand(logsRawCmd)
	logsCmd.AddCommand(logsRestartsCmd)
	logsCmd.AddCommand(logsCronCmd)
	logsCmd.AddCommand(logsWatchdogCmd)
	rootCmd.AddCommand(logsCmd)
}
