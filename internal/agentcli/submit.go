// package: agentcli / submit
// type:    command
// job:     the agent's `sindri-worker submit` — rebase, lint, create the PR, hand
//          off and submit the task for review, then return (no blocking wait).
// limits:  PR records live in store; lint via internal/lint; task state via td CLI.
package agentcli

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/ghlocal/store"
	"github.com/flo-at/sindri/internal/lint"
	"github.com/spf13/cobra"
)

var (
	submitTitle string
	submitBody  string
	submitDone  string
)

var submitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit your work: rebase, create PR, handoff, and wait for review",
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()

		branch, err := currentBranch()
		if err != nil {
			return err
		}

		base := baseBranch()
		if branch == base {
			return fmt.Errorf("you are on %s — nothing to submit. Run 'sindri-worker issue next' first", base)
		}

		taskID := branch
		if !taskIDPattern.MatchString(taskID) {
			return fmt.Errorf("branch %q doesn't look like a task ID — expected td-xxx", branch)
		}

		if submitTitle == "" {
			return fmt.Errorf("--title is required")
		}

		// Check for uncommitted changes
		if out, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(out) > 0 {
			return fmt.Errorf("uncommitted changes — commit or stash first:\n%s", strings.TrimSpace(string(out)))
		}

		// Rebase onto base
		if out, err := exec.Command("git", "rebase", base).CombinedOutput(); err != nil {
			return fmt.Errorf("rebase onto %s failed: %s", base, strings.TrimSpace(string(out)))
		}

		// Lint gate: never submit an unlinted PR.
		if err := runLint(cmd.OutOrStdout()); err != nil {
			return err
		}

		// Create PR
		diff, err := gitDiff(base, branch)
		if err != nil {
			return fmt.Errorf("git diff failed: %w", err)
		}

		baseID := "pr-" + taskID
		prID, err := resolveCreateID(baseID)
		if err != nil {
			return err
		}

		pr := &store.PR{
			ID:        prID,
			Branch:    branch,
			Base:      base,
			Status:    "open",
			Title:     submitTitle,
			Body:      submitBody,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Diff:      diff,
		}
		if err := store.Write(pr); err != nil {
			return err
		}
		fmt.Printf("PR created: %s (%s → %s)\n", pr.ID, branch, base)

		// td handoff
		doneMsg := submitDone
		if doneMsg == "" {
			doneMsg = submitTitle
		}
		if out, err := td("handoff", taskID, "--done", doneMsg); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: td handoff failed: %s\n", out)
		}

		// td review
		if out, err := td("review", taskID); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: td review failed: %s\n", out)
		}

		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
		fmt.Println("║  Submitted for review.                                          ║")
		fmt.Println("║  Run 'sindri-worker done' then 'sindri-worker issue next'.      ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
		return nil
	},
}

// runLint runs the project linters (loc + deadcode) against the workspace and
// refuses to proceed if any violation is found, so agents never submit an
// unlinted PR. A linter that cannot run (e.g. the code does not compile) is also
// a hard stop — the agent must fix it before submitting.
func runLint(w io.Writer) error {
	var out strings.Builder

	fmt.Fprintln(&out, "== loc ==")
	locFound, err := lint.LOC([]string{"."}, lint.DefaultMaxLines, &out)
	if err != nil {
		return fmt.Errorf("lint (loc) could not run — fix this before submitting:\n%s\n%w", out.String(), err)
	}

	fmt.Fprintln(&out, "== deadcode ==")
	dcFound, err := lint.Deadcode([]string{"./..."}, "", false, &out)
	if err != nil {
		return fmt.Errorf("lint (deadcode) could not run — fix build errors before submitting:\n%s\n%w", out.String(), err)
	}

	if locFound || dcFound {
		fmt.Fprint(w, out.String())
		return fmt.Errorf("lint failed — fix the violations above before submitting")
	}
	fmt.Fprintln(w, "Lint passed.")
	return nil
}

func init() {
	submitCmd.Flags().StringVarP(&submitTitle, "title", "t", "", "PR title (required, use conventional commits)")
	submitCmd.Flags().StringVarP(&submitBody, "body", "b", "", "PR body")
	submitCmd.Flags().StringVar(&submitDone, "done", "", "Handoff summary (defaults to title)")
}
