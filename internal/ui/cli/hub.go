// package: ui/cli / commands
// type:    command (host CLI)
// job:     the host command tree — hierarchical <category> <action>: agent
// {list,new,launch,tell,attach,info}, task {list,new,info}, pr
// {list,info,merge}; plus first-order hub. Every hub capability has a
// CLI verb so functionality is verifiable from the shell, not only the
// TUI.
// limits:  no logic — each verb is a thin call into the socket client.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/client"
	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/tools/paths"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// backend is the hub operation set the host CLI uses, satisfied by *client.HTTP. Repo context
// rides the client's X-Sindri-Project header, so these signatures carry no project.
type backend interface {
	NewAgent(name, role, memory string) (string, error)
	SetMemory(name, memory string) error
	SetRetired(name string, retired bool) error
	DeleteAgent(name string) error
	StopAgent(name string) error
	SetClearArmed(name string, armed bool) error
	RebaseAgent(name string) error
	RebuildImage(name string, out io.Writer) error
	AgentPane(name string, lines int) (string, error)
	Diagnose(name string) (string, error)
	Stats() (api.StatsReport, error)
	Instance(name string) (string, error)
	Clients(name string) ([]api.ClientView, error)
	Launch(name string, shell, debug bool, out io.Writer) error
	Tell(name, msg, source string) error
	AssignPlan(name, goal, taskID string) error
	ChatAdd(name string) error
	ChatRemove(name string) error
	ChatSay(msg string) error
	NewMeeting() error
	ChatHeartbeat() error
	Chat() (api.ChatView, error)
	ChatWatch(ctx context.Context) (<-chan api.ChatView, error)
	State() (api.BoardState, error)
	Log(name string) ([]api.Event, error)
	Tasks() ([]api.Task, error)
	TaskInfo(id string) (api.Task, error)
	CreateTask(s api.TaskSpec) (string, error)
	EditTask(id string, s api.TaskSpec) error
	SetPriority(id, priority string, scope api.PriorityScope) error
	ApproveTask(id string, subtree bool) error
	RejectTask(id, comment string) error
	AddTaskComment(id, body string) error
	RefreshTaskComments(id string) error
	NextTask(agent string) (api.NextExplain, error)
	UnassignTask(id string) error
	CloseTask(id string) error
	ReopenTask(id, reason string) error
	ScrapTask(id string, subtree, withPRs bool) error
	Refresh() error
	PRs() ([]api.PR, error)
	PRInfo(id string) (api.PRDetail, error)
	RejectPR(id, feedback string) error
	ApprovePR(id string) error
	DiscardPR(id string) error
	LintPR(id string) (string, error)
	RequestReview(id, requirement string) error
	MaterializeReview(id string) (string, error)
	Merge(id string) (api.PR, error)
	MilestonePR(agent string) (api.PR, error)
	Repos() ([]api.RepoSummary, error)
	RepoInfo(tag string) (api.RepoDetail, error)
	RepoInit() (api.RepoSummary, error)
	RepoForget(tag string) error
	SetRepoColor(tag string, color int) error
	RemoveOrphan(name string) error
	WriteRepoConfig(cfg config.Config) error
	Close() error
}

func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return git.Root(wd)
}

// open connects to the single global hub, auto-starting it if needed. One hub serves every repo
// and is cheap to keep running, so there is no in-process backend.
func open(root string) (backend, error) {
	if err := ensureHubRunning(); err != nil {
		return nil, err
	}
	return dialHub(root)
}

func withBackend(fn func(backend) error) error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	b, err := open(root)
	if err != nil {
		return err
	}
	defer b.Close()
	return fn(b)
}

// --- first-order: hub ---

// NewHubCmd builds the `hub` command tree (start/stop/status the hub).
func NewHubCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "hub",
		Short: "Manage the global hub service (start, restart, status, stop)",
		Long: "The hub is the single global coordinator that drives the agents across\n" +
			"every repo.\n\n" +
			"  sindri hub start        run the hub in the foreground\n" +
			"  sindri hub start --bg   run it in the background (same as `sindri hub start &`)\n" +
			"  sindri hub restart      stop it and start a fresh detached one (pick up a rebuild)\n" +
			"  sindri hub status       show the running hub (pid, version, uptime)\n" +
			"  sindri hub stop         stop the running hub\n" +
			"  sindri hub logs         show its log (--agent/--grep to filter, --follow to stream)",
	}
	c.AddCommand(newHubStartCmd(), newHubRestartCmd(), newHubStatusCmd(), newHubStopCmd(), newHubLogsCmd())
	return c
}

