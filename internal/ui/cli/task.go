// package: ui/cli / task
// type:    entrypoint (CLI command group)
// job:     the `sindri task …` verbs — list/info/new/edit/priority and the
// workflow actions (approve/reject/unassign/close). Each delegates to
// the hub via the shared backend (in-process or over the socket).
// limits:  no logic — argument plumbing only; the hub owns task semantics.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/ui/table"
	"github.com/flo-at/sindri/internal/ui/theme"
	"github.com/spf13/cobra"
)

// --- task ---

// tasksJSON renders task rows as JSON, always an array — never null — even with none.
func tasksJSON(tasks []api.Task) (string, error) {
	if tasks == nil {
		tasks = []api.Task{}
	}
	out, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// NewTaskCmd builds the `task` command tree (the backlog).
func NewTaskCmd() *cobra.Command {
	c := &cobra.Command{Use: "task", Short: "Inspect and create tasks"}
	c.AddCommand(taskListCmd(), taskInfoCmd(), taskNewCmd(), taskEditCmd(), taskPriorityCmd(), taskApproveCmd(), taskRejectCmd(), taskUnassignCmd(), taskCloseCmd(), taskReopenCmd(), taskDeleteCmd(), taskRefreshCmd(), taskCommentCmd(), taskNextCmd())
	return c
}

// taskRefreshCmd forces a re-sync without listing — e.g. to push fresh state to a running TUI.
// taskCommentCmd comments on a task; a GitHub issue's goes upstream, everything else stays in the hub.
func taskCommentCmd() *cobra.Command {
	return &cobra.Command{
		Use: "comment <id> <text...>", Short: "Comment on a task (a GitHub issue is commented upstream)",
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			body := strings.Join(args[1:], " ")
			return withBackend(func(b backend) error {
				if err := b.AddTaskComment(args[0], body); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "commented on %s\n", args[0])
				return nil
			})
		},
	}
}

// taskNextCmd answers "why is nothing being assigned" — what would be handed out, and why not the rest.
func taskNextCmd() *cobra.Command {
	var agent, role string
	c := &cobra.Command{
		Use: "next", Short: "Show what would be assigned next, and why each open task would not be",
		Long: "Show what would be assigned next, and why each open task would not be.\n\n" +
			"--agent asks on behalf of an agent that exists, whose own state can rule everything out.\n" +
			"--role asks as a hypothetical agent of that role holding nothing, which is how to find out\n" +
			"whether starting one would give it anything to do. The two cannot be combined: an agent\n" +
			"already has a role, so passing both states two things that can contradict.\n\n" +
			"A reviewer is served PRs rather than tasks, so its answer lives under `sindri pr next`.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if role == "reviewer" {
				// Answered, not refused blankly: the noun is the point — a reviewer's pool is PRs.
				return fmt.Errorf("a reviewer is offered PRs, not tasks — ask `sindri pr next`")
			}
			return withBackend(func(b backend) error {
				x, err := b.NextTask(agent, role)
				if err != nil {
					return err
				}
				// Checked again after the call: a named agent brings its own role, known only now.
				if nextIsAboutPRs(x.Role) {
					return wrongNounRefusal(x.Role, agent)
				}
				fmt.Print(theme.FormatNext(x))
				return nil
			})
		},
	}
	c.Flags().StringVar(&agent, "agent", "", "ask on behalf of this agent (its own state can rule everything out)")
	c.Flags().StringVar(&role, "role", "", "ask as a hypothetical agent of this role: worker|planner|coauthor (a reviewer: `sindri pr next`)")
	c.MarkFlagsMutuallyExclusive("agent", "role")
	return c
}

func taskRefreshCmd() *cobra.Command {
	return &cobra.Command{
		Use: "refresh", Short: "Re-sync tasks from every source and notify watchers", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return withBackend(func(b backend) error {
				if err := b.Refresh(); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "synced tasks")
				return nil
			})
		},
	}
}

// taskCloseCmd marks a task done — dispatched by backend (td close / openspec archive / issue close).
func taskCloseCmd() *cobra.Command {
	return &cobra.Command{
		Use: "close <id>", Short: "Close a task (done): td close · openspec archive · issue close", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.CloseTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "closed %s\n", args[0])
				return nil
			})
		},
	}
}

