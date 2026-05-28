// package: agentcli / root
// type:    entrypoint
// job:     the shared root builder and the "not GitHub" unknown-command help
//          text for the agent CLIs (sindri-worker, sindri-review).
// limits:  the two command trees are assembled in agentcli.go; subcommand
//          behavior lives in the sibling files; storage in store.
package agentcli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newRoot builds an agent CLI root with the shared "not GitHub" unknown-command
// help. Each binary supplies its own name, short, and help body.
func newRoot(use, short, help string) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown command: %s %s\n\n%s", use, strings.Join(args, " "), help)
		},
	}
}

const workerHelp = `This is sindri-worker — a local workflow engine, NOT the GitHub CLI.
There is no GitHub remote. All operations are local.

Workflow commands:
  sindri-worker issue next       Pick up the next task (claim, branch, show details)
  sindri-worker issue list       List tasks
  sindri-worker issue view <id>  Show task details + comments + PR status
  sindri-worker issue comment    Add a comment to a task
  sindri-worker submit           Submit work (rebase, lint, create PR, handoff)
  sindri-worker done             Return to base branch for next task
  sindri-worker pr list|view     Inspect local PRs
  sindri-worker pr create        Create a PR (prefer 'sindri-worker submit')

Approving, rejecting, and merging are not worker actions.`

const reviewHelp = `This is sindri-review — local PR review, NOT the GitHub CLI.
There is no GitHub remote. All operations are local.

Review commands:
  sindri-review pr list          List open PRs
  sindri-review pr view <id>     Show a PR (diff + task)
  sindri-review pr approve <id>  Approve a PR (satisfies its review gates)
  sindri-review pr reject <id>   Reject a PR back for rework (-m reason)
  sindri-review issue view <id>  Show task details + comments
  sindri-review issue comment    Record findings on a task

Merging is human-only and lives on the host (sindri pr merge).`