func newHubStartCmd() *cobra.Command {
	var bg bool
	c := &cobra.Command{
		Use:   "start",
		Short: "Run this repo's hub in the foreground (--bg to run it in the background)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if client.IsRunning() {
				// A hub is already up. Same build → nothing to do; a different (or
				// unknown, pre-stamp) build → offer to take over.
				_, ver, ok := client.ReadPID()
				if ok && ver == version {
					return fmt.Errorf("a hub is already running (%s)", paths.HubSocket())
				}
				desc := "an older build (predates version stamping)"
				if ok {
					desc = "sindri " + ver
				}
				fmt.Fprintf(os.Stderr, "a hub (%s) is already running; this CLI is %s.\n", desc, version)
				if !term.IsTerminal(int(os.Stdin.Fd())) || !promptYesNo("stop it and start this one?") {
					return fmt.Errorf("a hub is already running (%s)", paths.HubSocket())
				}
				pid, havePID := client.HubPID()
				if !havePID {
					return fmt.Errorf("couldn't find the running hub's pid to stop it — stop it manually, then re-run")
				}
				if err := stopHub(pid); err != nil {
					return err
				}
			}
			if bg {
				return startHub() // detached; returns once the socket answers
			}
			warnShadowedInstall(os.Stderr) // a rival copy on PATH decides which build agents get
			bin, err := hubBinary()
			if err != nil {
				return err
			}
			// Exec, not spawn: the hub takes over this process outright, so signals (a
			// terminal's ctrl-C, a service manager's SIGTERM) land on it directly, with no
			// wrapper process in between.
			return syscall.Exec(bin, []string{bin}, os.Environ())
		},
	}
	c.Flags().BoolVar(&bg, "bg", false, "run the hub detached in the background instead of the foreground")
	return c
}

// newHubRestartCmd stops the running hub and starts a fresh detached one — how a rebuilt binary
// is picked up. With no hub up it is just a start. Agents survive; only the coordinator moves.
func newHubRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the hub in the background (starts one if none is running)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !client.IsRunning() {
				fmt.Fprintln(os.Stderr, "no hub running — starting a fresh one.")
				return startHub()
			}
			pid, ok := client.HubPID()
			if !ok {
				return fmt.Errorf("couldn't find the running hub's pid to restart it — stop it manually, then `sindri hub start --bg`")
			}
			return restartHub(pid)
		},
	}
}

// --- agent ---

// NewAgentCmd builds the `agent` command tree (manage agents).
func NewAgentCmd() *cobra.Command {
	// No PersistentPreRun: the runtime warning comes off the board (-> warnRuntime), which the
	// commands that need it already fetch. Probing here cost every agent verb a `podman info`.
	c := &cobra.Command{Use: "agent", Short: "Manage agents (workers, reviewers, planners, coauthors)"}
	c.AddCommand(agentListCmd(), agentStatsCmd(), agentNewCmd(), agentDeleteCmd(), agentPaneCmd(), agentStartCmd(), agentStopCmd(), agentRestartCmd(), agentRebaseCmd(), agentRebuildCmd(), agentMemoryCmd(), agentRetireCmd(), agentClearContextCmd(), agentTellCmd(), agentPlanCmd(), agentDirCmd(), agentAttachCmd(), agentInfoCmd())
	return c
}

// --- pr ---

// NewPrCmd builds the `pr` command tree (review/merge pull requests).
func NewPrCmd() *cobra.Command {
	c := &cobra.Command{Use: "pr", Short: "Inspect and merge pull requests (merge-intents)"}
	c.AddCommand(prListCmd(), prInfoCmd(), prReviewCmd(), prVerifyCmd(), prApproveCmd(), prRejectCmd(), prScrapCmd(), prLintCmd(), prMergeCmd(), prMilestoneCmd())
	return c
}

// prApproveCmd is the human approve: mark an open PR approved so it can be merged
// without a reviewer agent (the positive counterpart of pr reject).
func prApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use: "approve <pr-id>", Short: "Approve an open PR yourself (no reviewer agent needed), so it can be merged", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ApprovePR(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "%s approved — merge it with 'sindri pr merge %s'\n", args[0], args[0])
				return nil
			})
		},
	}
}

