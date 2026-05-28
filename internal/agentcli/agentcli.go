// package: agentcli / agentcli
// type:    entrypoint
// job:     assembles the two agent command trees from the shared command set —
//          WorkerRoot (sindri-worker) and ReviewRoot (sindri-review).
// limits:  command behavior lives in the sibling files; merge is human-only and
//          lives on the host (-> cmd/sindri/pr.go).
package agentcli

import (
	"os"

	"github.com/spf13/cobra"
)

// WorkerRoot builds the sindri-worker CLI: the worker agent's verbs.
func WorkerRoot() *cobra.Command {
	root := newRoot("sindri-worker",
		"sindri-worker — local workflow engine for the worker agent (NOT GitHub)",
		workerHelp)
	root.AddCommand(workerIssueCmd(), submitCmd, doneCmd, workerPRCmd())
	return root
}

// ReviewRoot builds the sindri-review CLI: the reviewer agent's verbs.
func ReviewRoot() *cobra.Command {
	bannerName = "sindri-review"
	root := newRoot("sindri-review",
		"sindri-review — local PR review for the reviewer agent (NOT GitHub)",
		reviewHelp)
	root.AddCommand(reviewIssueCmd(), reviewPRCmd())
	return root
}

// Execute runs the given root and exits non-zero on error.
func Execute(root *cobra.Command) {
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func workerIssueCmd() *cobra.Command {
	c := &cobra.Command{Use: "issue", Short: "Manage tasks (issues)"}
	c.AddCommand(issueNextCmd, issueListCmd, issueViewCmd, issueCommentCmd)
	return c
}

func reviewIssueCmd() *cobra.Command {
	c := &cobra.Command{Use: "issue", Short: "Inspect tasks and record findings"}
	c.AddCommand(issueListCmd, issueViewCmd, issueCommentCmd)
	return c
}

func workerPRCmd() *cobra.Command {
	c := &cobra.Command{Use: "pr", Short: "Create and inspect local PRs"}
	c.AddCommand(prCreateCmd, prListCmd, prViewCmd)
	return c
}

func reviewPRCmd() *cobra.Command {
	c := &cobra.Command{Use: "pr", Short: "Review local PRs (approve/reject) — merge is human-only on the host"}
	c.AddCommand(prListCmd, prViewCmd, prApproveCmd, prRejectCmd)
	return c
}
