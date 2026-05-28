// package: gh / banner
// type:    command
// job:     shared sindri-worker helpers — the "[sindri-worker]" banner, the td
//          wrapper, and base-branch detection used across the subcommands.
// limits:  helpers only; each subcommand's behavior lives in its own file.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const bannerPrefix = "\033[2m[sindri-worker — not github]"

func printBanner() {
	taskID := currentTaskID()
	if taskID != "" {
		fmt.Fprintf(os.Stderr, "%s [task: %s]\033[0m\n", bannerPrefix, taskID)
	} else {
		fmt.Fprintf(os.Stderr, "%s [no task selected]\033[0m\n", bannerPrefix)
	}
}

func tdProjectDir() string {
	return os.Getenv("TD_ROOT")
}

func td(args ...string) (string, error) {
	root := tdProjectDir()
	if root != "" {
		args = append([]string{"-w", root}, args...)
	}
	out, err := exec.Command("td", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func baseBranch() string {
	if b := os.Getenv("GH_LOCAL_BASE"); b != "" {
		return b
	}
	if out, err := exec.Command("git", "-C", "/repo", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		b := strings.TrimSpace(string(out))
		if b != "" && b != "HEAD" {
			return b
		}
	}
	return "master"
}
