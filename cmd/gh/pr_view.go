// package: gh / pr_view
// type:    command
// job:     `sindri-worker pr view` — prints a PR (JSON detail).
// limits:  PR records live in internal/ghlocal/store.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/flo-at/sindri/internal/ghlocal/store"
)

var prViewCmd = &cobra.Command{
	Use:   "view [pr-id]",
	Short: "View a local PR",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		printBanner()
		id, err := resolveID(args)
		if err != nil {
			return err
		}

		pr, err := store.Read(id)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(pr); err != nil {
			return err
		}
		return nil
	},
}

// resolveID returns the explicit ID or derives it from the current branch.
func resolveID(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}
	branch, err := currentBranch()
	if err != nil {
		return "", err
	}
	if branch == "" {
		return "", fmt.Errorf("no pr-id given and could not determine current branch")
	}
	return "pr-" + branch, nil
}
