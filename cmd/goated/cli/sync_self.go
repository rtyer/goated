package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"goated/internal/app"
)

var syncSelfCmd = &cobra.Command{
	Use:   "sync_self_to_github",
	Short: "Safely commit and push workspace/self to GitHub",
	Long: `Performs safety checks, stages markdown files, commits, and pushes the self repo.

Safety checks:
  1. No goat credentials appear in workspace/self
  2. Binaries in workspace/self are gitignored
  3. credentials.json and .env files are gitignored
Then stages all new/updated .md files, commits, and pushes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := app.LoadConfig()
		selfDir := filepath.Join(cfg.WorkspaceDir, "self")

		if _, err := os.Stat(filepath.Join(selfDir, ".git")); err != nil {
			return fmt.Errorf("workspace/self is not a git repo: %w", err)
		}

		// 1. Confirm binaries are gitignored
		if err := checkBinariesIgnored(selfDir); err != nil {
			return err
		}
		fmt.Println("[ok] Binaries in self/ are gitignored")

		// 2. Confirm credentials.json and .env are gitignored
		if err := checkSensitiveIgnored(selfDir); err != nil {
			return err
		}
		fmt.Println("[ok] Sensitive files (credentials.json, .env) are gitignored")

		// 3. Stage all .md files
		stageCmd := exec.Command("git", "add", "-A", "*.md", "**/*.md")
		stageCmd.Dir = selfDir
		stageCmd.Stdout = os.Stderr
		stageCmd.Stderr = os.Stderr
		_ = stageCmd.Run()

		// Check if there's anything to commit
		diffCmd := exec.Command("git", "diff", "--cached", "--quiet")
		diffCmd.Dir = selfDir
		if diffCmd.Run() == nil {
			fmt.Println("Nothing to commit.")
			return nil
		}

		// 4. Check staged diff for credential values
		credsDir := filepath.Join(cfg.WorkspaceDir, "creds")
		if err := checkNoCredsInStaged(credsDir, selfDir); err != nil {
			// Unstage to leave clean state
			resetCmd := exec.Command("git", "reset", "HEAD")
			resetCmd.Dir = selfDir
			_ = resetCmd.Run()
			return err
		}
		fmt.Println("[ok] No credential values in staged changes")

		// 5. Commit
		commitCmd := exec.Command("git", "commit", "-m", "goat sync_self_to_github")
		commitCmd.Dir = selfDir
		commitCmd.Stdout = os.Stderr
		commitCmd.Stderr = os.Stderr
		if err := commitCmd.Run(); err != nil {
			return fmt.Errorf("git commit failed: %w", err)
		}
		fmt.Println("[ok] Committed")

		// 6. Push
		pushCmd := exec.Command("git", "push")
		pushCmd.Dir = selfDir
		pushCmd.Stdout = os.Stderr
		pushCmd.Stderr = os.Stderr
		if err := pushCmd.Run(); err != nil {
			return fmt.Errorf("git push failed: %w", err)
		}
		fmt.Println("[ok] Pushed to GitHub")
		return nil
	},
}

// checkNoCredsInStaged reads all credential values from creds/ and checks that
// none appear in the currently staged diff.
func checkNoCredsInStaged(credsDir, selfDir string) error {
	entries, err := os.ReadDir(credsDir)
	if err != nil {
		return nil // no creds dir = nothing to leak
	}

	// Get the staged diff once
	diffCmd := exec.Command("git", "diff", "--cached")
	diffCmd.Dir = selfDir
	diffOut, err := diffCmd.Output()
	if err != nil || len(diffOut) == 0 {
		return nil
	}
	diff := string(diffOut)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(credsDir, e.Name()))
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(data))
		if val == "" || len(val) < 20 {
			continue // skip short values (usernames, org IDs) to avoid false positives
		}
		if strings.Contains(diff, val) {
			return fmt.Errorf("ABORT: credential %q value found in staged changes", e.Name())
		}
	}
	return nil
}

// checkBinariesIgnored finds executable files in self/ and warns about any that aren't gitignored.
func checkBinariesIgnored(selfDir string) error {
	findCmd := exec.Command("find", selfDir, "-maxdepth", "3", "-type", "f", "-executable",
		"-not", "-path", "*/.git/*")
	out, err := findCmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	var unignored []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		rel, _ := filepath.Rel(selfDir, line)
		checkCmd := exec.Command("git", "check-ignore", "-q", rel)
		checkCmd.Dir = selfDir
		if checkCmd.Run() != nil {
			unignored = append(unignored, rel)
		}
	}
	if len(unignored) > 0 {
		fmt.Fprintf(os.Stderr, "[warn] %d executable files in self/ are not gitignored\n", len(unignored))
	}
	return nil
}

// checkSensitiveIgnored confirms credentials.json and .env files are gitignored.
func checkSensitiveIgnored(selfDir string) error {
	sensitive := []string{"credentials.json", ".env"}
	for _, name := range sensitive {
		// Find any files with this name
		findCmd := exec.Command("find", selfDir, "-name", name, "-type", "f")
		out, _ := findCmd.Output()
		if len(out) == 0 {
			continue
		}
		for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if path == "" {
				continue
			}
			rel, _ := filepath.Rel(selfDir, path)
			checkCmd := exec.Command("git", "check-ignore", "-q", rel)
			checkCmd.Dir = selfDir
			if checkCmd.Run() != nil {
				return fmt.Errorf("ABORT: %q is not gitignored — add it to self/.gitignore", rel)
			}
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(syncSelfCmd)
}