// prMilestoneCmd opens a milestone PR for the container an agent collaborates on: it captures
// the feature branch and blocks the agent until you merge, then it resumes the same container.
func prMilestoneCmd() *cobra.Command {
	return &cobra.Command{
		Use: "milestone <agent>", Short: "Open a milestone PR for the feature an agent is working (blocks it until merged)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				pr, err := b.MilestonePR(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "opened milestone %s on %s\n", pr.ID, pr.Branch)
				return nil
			})
		},
	}
}

func prVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use: "verify <pr-id>", Short: "Check the PR out into the review workspace for hands-on inspection", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				path, err := b.MaterializeReview(args[0])
				if err != nil {
					return err
				}
				fmt.Println(path) // the worktree path — cd there to inspect
				return nil
			})
		},
	}
}

func prReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use: "review <pr-id> [requirement...]", Short: "Request an agentic review of a PR (assigns a reviewer agent)", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RequestReview(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "review requested for %s\n", args[0])
				return nil
			})
		},
	}
}

func prRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reject <pr-id> <feedback...>", Short: "Reject a PR with feedback (routed to the worker)", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RejectPR(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rejected %s\n", args[0])
				return nil
			})
		},
	}
}

// prScrapCmd discards a PR outright (delete/rm alias it), matching the TUI's D. Unlike reject,
// which sends it BACK for another try, scrap ends it and drops the branch — hence --yes.
func prScrapCmd() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use: "scrap <pr-id>", Aliases: []string{"delete", "rm", "discard"},
		Short: "Scrap a PR and delete its branch (no feedback, nobody retries)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("scrapping %s deletes its branch and cannot be undone — pass --yes to confirm.\n"+
					"To send it back for another attempt instead, use `sindri pr reject %s \"<what to fix>\"`", args[0], args[0])
			}
			return withBackend(func(b backend) error {
				if err := b.DiscardPR(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "scrapped %s — branch deleted\n", args[0])
				return nil
			})
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "confirm: scrap the PR and delete its branch")
	return c
}

func prLintCmd() *cobra.Command {
	return &cobra.Command{
		Use: "lint <pr-id>", Short: "Run the quality gate against a PR's worktree", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				out, err := b.LintPR(args[0])
				if err != nil {
					return err
				}
				fmt.Print(out)
				return nil
			})
		},
	}
}

func prListCmd() *cobra.Command {
	var filter string
	c := &cobra.Command{
		Use: "list", Short: "List PRs", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := api.ParsePRFilter(filter)
			if err != nil {
				return err
			}
			return withBackend(func(b backend) error {
				all, err := b.PRs()
				if err != nil {
					return err
				}
				prs := api.FilterPRs(f, all)
				// The roster, because half of "waiting on you" is whether a reviewer runs in that
				// PR's repo (-> api.PRNeedsUser). A second round trip and the cheapest available:
				// /state is how a front-end learns who is running, off the hub's existing snapshot.
				st, err := b.State()
				if err != nil {
					return err
				}
				for _, p := range prs {
					status := api.StatusLabel(p.Status, p.Approvals)
					if p.Kind == "interim" { // ◇ = mid-task contribution (vs a final, task-done PR)
						status = "◇" + status
					}
					// Who is reviewing it, alongside who wrote it — the same column the PRs tab shows,
					// from the same field, so the two front-ends cannot answer differently.
					line := fmt.Sprintf("%-14s %-13s %4s  %-10s %-10s %s",
						p.ID, status, shortAge(p.CreatedAt), p.Agent, dash(p.Reviewer), p.Branch)
					if why := prWaitRow(api.PRWaitReason(p, st.Agents)); why != "" {
						line += "  ! " + why
					}
					fmt.Println(line)
				}
				if n := len(all) - len(prs); n > 0 {
					fmt.Fprintf(os.Stderr, "(filter %s — %d of %d PR(s) shown)\n", f, len(prs), len(all))
				} else if len(prs) == 0 {
					fmt.Fprintln(os.Stderr, "no PRs")
				}
				// Last, where a closing line is read: the same set the TUI counts on the PRs handle.
				// Over every PR, not the filtered rows — a PR waits on you whether or not this
				// listing happens to show it, exactly as `task list` reports gated work.
				if s := prNeedsYouSummary(all, st.Agents); s != "" {
					fmt.Fprintln(os.Stderr, "\n"+s)
				}
				return nil
			})
		},
	}
	// Defaults to "all", the same reasoning taskListCmd gives: a listing is a record, not the
	// TUI's redrawn view, which opens on "active" instead.
	c.Flags().StringVar(&filter, "filter", string(api.PRFilterAll),
		"which PRs to list: "+api.PRFilterNames()+" (active = open, plus anything closed within "+
			api.ActiveWindow.String()+")")
	return c
}