// taskReopenCmd reopens a closed sindri-owned task with a reason; a task whose status comes from
// its own source (openspec, GitHub) refuses.
func taskReopenCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reopen <id> <reason...>", Short: "Reopen a closed task, with a reason (sindri-owned tasks only)", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			id, reason := args[0], strings.Join(args[1:], " ")
			return withBackend(func(b backend) error {
				if err := b.ReopenTask(id, reason); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "reopened %s\n", id)
				// A priority left standing from before the close makes this claimable right away.
				if t, terr := b.TaskInfo(id); terr == nil && t.Priority != "" {
					fmt.Fprintf(os.Stderr, "%s still carries priority %s, so a worker may claim it immediately\n",
						id, theme.PriorityLabel(t.Priority))
				}
				return nil
			})
		},
	}
}

// taskDeleteCmd scraps a task; --subtasks widens down the tree, --prs takes their open PRs too.
func taskDeleteCmd() *cobra.Command {
	var subtasks, prs bool
	c := &cobra.Command{
		Use: "delete <id>", Aliases: []string{"rm", "scrap"},
		Short: "Scrap a task (discard): td delete · openspec change removal · issue delete", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.ScrapTask(args[0], subtasks, prs); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "scrapped %s%s\n", args[0], scrapExtent(subtasks, prs))
				return nil
			})
		},
	}
	c.Flags().BoolVar(&subtasks, "subtasks", false, "scrap everything under the task too (children, theirs, …)")
	c.Flags().BoolVar(&prs, "prs", false, "scrap the open PR of every task scrapped")
	return c
}

// scrapExtent names how far a scrap reached, for the confirmation line.
func scrapExtent(subtasks, prs bool) string {
	switch {
	case subtasks && prs:
		return " with its subtasks and their PRs"
	case subtasks:
		return " with its subtasks"
	case prs:
		return " and its PR"
	}
	return ""
}

func taskUnassignCmd() *cobra.Command {
	return &cobra.Command{
		Use: "unassign <id>", Short: "Release a task back to the backlog (refused if a live agent holds it)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.UnassignTask(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "unassigned %s\n", args[0])
				return nil
			})
		},
	}
}

func taskApproveCmd() *cobra.Command {
	var subtasks bool
	var priority, scope string
	c := &cobra.Command{
		Use: "approve <id>", Short: "Approve a planner-proposed task (makes it claimable)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			sc, ok := api.ParsePriorityScope(scope)
			if !ok {
				return fmt.Errorf("unknown --scope %q (task, unrated, all)", scope)
			}
			return withBackend(func(b backend) error {
				id := args[0]
				// Read before the write, so both numbers name what this call decided on.
				all, _ := b.Tasks()
				pending := len(api.PendingApproval(all, id))
				if err := b.ApproveTask(id, subtasks); err != nil {
					return err
				}
				switch {
				case subtasks && pending > 0:
					fmt.Fprintf(os.Stderr, "approved %s and %s below it\n", id, theme.Plural(pending, "task", "tasks"))
				case pending > 0:
					// The TUI offers the wider approve in a modal; here the flag is the way to it.
					fmt.Fprintf(os.Stderr, "approved %s — %s below it still await approval (--subtasks takes them too)\n",
						id, theme.Plural(pending, "task", "tasks"))
				default:
					fmt.Fprintf(os.Stderr, "approved %s\n", id)
				}
				return approvedPriority(b, id, priority, sc, all)
			})
		},
	}
	c.Flags().BoolVar(&subtasks, "subtasks", false, "approve every task below it that still awaits a verdict")
	c.Flags().StringVar(&priority, "priority", "",
		"rate it in the same call (critical|high|mid|low|none) — an approve alone leaves an unrated task inert")
	c.Flags().StringVar(&scope, "scope", string(api.ScopeTask), "how far --priority reaches: task, unrated, all")
	return c
}

// approvedPriority is the second gate: an unrated task is claimable by nobody, so the rating is
// either given here or its absence is said out loud.
func approvedPriority(b backend, id, priority string, scope api.PriorityScope, all []api.Task) error {
	if priority != "" {
		cascade := api.PriorityEffect(all, id)
		if err := b.SetPriority(id, theme.PriorityCode(priority), scope); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "set %s priority %s%s\n", id, priority, scopeExtent(scope, cascade))
		if note := theme.PriorityScopeNote(cascade); note != "" {
			fmt.Fprintf(os.Stderr, "%s\n", note)
		}
		return nil
	}
	if len(all) == 0 {
		return nil // the backlog didn't read; silence beats being wrong about what is owed
	}
	if api.ReleasedByPriority(all)[id] {
		// Name the rating this approve just made effective — a planner may have proposed it.
		if word := priorityOf(all, id); word != "" {
			fmt.Fprintf(os.Stderr, "%s is %s and now claimable\n", id, word)
		}
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s has no priority, so no worker can claim it yet — "+
		"`task priority %s <critical|high|mid|low|none>`, or --priority does both in one call\n", id, id)
	return nil
}

