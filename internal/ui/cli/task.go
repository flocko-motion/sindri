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
	"github.com/flo-at/sindri/internal/ui/theme"
	"github.com/spf13/cobra"
)

// --- task ---

// tasksJSON renders the task rows (their json tags) for machine consumers. It
// always yields a JSON array — never null — so the output parses even when there
// are no tasks.
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

// taskRefreshCmd re-syncs the task cache and notifies watchers. Reads sync on their own, so this is
// for forcing one without listing — e.g. pushing fresh state to a running TUI.
// taskCommentCmd comments on a task. A GitHub issue gets it upstream, so its own readers see it;
// every other kind keeps the thread in the hub.
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

// taskNextCmd answers "why is nothing being assigned" without anyone reading the queries: what
// would be handed out, and where every other open task stands. --role asks it of a role nobody is
// running yet — "would a second worker have anything to pick up" — which otherwise took starting one.
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
				// Answered rather than refused blankly: the noun is the point. A reviewer's pool is
				// PRs, and a command called `task next` has no business claiming otherwise.
				return fmt.Errorf("a reviewer is offered PRs, not tasks — ask `sindri pr next`")
			}
			return withBackend(func(b backend) error {
				x, err := b.NextTask(agent, role)
				if err != nil {
					return err
				}
				// The same rule after the call as before it: a named agent brings its own role, and
				// only the hub knows what that is, so this is where the noun is checked for one.
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

// taskCloseCmd marks a task done — dispatched by backend (td close / openspec archive
// / GitHub issue close).
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

// taskReopenCmd restores a closed sindri-owned task to open, with a required reason — the host's
// counterpart to `task close` (ARCHITECTURE.md's interchangeable-front-ends rule) and a planner's
// `reopen-task`. Refused for a task whose status comes from its own source (openspec, GitHub).
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
				// The one thing to be told rather than discover: a priority left standing from
				// before the close is enough on its own to make this immediately claimable.
				if t, terr := b.TaskInfo(id); terr == nil && t.Priority != "" {
					fmt.Fprintf(os.Stderr, "%s still carries priority %s, so a worker may claim it immediately\n",
						id, theme.PriorityLabel(t.Priority))
				}
				return nil
			})
		},
	}
}

// taskDeleteCmd scraps a task, dispatched by backend. --subtasks widens it down the tree and --prs
// takes the open PRs, the same two shapes the TUI's scrap modal offers.
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

// approvedPriority is the second gate, in the same call. Approving FEELS like releasing work, and it
// is not: an unrated task is claimable by nobody, which is how a dozen approved tasks came to sit in
// the backlog doing nothing. So the rating is either given here or its absence is said out loud — the
// TUI asks in a modal, and this is the same offer where there is nobody to ask.
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
		// Name the rating being authorised. A planner may have proposed the sequence, and the
		// approve is the moment it takes effect — silence here would land work in a worker's lap
		// at an order the user never consciously agreed to.
		if word := priorityOf(all, id); word != "" {
			fmt.Fprintf(os.Stderr, "%s is %s and now claimable\n", id, word)
		}
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s has no priority, so no worker can claim it yet — "+
		"`task priority %s <critical|high|mid|low|none>`, or --priority does both in one call\n", id, id)
	return nil
}

// taskLister is the slice of the backend this needs: what the backlog says, so the advice can be
// about the tree as it stands rather than the one call that just happened.
type taskLister interface{ Tasks() ([]api.Task, error) }

// ratedApproval is the mirror of approvedPriority: a rating on a task the approval gate still holds
// releases nothing, and reporting only success leaves the user to wonder why no worker took it.
// It names the act that would release it rather than performing one — approving is the user's.
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

// priorityOf is the readable priority a task carries itself, or "" — the ancestor's rating that
// also releases it is not this task's sequence to state.
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
				// The same note the TUI's scope modal carries, for the same reason: a rating that
				// reached the children of an open package ordered them, and did not release them.
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

// priorityReach is what a rating on id could carry to; a backlog that can't be read yields nothing to
// say about the tree, which must not turn the rating itself into a failure.
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

// taskState is the word a listing shows for a task: the approval gate where one is set, since that
// is what decides whether the task can be worked, and the status otherwise. A pending task printed
// as plain "open" claimed to be available when no worker could see it.
func taskState(t api.Task) string {
	if t.Approval == "pending" || t.Approval == "rejected" {
		return theme.ApprovalLabel(t.Approval)
	}
	return t.Status
}