// prWaitWords is how each reason (-> api.PRWaitReason) reads: the row's short why, and how the
// closing line names the group with the command that clears it. Rendering only — a reason added to
// the rule arrives here as a gap the tests catch, never as a confident wrong remedy.
var prWaitWords = map[api.PRWait]struct{ row, fix string }{
	api.PRWaitMergeFailed: {
		"a merge died in flight — the base branch needs a look",
		"left mid-merge with the outcome unknown — inspect the base branch (`sindri pr info <id>`)",
	},
	api.PRWaitMerge: {
		"waiting on your merge",
		"approved and unmerged — `sindri pr merge <id>`",
	},
	api.PRWaitUserGated: {
		"user-gated: no reviewer is asked for an interim PR",
		"user-gated, so no reviewer will look — `sindri pr approve <id>`",
	},
	api.PRWaitReview: {
		"no reviewer is running in this repo",
		"waiting on a review with no reviewer running in their repo — review one yourself " +
			"(`sindri pr approve <id>`) or start a reviewer (`sindri agent new --role reviewer`)",
	},
}

// prWaitRow is the marker a listed row carries, "" when the PR waits on nobody. A reason with no
// words yet says only that it needs you: naming the wrong remedy is worse than naming none.
func prWaitRow(w api.PRWait) string {
	if w == api.PRWaitNone {
		return ""
	}
	if words, ok := prWaitWords[w]; ok {
		return words.row
	}
	return "needs you"
}

// prNeedsYouSummary names the PRs nothing but the user will move, "" when none — an approved one
// looks finished, a stranded one busy. Most stuck first, each with the command that clears it.
func prNeedsYouSummary(prs []api.PR, agents []api.AgentView) string {
	byReason := map[api.PRWait][]string{}
	n := 0
	for _, p := range prs {
		if w := api.PRWaitReason(p, agents); w != api.PRWaitNone {
			byReason[w] = append(byReason[w], p.ID)
			n++
		}
	}
	if n == 0 {
		return ""
	}
	var parts []string
	for _, w := range api.PRWaits {
		ids, ok := byReason[w]
		if !ok {
			continue
		}
		fix := "needs you" // as prWaitRow: an unrendered reason still gets its PRs named
		if words, ok := prWaitWords[w]; ok {
			fix = words.fix
		}
		parts = append(parts, strings.Join(ids, ", ")+" "+fix)
	}
	return fmt.Sprintf("%d PR(s) need you: %s.", n, strings.Join(parts, "; "))
}

// reviewBadge renders one review verdict: its state, verdict, author and when, marking a
// planner's advisory badge for what it is — a second opinion, never the approval that satisfies
// the merge gate.
func reviewBadge(r api.Review) string {
	switch {
	case r.Verdict != "":
		who := r.Author
		if r.Advisory {
			who += " (advisory)"
		}
		return fmt.Sprintf("%s by %s at %s", r.Verdict, who, eventTime(r.VerdictAt))
	case r.Author != "":
		return "in review by " + r.Author
	default:
		return "unassigned"
	}
}

func prInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use: "info <pr-id>", Short: "Show a PR and its diff", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				d, err := b.PRInfo(args[0])
				if err != nil {
					return err
				}
				p := d.PR
				kind := "final (task done)"
				if p.Kind == "interim" {
					kind = "interim (mid-task contribution)"
				}
				status := api.StatusLabel(p.Status, api.ApprovalCount(d.Reviews))
				fmt.Printf("%s  [%s]  %s  by %s\nbranch %s → %s\n", p.ID, status, kind, p.Agent, p.Branch, p.Base)
				if p.Feedback != "" {
					fmt.Printf("feedback: %s\n", p.Feedback)
				}
				for _, r := range d.Reviews {
					fmt.Println("review: " + reviewBadge(r))
				}
				fmt.Printf("\n%s\n", strings.TrimSpace(d.Diff))
				return nil
			})
		},
	}
}

func prMergeCmd() *cobra.Command {
	return &cobra.Command{
		Use: "merge <pr-id>", Short: "Merge an approved PR (human-only — the hard gate)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				pr, err := b.Merge(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "merged %s (task %s) → %s\n", pr.ID, pr.Task, pr.Base)
				return nil
			})
		},
	}
}