// taskLister is the backend slice this needs: the backlog, for advice about the tree as it stands.
type taskLister interface{ Tasks() ([]api.Task, error) }

// ratedApproval names the approve that would release a still-pending task — approving is the user's.
func ratedApproval(w io.Writer, b taskLister, id string) {
	all, err := b.Tasks()
	if err != nil || len(all) == 0 {
		return // the backlog didn't read; silence beats being wrong about what is owed
	}
	pending := false
	for _, t := range all {
		if t.ID == id {
			pending = t.Approval == "pending"
			break
		}
	}
	if !pending {
		return
	}
	fmt.Fprintf(w, "%s still awaits approval, so no worker can claim it yet — "+
		"`task approve %s` releases it\n", id, id)
	// Children a scope reached can be pending too, and each is its own verdict to give.
	if below := len(api.PendingApproval(all, id)); below > 0 {
		fmt.Fprintf(w, "%s below it also await approval (`task approve %s --subtasks` takes them too)\n",
			theme.Plural(below, "task", "tasks"), id)
	}
}

// priorityOf is the priority a task carries itself, or "" — an ancestor's rating is not this task's.
func priorityOf(all []api.Task, id string) string {
	for _, t := range all {
		if t.ID == id && t.Priority != "" {
			return theme.PriorityLabel(t.Priority)
		}
	}
	return ""
}

func taskRejectCmd() *cobra.Command {
	return &cobra.Command{
		Use: "reject <id> <comment...>", Short: "Reject a planner-proposed task with a comment", Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.RejectTask(args[0], strings.Join(args[1:], " ")); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "rejected %s\n", args[0])
				return nil
			})
		},
	}
}

func taskPriorityCmd() *cobra.Command {
	var scope string
	c := &cobra.Command{
		Use:   "priority <id> <critical|high|mid|low|none>",
		Short: "Set a task's priority (a P-code; openspec and GitHub items keep theirs in our db)",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			sc, ok := api.ParsePriorityScope(scope)
			if !ok {
				return fmt.Errorf("unknown --scope %q (task, unrated, all)", scope)
			}
			return withBackend(func(b backend) error {
				id := args[0]
				// Read before the write, so the account below describes the tree this call decided on.
				cascade := priorityReach(b, id)
				if err := b.SetPriority(id, theme.PriorityCode(args[1]), sc); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "set %s priority %s%s\n", id, args[1], scopeExtent(sc, cascade))
				// A rating reaching open children ordered them; it did not release them.
				if note := theme.PriorityScopeNote(cascade); note != "" {
					fmt.Fprintf(os.Stderr, "%s\n", note)
					if sc == api.ScopeTask {
						fmt.Fprintf(os.Stderr, "--scope unrated|all carries the rating down to them\n")
					}
				}
				ratedApproval(os.Stderr, b, id)
				return nil
			})
		},
	}
	c.Flags().StringVar(&scope, "scope", string(api.ScopeTask),
		"how far the rating reaches: task (this one), unrated (+ the open tasks below with none set), all (+ every open task below)")
	return c
}

// priorityReach is what a rating on id could carry to; an unreadable backlog yields nothing, not a failure.
func priorityReach(b backend, id string) api.PriorityCascade {
	all, err := b.Tasks()
	if err != nil {
		return api.PriorityCascade{}
	}
	return api.PriorityEffect(all, id)
}

// scopeExtent names how far a rating reached, for the confirmation line.
func scopeExtent(scope api.PriorityScope, c api.PriorityCascade) string {
	n := c.Children
	if scope == api.ScopeUnrated {
		n = c.Unrated
	}
	if scope == api.ScopeTask || n == 0 {
		return ""
	}
	return " and " + theme.Plural(n, "task", "tasks") + " below it"
}

// taskState is the word a listing shows: the approval gate where one is set, else the status —
// "open" alone would claim a pending task is available when no worker can see it.
func taskState(t api.Task) string {
	if t.Approval == "pending" || t.Approval == "rejected" {
		return theme.ApprovalLabel(t.Approval)
	}
	return t.Status
}

// taskListTable is the columns `sindri task list` prints, its header and its rows alike.
var taskListTable = table.Table{
	{Label: "id", Width: 12},
	{Label: "prio", Width: 8},
	{Label: "tier", Width: 6},
	{Label: "state", Width: 12},
	{Label: "age", Width: 4, Right: true},
	{Label: "agent", Width: 12}, // who holds it (-> api.AgentsByTask)
	{Label: "title"},
}

