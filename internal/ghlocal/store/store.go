// package: store
// type:    adapter (local PR persistence + git)
// job:     the local pull-request store — PRs as JSON under .git/pr/, plus
//          approve/merge (git checkout/merge/branch-delete).
// limits:  no task knowledge (-> adapter/td), no review-gate or issue rules
//          (-> issue); callers decide policy, this just persists and merges.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PR represents a local pull request.
type PR struct {
	ID        string `json:"id"`
	Branch    string `json:"branch"`
	Base      string `json:"base"`
	Status    string `json:"status"` // open | approved | merged
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	Diff      string `json:"diff"`
}

// prDir returns the .git/pr/ directory, creating it if needed.
func prDir() (string, error) {
	gitDir, err := findGitDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(gitDir, "pr")
	if err := os.MkdirAll(p, 0755); err != nil {
		return "", err
	}
	return p, nil
}

// Approve marks a PR as approved.
func Approve(id string) (*PR, error) {
	pr, err := Read(id)
	if err != nil {
		return nil, err
	}
	if pr.Status == "merged" {
		return nil, fmt.Errorf("PR %s is already merged", id)
	}
	pr.Status = "approved"
	return pr, Write(pr)
}

// Merge merges an approved PR into its base branch.
func Merge(id string) (*PR, error) {
	pr, err := Read(id)
	if err != nil {
		return nil, err
	}
	if pr.Status != "approved" {
		return nil, fmt.Errorf("PR %s is not approved (status: %s)", id, pr.Status)
	}
	if out, err := exec.Command("git", "checkout", pr.Base).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("checkout %s failed: %s", pr.Base, out)
	}
	mergeMsg := pr.Title
	if out, err := exec.Command("git", "merge", "--no-ff", pr.Branch, "-m", mergeMsg).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("merge failed: %s", out)
	}
	_ = exec.Command("git", "branch", "-d", pr.Branch).Run()
	pr.Status = "merged"
	return pr, Write(pr)
}

// PRDirFor returns the .git/pr/ directory for a specific project root.
func PRDirFor(projectRoot string) string {
	return filepath.Join(projectRoot, ".git", "pr")
}

// ListFor returns all PRs for a specific project root.
func ListFor(projectRoot string) ([]*PR, error) {
	dir := PRDirFor(projectRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var prs []*PR
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		pr, err := ReadFrom(dir, id)
		if err != nil {
			continue
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

// ReadFrom loads a PR by ID from a specific directory.
func ReadFrom(dir, id string) (*PR, error) {
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("PR %q not found", id)
	}
	var pr PR
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// findGitDir walks up from cwd until it finds a .git directory or worktree file.
// For worktrees, resolves back to the main repo's .git directory so PRs are shared.
func findGitDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(cwd, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate, nil
			}
			// Worktree: .git is a file containing "gitdir: <path>"
			return resolveWorktreeGitDir(candidate)
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "", fmt.Errorf(".git directory not found")
}

// resolveWorktreeGitDir reads a worktree .git file and resolves to the main .git dir.
// The file contains "gitdir: /main/repo/.git/worktrees/<branch>".
func resolveWorktreeGitDir(gitFile string) (string, error) {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("unexpected .git file format: %s", line)
	}
	gitdir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(gitFile), gitdir)
	}
	// gitdir is /main/repo/.git/worktrees/<branch> — walk up to .git
	resolved := filepath.Clean(gitdir)
	for {
		if filepath.Base(resolved) == ".git" {
			return resolved, nil
		}
		parent := filepath.Dir(resolved)
		if parent == resolved {
			break
		}
		resolved = parent
	}
	return "", fmt.Errorf("could not resolve main .git from worktree: %s", gitdir)
}

func prPath(id string) (string, error) {
	dir, err := prDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

// Write persists a PR to .git/pr/<id>.json.
func Write(pr *PR) error {
	path, err := prPath(pr.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(pr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Read loads a PR by ID.
func Read(id string) (*PR, error) {
	path, err := prPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("PR %q not found", id)
		}
		return nil, err
	}
	var pr PR
	if err := json.Unmarshal(data, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// List returns all PRs.
func List() ([]*PR, error) {
	dir, err := prDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var prs []*PR
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		pr, err := Read(id)
		if err != nil {
			continue
		}
		prs = append(prs, pr)
	}
	return prs, nil
}
