package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "sindri",
		Short: "Sindri — AI agent orchestrator",
	}

	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Manage Sindri workers",
	}

	workerListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all Sindri workers (containers + worktrees)",
		RunE:  runWorkerList,
	}

	workerStreamCmd := &cobra.Command{
		Use:   "stream <name>",
		Short: "Stream live output from a worker",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkerStream,
	}

	workerCmd.AddCommand(workerListCmd, workerStreamCmd)
	rootCmd.AddCommand(workerCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type podmanContainer struct {
	Names  []string          `json:"Names"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

func runWorkerList(cmd *cobra.Command, args []string) error {
	// Get project root
	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Query podman for sindri containers
	out, err := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--format", "json",
	).Output()
	if err != nil {
		return fmt.Errorf("podman ps failed: %w", err)
	}

	var containers []podmanContainer
	if len(strings.TrimSpace(string(out))) > 0 {
		if err := json.Unmarshal(out, &containers); err != nil {
			return fmt.Errorf("parse podman output: %w", err)
		}
	}

	// Query git worktrees
	wtOut, _ := exec.Command("git", "-C", projectRoot, "worktree", "list", "--porcelain").Output()
	worktrees := parseWorktreeNames(string(wtOut), projectRoot)

	// Build combined view
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tROLE\tCONTAINER\tSTATUS")
	fmt.Fprintln(w, "----\t----\t---------\t------")

	// Track which workers have containers
	containerByWorker := make(map[string]*podmanContainer)
	for i, c := range containers {
		worker := c.Labels["sindri.worker"]
		if worker != "" {
			containerByWorker[worker] = &containers[i]
		}
	}

	// Main repo = reviewer
	mainPath := worktrees["main"]
	name := "sindri"
	if mainPath != "" {
		parts := strings.Split(mainPath, "/")
		name = parts[len(parts)-1]
	}
	if c, ok := containerByWorker["_reviewer"]; ok {
		fmt.Fprintf(w, "👑 %s\treviewer\t%s\t%s\n", name, containerName(c), c.State)
		delete(containerByWorker, "_reviewer")
	} else {
		fmt.Fprintf(w, "👑 %s\treviewer\t-\tno container\n", name)
	}

	// Worker worktrees
	for wtName := range worktrees {
		if wtName == "main" {
			continue
		}
		if c, ok := containerByWorker[wtName]; ok {
			fmt.Fprintf(w, "🔨 %s\tworker\t%s\t%s\n", wtName, containerName(c), c.State)
			delete(containerByWorker, wtName)
		} else {
			fmt.Fprintf(w, "🔨 %s\tworker\t-\tno container\n", wtName)
		}
	}

	// Orphaned containers (no worktree)
	for worker, c := range containerByWorker {
		fmt.Fprintf(w, "⚠  %s\torphan\t%s\t%s\n", worker, containerName(c), c.State)
	}

	w.Flush()
	return nil
}

func runWorkerStream(cmd *cobra.Command, args []string) error {
	name := args[0]

	projectRoot, err := gitRoot()
	if err != nil {
		return fmt.Errorf("not in a git repo: %w", err)
	}

	// Determine container name
	cName := "sindri-" + name
	if name == "reviewer" || name == "_reviewer" {
		cName = "sindri-reviewer"
	}

	// Verify container exists
	check := exec.Command("podman", "ps", "-a",
		"--filter", "label=sindri.project="+projectRoot,
		"--filter", "name="+cName,
		"--format", "{{.State}}")
	stateOut, err := check.Output()
	if err != nil || strings.TrimSpace(string(stateOut)) == "" {
		return fmt.Errorf("no container found for worker %q", name)
	}

	state := strings.TrimSpace(string(stateOut))
	if state != "running" {
		return fmt.Errorf("container %s is %s (not running)", cName, state)
	}

	fmt.Fprintf(os.Stderr, "Streaming %s (%s)...\n", name, cName)

	// Stream logs
	logs := exec.Command("podman", "logs", "-f", cName)
	logs.Stdout = os.Stdout
	logs.Stderr = os.Stderr
	return logs.Run()
}

func gitRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func containerName(c *podmanContainer) string {
	if len(c.Names) > 0 {
		return c.Names[0]
	}
	return "?"
}

func parseWorktreeNames(output, mainDir string) map[string]string {
	names := make(map[string]string)
	var currentPath string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
			name := "main"
			if currentPath != mainDir {
				parts := strings.Split(currentPath, "/")
				name = parts[len(parts)-1]
			}
			names[name] = currentPath
		}
	}
	return names
}