func taskListCmd() *cobra.Command {
	var asJSON bool
	var filter string
	var limit int
	c := &cobra.Command{
		Use: "list", Short: "List tasks", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			f, err := api.ParseTaskFilter(filter)
			if err != nil {
				return err
			}
			return withBackend(func(b backend) error {
				all, err := b.Tasks()
				if err != nil {
					return err
				}
				tasks := api.FilterTasks(f, all)
				if asJSON {
					// Uncapped: a script asked for the filtered set and reads only stdout, so it
					// cannot see a stderr notice — --limit is for a screen, not this.
					out, err := tasksJSON(tasks)
					if err != nil {
						return err
					}
					fmt.Println(out)
					return nil
				}
				// Highest priority first, then id (-> store.AllTasks), so capHead keeps what matters most.
				tasks, matched := capHead(tasks, limit)
				// Off the board, since a task carries no owner of its own (-> AgentsByTask).
				holders := map[string]api.TaskHolder{}
				if st, serr := b.State(); serr == nil {
					holders = api.AgentsByTask(st.Agents, st.PRs)
				}
				lines := make([]string, 0, len(tasks))
				for _, t := range tasks {
					lines = append(lines, taskListTable.Line(
						table.Cell{Text: t.ID},
						table.Cell{Text: theme.PriorityLabel(t.Priority)},
						table.Cell{Text: api.TierOrDefault(t.Tier)},
						table.Cell{Text: taskState(t)},
						table.Cell{Text: theme.Age(t.CreatedAt)},
						table.Cell{Text: dash(holders[t.ID].Agent)},
						table.Cell{Text: t.Title},
					))
				}
				printRows(taskListTable, lines)
				// The same wording the agent's own `task list` uses (-> api.TaskListSummary).
				if len(all) == 0 {
					fmt.Fprintln(os.Stderr, "no tasks")
				} else {
					fmt.Fprintln(os.Stderr, api.TaskListSummary(f, len(tasks), all))
				}
				if note := limitNotice("task", len(tasks), matched); note != "" {
					fmt.Fprint(os.Stderr, note)
				}
				// Counted over every task, not the filtered set — a verdict is owed regardless.
				if n := api.CountAwaitingVerdict(all); n > 0 {
					fmt.Fprintf(os.Stderr, "\n%d task(s) await your verdict and no worker can claim them: "+
						"`sindri task approve <id>` (--subtasks clears the tree below it), or `sindri task reject <id> <why>`.\n", n)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output tasks as JSON (machine-readable) instead of the table")
	// Defaults to "active", matching mail list and the TUI — --filter all recovers the whole record.
	c.Flags().StringVar(&filter, "filter", string(api.FilterActive),
		"which tasks to list: "+api.TaskFilterNames()+" (active = open, plus anything closed within "+
			api.ActiveWindow.String()+")")
	c.Flags().IntVar(&limit, "limit", DefaultListLimit, "show at most this many, highest priority first (0 = no limit)")
	return c
}

func taskInfoCmd() *cobra.Command {
	var refresh bool
	var limit int
	c := &cobra.Command{
		Use: "info <id>", Short: "Show a task", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				// An upstream comment added since the last sync is otherwise unreachable from here.
				if refresh {
					if err := b.RefreshTaskComments(args[0]); err != nil {
						return err
					}
				}
				t, err := b.TaskInfo(args[0])
				if err != nil {
					return err
				}
				// The same rule the dashboard's detail pane uses, off the board — a task owns no agent.
				st, err := b.State()
				if err != nil {
					return err
				}
				// Named with its relationship, as the TUI pane is: holding a hierarchy and working a leaf
				// inside it are both true of one agent at once, and a bare name says neither.
				agent := ""
				if h := api.AgentOnTask(st.Agents, st.PRs, t.ID); h.Agent != "" {
					agent = h.Agent + " — " + theme.TaskRelationLabel(h.Rel)
				}
				// The same fields the TUI pane and the agent's `task <id>` show.
				fmt.Printf("id:       %s\ntitle:    %s\nstatus:   %s\ntype:     %s\npriority: %s\ntier:     %s\nparent:   %s\nagent:    %s\napproval: %s\nlabels:   %s\nurl:      %s\n",
					t.ID, t.Title, t.Status, dash(t.Type), theme.PriorityLabel(t.Priority), api.TierOrDefault(t.Tier),
					dash(t.ParentID), dash(agent), dash(theme.ApprovalLabel(t.Approval)), dash(t.Labels), dash(t.URL))
				// Exact, where the list rounds; "changed" is the field the active filter reads.
				fmt.Printf("created:  %s\nchanged:  %s\n", theme.When(t.CreatedAt), theme.When(t.UpdatedAt))
				if body := strings.TrimRight(t.Description, "\n"); body != "" {
					fmt.Printf("\n%s\n", body)
				}
				// The description stays whole; a long thread is capped like every other listing now.
				comments, total := capTail(t.Comments, limit)
				for _, c := range comments {
					fmt.Printf("\n— %s (%s, %s)\n%s\n", dash(c.Author), c.Source, c.CreatedAt,
						strings.TrimRight(c.Body, "\n"))
				}
				if note := limitNotice("comment", len(comments), total); note != "" {
					fmt.Fprint(os.Stderr, note)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "re-pull the comment thread from its source first (a GitHub issue's upstream replies)")
	c.Flags().IntVar(&limit, "limit", DefaultListLimit, "show at most this many of the newest comments (0 = no limit)")
	return c
}

func taskNewCmd() *cobra.Command {
	var typ, priority, tier, parent, labels, desc string
	c := &cobra.Command{
		Use: "new <title...>", Short: "Create a task", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				id, err := b.CreateTask(api.TaskSpec{
					Title: strings.Join(args, " "), Type: typ, Priority: priority, Tier: tier,
					Parent: parent, Description: desc, Labels: splitCSV(labels),
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "created %s\n", id)
				return nil
			})
		},
	}
	taskSpecFlags(c, &typ, &priority, &tier, &parent, &labels, &desc)
	return c
}

// editReport says where the task stands after an edit, read back from the hub. An echo of the
// request would claim whatever was asked for: a mirrored task keeps its content at its own source
// and takes only the fields sindri owns, so some flags land and others cannot.
func editReport(b backend, id string) string {
	t, err := b.TaskInfo(id)
	if err != nil {
		return fmt.Sprintf("edited %s, but reading it back failed: %v", id, err)
	}
	return fmt.Sprintf("%s is now: %s  [%s]  priority=%s tier=%s", id, t.Title, t.Status,
		dash(t.Priority), dash(t.Tier))
}

func taskEditCmd() *cobra.Command {
	var typ, priority, tier, parent, labels, desc, title string
	c := &cobra.Command{
		Use: "edit <id>", Short: "Edit a task (only the flags you pass are changed)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.EditTask(args[0], api.TaskSpec{
					Title: title, Type: typ, Priority: priority, Tier: tier,
					Parent: parent, Description: desc, Labels: splitCSV(labels),
				}); err != nil {
					return err
				}
				// Read back rather than echoing the request: a mirrored task takes only what sindri
				// owns, so "edited os-36fcbb" was printed over a tier that never moved.
				fmt.Fprintln(os.Stderr, editReport(b, args[0]))
				return nil
			})
		},
	}
	c.Flags().StringVar(&title, "title", "", "new title")
	taskSpecFlags(c, &typ, &priority, &tier, &parent, &labels, &desc)
	return c
}