func taskListCmd() *cobra.Command {
	var asJSON bool
	var filter string
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
					out, err := tasksJSON(tasks)
					if err != nil {
						return err
					}
					fmt.Println(out)
					return nil
				}
				for _, t := range tasks {
					fmt.Printf("%-12s %-8s %-12s %4s  %s\n", t.ID, theme.PriorityLabel(t.Priority),
						taskState(t), theme.Age(t.CreatedAt), t.Title)
				}
				if n := len(all) - len(tasks); n > 0 {
					// What a filter hid is said out loud: an empty listing under `--filter closed`
					// otherwise reads as "no tasks" when the backlog is full of open ones.
					fmt.Fprintf(os.Stderr, "(filter %s — %d of %d task(s) shown)\n", f, len(tasks), len(all))
				} else if len(tasks) == 0 {
					fmt.Fprintln(os.Stderr, "no tasks")
				}
				// The gate hides these from every worker, so a list that ended here read as a full
				// backlog while nothing in it could be claimed. Counted over every task, not the
				// filtered set: a verdict is owed whether or not this listing shows the task.
				if n := api.CountAwaitingVerdict(all); n > 0 {
					fmt.Fprintf(os.Stderr, "\n%d task(s) await your verdict and no worker can claim them: "+
						"`sindri task approve <id>` (--subtasks clears the tree below it), or `sindri task reject <id> <why>`.\n", n)
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output tasks as JSON (machine-readable) instead of the table")
	// Defaults to "all", which is what the bare command has always printed. The TUI opens on
	// "active" instead: a screen redrawn every few seconds is a view, and a listing is a record.
	c.Flags().StringVar(&filter, "filter", string(api.FilterAll),
		"which tasks to list: "+api.TaskFilterNames()+" (active = open, plus anything closed within "+
			api.ActiveWindow.String()+")")
	return c
}

func taskInfoCmd() *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use: "info <id>", Short: "Show a task", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				// The TUI re-pulls a thread on demand, so the CLI can too: an upstream comment
				// added since the last sync is otherwise unreachable from here.
				if refresh {
					if err := b.RefreshTaskComments(args[0]); err != nil {
						return err
					}
				}
				t, err := b.TaskInfo(args[0])
				if err != nil {
					return err
				}
				// Who is behind it, by the rule the dashboard's detail pane and its row marker use,
				// so the same task never names a different agent in the two front-ends. Read off the
				// board, since a task carries no owner of its own: an agent holds it, or its PR does.
				st, err := b.State()
				if err != nil {
					return err
				}
				agent := api.AgentOnTask(st.Agents, st.PRs, t.ID)
				// The same fields the TUI pane and the agent's `task <id>` show: a front-end
				// chooses layout, not which facts exist, or it answers a different question.
				fmt.Printf("id:       %s\ntitle:    %s\nstatus:   %s\ntype:     %s\npriority: %s\nparent:   %s\nagent:    %s\napproval: %s\nlabels:   %s\nurl:      %s\n",
					t.ID, t.Title, t.Status, dash(t.Type), theme.PriorityLabel(t.Priority),
					dash(t.ParentID), dash(agent), dash(theme.ApprovalLabel(t.Approval)), dash(t.Labels), dash(t.URL))
				// Exact, where the list rounds — and "changed" beside it, the field the active
				// filter reads, so its "n/a" says why a mirrored task can be missing from that view.
				fmt.Printf("created:  %s\nchanged:  %s\n", theme.When(t.CreatedAt), theme.When(t.UpdatedAt))
				if body := strings.TrimRight(t.Description, "\n"); body != "" {
					fmt.Printf("\n%s\n", body)
				}
				// The thread too, for the same reason the fields above are all here: the TUI's pane
				// shows it, so a CLI that omitted it answered a different question.
				for _, c := range t.Comments {
					fmt.Printf("\n— %s (%s, %s)\n%s\n", dash(c.Author), c.Source, c.CreatedAt,
						strings.TrimRight(c.Body, "\n"))
				}
				return nil
			})
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "re-pull the comment thread from its source first (a GitHub issue's upstream replies)")
	return c
}

func taskNewCmd() *cobra.Command {
	var typ, priority, parent, labels, desc string
	c := &cobra.Command{
		Use: "new <title...>", Short: "Create a task", Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				id, err := b.CreateTask(api.TaskSpec{
					Title: strings.Join(args, " "), Type: typ, Priority: priority,
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
	taskSpecFlags(c, &typ, &priority, &parent, &labels, &desc)
	return c
}

func taskEditCmd() *cobra.Command {
	var typ, priority, parent, labels, desc, title string
	c := &cobra.Command{
		Use: "edit <id>", Short: "Edit a task (only the flags you pass are changed)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withBackend(func(b backend) error {
				if err := b.EditTask(args[0], api.TaskSpec{
					Title: title, Type: typ, Priority: priority,
					Parent: parent, Description: desc, Labels: splitCSV(labels),
				}); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "edited %s\n", args[0])
				return nil
			})
		},
	}
	c.Flags().StringVar(&title, "title", "", "new title")
	taskSpecFlags(c, &typ, &priority, &parent, &labels, &desc)
	return c
}

func taskSpecFlags(c *cobra.Command, typ, priority, parent, labels, desc *string) {
	c.Flags().StringVarP(typ, "type", "t", "", "issue type: bug, feature, task, epic, chore (default: task)")
	c.Flags().StringVarP(priority, "priority", "p", "", "priority: P0, P1, P2, P3, P4 (P0 highest; high/medium/low also accepted)")
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

// nextIsAboutPRs reports whether a role's assignment answer is PRs rather than tasks — the one
// distinction both `next` commands' nouns turn on.
func nextIsAboutPRs(role string) bool { return role == "reviewer" }

// wrongNounRefusal is what a `next` command says when the hub answered for the other pool: the
// question was legitimate and asked at the wrong door, so it names the door.
func wrongNounRefusal(role, agent string) error {
	if nextIsAboutPRs(role) {
		return fmt.Errorf("%s is a reviewer, and is offered PRs rather than tasks — ask `sindri pr next --agent %s`", agent, agent)
	}
	return fmt.Errorf("%s is a %s, and is served tasks rather than PRs — ask `sindri task next --agent %s`", agent, role, agent)
}
