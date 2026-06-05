// package: main (sindri) / init
// type:    command
// job:     wires `sindri init` — interactively scaffold the `.sindri/` project
//          directory (agent index, config, gitignore).
// limits:  the scaffold logic lives in internal/sindri (EnsureSindri).
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/sindri"
	"github.com/flo-at/sindri/internal/worker"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newInitCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold the .sindri/ project directory (agent index)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	return cmd
}

func runInit(yes bool) error {
	projectRoot, err := worker.GitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	if sindri.Exists(projectRoot) {
		fmt.Printf("✓ .sindri/ already initialised in %s\n", projectRoot)
		// Re-run EnsureSindri so a partial scaffold is healed.
		return sindri.EnsureSindri(projectRoot)
	}

	if !yes && !confirm(fmt.Sprintf("Initialise .sindri/ in %s?", projectRoot)) {
		fmt.Fprintln(os.Stderr, "Aborted.")
		return nil
	}

	if err := sindri.EnsureSindri(projectRoot); err != nil {
		return err
	}

	fmt.Printf("✓ Created %s\n", sindri.Dir(projectRoot))
	fmt.Println("  · agents/            (agent index — one file per agent)")
	fmt.Println("  · config.json        (project config)")
	fmt.Println("  · agents/reviewer.json (seeded reviewer)")
	fmt.Println("  · .gitignore         (.sindri/ ignored — agents are local)")
	return nil
}

// confirm prompts y/N on a TTY; without a TTY it defaults to yes (non-blocking
// for automation, e.g. the TUI's startup ensure path which never reaches here).
func confirm(prompt string) bool {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	fmt.Printf("%s [Y/n] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes"
}