func taskSpecFlags(c *cobra.Command, typ, priority, tier, parent, labels, desc *string) {
	c.Flags().StringVarP(typ, "type", "t", "", "issue type: bug, feature, task, epic, chore (default: task)")
	c.Flags().StringVarP(priority, "priority", "p", "", "priority: P0, P1, P2, P3, P4 (P0 highest; high/medium/low also accepted)")
	c.Flags().StringVar(tier, "tier", "", "difficulty tier: junior, mid, senior (default: mid)")
	c.Flags().StringVar(parent, "parent", "", "parent task id (creates a child)")
	c.Flags().StringVarP(desc, "desc", "d", "", "description body")
	c.Flags().StringVar(labels, "labels", "", "comma-separated labels")
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// nextIsAboutPRs reports whether a role's assignment answer is PRs rather than tasks.
func nextIsAboutPRs(role string) bool { return role == "reviewer" }

// wrongNounRefusal names the right door when a `next` command answered for the other pool.
func wrongNounRefusal(role, agent string) error {
	if nextIsAboutPRs(role) {
		return fmt.Errorf("%s is a reviewer, and is offered PRs rather than tasks — ask `sindri pr next --agent %s`", agent, agent)
	}
	return fmt.Errorf("%s is a %s, and is served tasks rather than PRs — ask `sindri task next --agent %s`", agent, role, agent)
}
