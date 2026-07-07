// package: main (sindri) / repo commands
// type:    command (host CLI)
// job:     the `repo` verb group — init (register cwd + scaffold config), list,
//          info, forget — over the hub's registry. init/bare-info/config act on the
//          cwd repo; list/info<sel>/forget are global (resolve a repo by name or tag).
// limits:  thin calls into the backend; no logic of its own.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/flo-at/sindri/internal/hub"
	"github.com/spf13/cobra"
)

func newRepoCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "repo",
		Short: "Manage the repos the hub tracks (init, list, info, forget)",
		Long: "Repos self-register on first use; these verbs make that explicit and manageable.\n\n" +
			"  sindri repo init          register the current repo + scaffold .sindri/config.yaml\n" +
			"  sindri repo list          list every repo the hub tracks\n" +
			"  sindri repo info [repo]   show a repo's config + counts (default: current repo)\n" +
			"  sindri repo forget <repo> stop tracking a repo (registry only — files untouched)",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return repoInfo("") }, // bare `repo` → current repo
	}
	c.AddCommand(repoInitCmd(), repoListCmd(), repoInfoCmd(), repoForgetCmd(), repoColorCmd())
	return c
}

func repoInitCmd() *cobra.Command {
	return &cobra.Command{
		Use: "init", Short: "Register the current repo and scaffold .sindri/config.yaml", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withBackend(func(b backend) error {
				sum, err := b.RepoInit()
				if err != nil {
					return err
				}
				fmt.Printf("registered %s (%s)\n", sum.Name, sum.Path)
				fmt.Fprintln(os.Stderr, "scaffolded .sindri/config.yaml — edit it to configure this repo")
				return nil
			})
		},
	}
}

func repoListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List every repo the hub tracks", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withHub(func(b backend) error {
				repos, err := b.Repos()
				if err != nil {
					return err
				}
				for _, r := range repos {
					issues := "off"
					if r.IssuesEnabled {
						issues = "on"
					}
					fmt.Printf("%-16s %2d agents  issues:%-3s  %s\n", r.Name, r.Agents, issues, r.Path)
				}
				if len(repos) == 0 {
					fmt.Fprintln(os.Stderr, "no repos registered")
				}
				return nil
			})
		},
	}
}

func repoInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info [repo]", Short: "Show a repo's config and counts (default: current repo)", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			sel := ""
			if len(args) == 1 {
				sel = args[0]
			}
			return repoInfo(sel)
		},
	}
}

func repoForgetCmd() *cobra.Command {
	return &cobra.Command{
		Use: "forget <repo>", Short: "Stop tracking a repo (registry only — its files are untouched)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withHub(func(b backend) error {
				repos, err := b.Repos()
				if err != nil {
					return err
				}
				tag, err := resolveRepo(repos, args[0])
				if err != nil {
					return err
				}
				if err := b.RepoForget(tag); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "forgot %s — its files are untouched; it re-registers if you use it again\n", args[0])
				return nil
			})
		},
	}
}

func repoColorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "color <repo> <n>",
		Short: "Pin a repo's display colour (0 = default; 1..24 = palette index)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[1])
			if err != nil || n < 0 {
				return fmt.Errorf("colour must be a non-negative number (0 = default), got %q", args[1])
			}
			return withHub(func(b backend) error {
				repos, err := b.Repos()
				if err != nil {
					return err
				}
				tag, err := resolveRepo(repos, args[0])
				if err != nil {
					return err
				}
				if err := b.SetRepoColor(tag, n); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "set %s colour to %d\n", args[0], n)
				return nil
			})
		},
	}
}

// repoInfo prints one repo's detail: sel "" is the current repo (via the client's
// repo context); a non-empty sel is resolved by name or tag against the registry.
func repoInfo(sel string) error {
	return withHub(func(b backend) error {
		tag := ""
		if sel != "" {
			repos, err := b.Repos()
			if err != nil {
				return err
			}
			t, err := resolveRepo(repos, sel)
			if err != nil {
				return err
			}
			tag = t
		}
		d, err := b.RepoInfo(tag)
		if err != nil {
			return err
		}
		printRepoDetail(d)
		return nil
	})
}

// resolveRepo maps a user-supplied selector (repo name or tag) to a registry tag,
// erroring on no match or an ambiguous name (in which case the caller uses the tag).
func resolveRepo(repos []hub.RepoSummary, sel string) (string, error) {
	var matches []hub.RepoSummary
	for _, r := range repos {
		if r.Tag == sel || r.Name == sel {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no registered repo matches %q (see `sindri repo list`)", sel)
	case 1:
		return matches[0].Tag, nil
	default:
		var paths []string
		for _, m := range matches {
			paths = append(paths, m.Path+" ("+m.Tag+")")
		}
		return "", fmt.Errorf("%q is ambiguous — matches %s; use the tag", sel, strings.Join(paths, ", "))
	}
}

func printRepoDetail(d hub.RepoDetail) {
	issues := "off"
	if d.IssuesEnabled {
		issues = "on"
	}
	fmt.Printf("repo:     %s\npath:     %s\ntag:      %s\n", d.Name, d.Path, d.Tag)
	fmt.Printf("agents:   %d\ntasks:    %d open / %d total\nprs:      %d open / %d total\n",
		d.Agents, d.OpenTasks, d.Tasks, d.OpenPRs, d.PRs)
	fmt.Printf("config:\n  architecture:  %s\n  containerfile: %s\n  review_prompt: %s\n  github.issues: %s\n",
		dash(d.Config.Architecture), dash(d.Config.Containerfile), dash(d.Config.ReviewPrompt), issues)
}

// withHub runs fn against the hub without requiring the cwd to be a git repo — for
// the global repo verbs (list, forget, info <sel>) that operate on any registered
// repo by selector, not on the current directory.
func withHub(fn func(backend) error) error {
	root, _ := repoRoot() // "" is fine — global repo routes don't need a repo context
	b, err := open(root)
	if err != nil {
		return err
	}
	defer b.Close()
	return fn(b)
}
