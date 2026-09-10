// package: hub/flow/task / planner_act
// type:    logic (the planner's verb surface)
// job:     the verbs a planner drives the backlog with — read it as a tree (task list,
// task <id>), propose work into it (create-task, parented so a proposal joins an
// existing change or task), and set its own resting state.
// limits:  command surface only; task creation itself is task_act.go's (CreateTask) and
// persistence the store's.
package task

import (
	"fmt"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/prompts"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// AssignPlan hands a planner one thing to plan, as a phased brief (-> prompts.MsgPlanAssignment). Refused
// while it has a PR open, since it drafts on one standing branch.
func (a *Act) AssignPlan(project, agent, goal, taskID string) error {
	goal, taskID = strings.TrimSpace(goal), strings.TrimSpace(taskID)
	if goal == "" && taskID == "" {
		return fmt.Errorf("say what to plan: a goal, question or feature to work out, or a task to work up")
	}
	ps := a.Store.For(project)
	ag, ok, err := ps.GetAgent(agent)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no such agent %q", agent)
	}
	if ag.Role != "planner" {
		return fmt.Errorf("%s is a %s — planning is a planner's job (`sindri agent new --role planner`)", agent, ag.Role)
	}
	if pr, found, perr := a.openPlannerPR(ps, agent); perr != nil {
		return perr
	} else if found {
		return fmt.Errorf("%s still has %s open — merge it (`sindri pr merge %s`) or scrap it "+
			"(`sindri pr scrap %s --yes`) before assigning a new plan, or the new work would be "+
			"drafted on top of specs nobody has ruled on yet", agent, pr.ID, pr.ID, pr.ID)
	}

	subject, err := a.planSubject(ps, taskID, goal)
	if err != nil {
		return err
	}
	// Interrupt first: the directive has to land on an idle prompt, or it queues behind whatever
	// the agent is already doing and arrives after the work it was meant to redirect.
	if a.Harness.Probe(project, agent).Up {
		_ = a.Harness.Interrupt(project, agent)
	}
	brief := prompts.MsgPlanAssignment(subject, taskID, a.Deps.ArchitectureDoc(project), a.planReading(project))
	if err := a.Harness.Say(project, agent, brief, mail.MailAndPush); err != nil {
		return err
	}
	st, _ := ps.GetState(agent)
	// A brief is a planner's claim: it reads the code and the backlog to work one out, which is the
	// same vantage point a worker's task gives (-> store.GrantNotes).
	if err := ps.GrantNotes(agent, prompts.NotesPerClaim); err != nil {
		return err
	}
	st.Agent, st.Phase = agent, "planning"
	_ = ps.SetState(st, store.ReasonClaimed, "Assigned to plan: "+subject)
	_ = ps.Log(agent, "plan", subject)
	a.Deps.Notify()
	return nil
}

// planSubject resolves what the planner is being handed, quoting a task's own title and body rather
// than asking the user to retype them, and moves it to "pending approval" while it's worked out.
func (a *Act) planSubject(ps *store.ProjectStore, taskID, goal string) (string, error) {
	if taskID == "" {
		return goal, nil
	}
	t, ok, err := ps.OwnedTask(taskID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is not a task this project owns — a planner works up sindri's own tasks", taskID)
	}
	if err := ps.SetApproval(taskID, "pending", ""); err != nil {
		return "", err
	}
	subject := t.Title
	if body := strings.TrimSpace(t.Description); body != "" {
		subject += "\n\n" + body
	}
	if goal != "" {
		subject += "\n\nThe user adds: " + goal
	}
	return subject, nil
}

// planReading is the project's configured reading list, as one /workspace-rooted phrase for the
// assignment. Empty when unconfigured, and the phase simply drops out.
func (a *Act) planReading(project string) string {
	cfg, err := a.Deps.ProjectConfig(project)
	if err != nil || len(cfg.Reading) == 0 {
		return ""
	}
	paths := make([]string, 0, len(cfg.Reading))
	for _, p := range cfg.Reading {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, "/workspace/"+strings.TrimPrefix(p, "/"))
		}
	}
	return strings.Join(paths, ", ")
}

// openPlannerPR is the planner's own PR still awaiting a verdict, if any.
func (a *Act) openPlannerPR(ps *store.ProjectStore, agent string) (store.PR, bool, error) {
	prs, err := ps.PRs()
	if err != nil {
		return store.PR{}, false, err
	}
	for _, pr := range prs {
		if pr.Agent == agent && pr.Status != "merged" && pr.Status != "scrapped" {
			return pr, true, nil
		}
	}
	return store.PR{}, false, nil
}

// CmdState lets a planner flip its own resting state between "planning" and "idle".
func (a *Act) CmdState(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) != 1 || (args[0] != "planning" && args[0] != "idle") {
		fmt.Fprintln(out, "usage: state <planning|idle>")
		return 2, nil
	}
	ps := a.Store.For(c.Project)
	st, _ := ps.GetState(c.Agent)
	st.Agent, st.Phase = c.Agent, args[0]
	// Both values are the planner's OWN resting labels (CmdState's own doc), never new work claimed —
	// so this is a release either way, whichever of the two names it settles on.
	if err := ps.SetState(st, store.ReasonFreed, "planner set its own state to "+args[0]); err != nil {
		return 1, err
	}
	a.Deps.Notify()
	fmt.Fprintf(out, "state: %s\n", args[0])
	return 0, nil
}

// CmdCreateTask lets a planner propose a task, flagged pending the user's approval.
func (a *Act) CmdCreateTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	spec, words, perr := ParseTaskFlags(args)
	if perr != nil {
		fmt.Fprintf(out, "%v\n%s\n", perr, CreateTaskUsage)
		return 2, nil
	}
	spec.Title = strings.TrimSpace(strings.Join(words, " "))
	if spec.Title == "" {
		fmt.Fprintln(out, CreateTaskUsage)
		return 2, nil
	}
	if spec.Type == "" {
		spec.Type = "task"
	}
	// Applied AFTER the approval row: a rated task with no approval row is claimable, opening a
	// window where a worker could take work the user has not seen.
	proposed := spec.Priority
	spec.Priority = ""
	id, err := a.CreateTask(c.Project, spec)
	if err != nil {
		// A rejected parent is the caller's to fix, so it goes to them rather than to the
		// operator's log as an internal fault.
		fmt.Fprintf(out, "could not create the task: %v\n", err)
		return 1, nil
	}
	if err := a.Store.For(c.Project).SetApproval(id, "pending", ""); err != nil {
		return 1, err
	}
	if proposed != "" {
		if err := a.writePriority(c.Project, id, proposed); err != nil {
			return 1, err
		}
		a.RefreshCachedTask(c.Project, id)
	}
	nudge := ""
	if spec.Parent == "" {
		nudge = a.unparentedNudge(c.Project, c.Agent, id)
	}
	_ = a.Store.For(c.Project).Log(c.Agent, createTaskLogType, id)
	a.Deps.Notify()
	fmt.Fprintln(out, prompts.ReplyTaskProposed(id, spec.Title, nudge))
	return 0, nil
}

// createTaskLogType tags a proposal's log entry for unparentedNudge to find; an edit is never
// logged under it, so reparenting can never look like another flat proposal.
const createTaskLogType = "create-task"

// recentUnparentedWindow bounds how far back unparentedNudge looks — recent context, not the
// planner's whole history of flat tasks it may have long since tidied up.
const recentUnparentedWindow = 5

// unparentedNudge names this agent's OTHER recent proposals still without a parent right now,
// checked live so a task reparented since drops off the list on its own.
func (a *Act) unparentedNudge(project, agent, justCreated string) string {
	ps := a.Store.For(project)
	events, err := ps.Events(agent, recentUnparentedWindow)
	if err != nil {
		return ""
	}
	var recent []string
	for _, ev := range events {
		if ev.Type != createTaskLogType || ev.Payload == justCreated {
			continue
		}
		t, ok, err := ps.GetTask(ev.Payload)
		if err != nil || !ok || t.ParentID != "" {
			continue
		}
		recent = append(recent, ev.Payload)
	}
	if len(recent) == 0 {
		return ""
	}
	return fmt.Sprintf(" You also proposed %s without a parent recently — if they're related, propose "+
		"a container and hang them under it.", prompts.FileList(recent))
}

// CreateTaskUsage is the one description of create-task's surface, shown for a bad flag, a
// missing title, and (via CreateTaskHelp) `create-task --help`.
const CreateTaskUsage = "usage: create-task [--parent <id>] [--type <task|feature|bug|epic>] [--body <text>] [--labels a,b] [--priority <critical|high|mid|low|none>] [--tier <junior|mid|senior>] <title...>\n" +
	"  --parent    the default for related work, not a special case: propose the container first,\n" +
	"              then each piece with --parent pointed at it, so it joins that tree\n" +
	"  --body      the task's description — what a worker needs in order to start\n" +
	"  --priority  the order you propose this is worked in; `prioritise-task` changes it afterwards\n" +
	"  --tier      your estimate of the difficulty (default: mid); `edit-task` changes it afterwards\n" +
	"Approval answers \"have I read this?\" — it is the user's record of what they have seen, which is\n" +
	"why an edit to a task returns it for a fresh one. Priority answers \"do I want this worked now?\"\n" +
	"— their control over pacing. Neither is a guard against you: they are the user's levers over\n" +
	"their own attention, and a task stays unclaimable until they have pulled both."

// CreateTaskHelp is what the command registry advertises for create-task, so the verb list
// and the verb's own usage describe one surface.
const CreateTaskHelp = "propose a new task, needing the user's approval. " + CreateTaskUsage

// ParseTaskFlags splits create-task's flags (`--flag value` or `--flag=value`) from the title
// words. An unknown flag errors rather than being silently dropped.
func ParseTaskFlags(args []string) (api.TaskSpec, []string, error) {
	var s api.TaskSpec
	var words []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			words = append(words, arg)
			continue
		}
		name, inline, hasInline := strings.Cut(arg, "=")
		val := inline
		if !hasInline {
			if i+1 >= len(args) {
				return s, nil, fmt.Errorf("%s needs a value", name)
			}
			i++
			val = args[i]
		}
		switch name {
		case "--parent", "-P":
			s.Parent = val
		case "--type", "-t":
			s.Type = val
		case "--body", "--description", "-d":
			s.Description = val
		case "--labels", "-l":
			s.Labels = strings.Split(val, ",")
		case "--priority", "-p":
			code, known := api.ParsePriority(val)
			if !known {
				return s, nil, fmt.Errorf("unknown priority %q — one of: %s", val, strings.Join(api.PriorityWords, ", "))
			}
			s.Priority = code
		case "--tier", "-T":
			tier, known := api.ParseTier(val)
			if !known {
				return s, nil, fmt.Errorf("unknown tier %q — one of: %s", val, strings.Join(api.TierWords, ", "))
			}
			s.Tier = tier
		default:
			return s, nil, fmt.Errorf("unknown flag %q", name)
		}
	}
	return s, words, nil
}

// editTaskUsage is the one description of edit-task's surface.
const editTaskUsage = "usage: edit-task <id> [--parent <id>] [--type <task|feature|bug|epic>] [--body <text>] [--labels a,b] [--tier <junior|mid|senior>] [<new title...>]\n" +
	"  --parent  hang this task under another task or openspec change — how a set of flat\n" +
	"            proposals becomes a tree: propose the parent, then point each child at it\n" +
	"  --tier    revise your difficulty estimate; unlike priority, tier is changed here directly\n" +
	"  Any task you can see, whether or not the user has approved it. An edit returns the task\n" +
	"  to the user for a fresh verdict, which also holds it out of the claim pools until they\n" +
	"  have seen the change. Omitted fields are left as they are; the order work is done in is\n" +
	"  `prioritise-task`'s."

// EditTaskHelp is what the command registry advertises for edit-task.
const EditTaskHelp = "revise any task, returning it to the user for re-approval. " + editTaskUsage

// CmdEditTask revises any task, approved or not. ONE consequence regardless of field: it returns to
// awaiting-review, since approval records having READ the task, not permission the planner holds.
func (a *Act) CmdEditTask(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, editTaskUsage)
		return 2, nil
	}
	id := args[0]
	spec, words, perr := ParseTaskFlags(args[1:])
	if perr != nil {
		fmt.Fprintf(out, "%v\n%s\n", perr, editTaskUsage)
		return 2, nil
	}
	spec.Title = strings.TrimSpace(strings.Join(words, " "))
	if spec.Priority != "" {
		// One field, two verbs with opposite consequences: an edit un-approves and a re-ordering
		// must not (-> CmdPrioritiseTask), so a rating here would mean whichever was reached for.
		fmt.Fprintf(out, "edit-task doesn't rate a task — `prioritise-task %s <%s>` does, and it leaves the "+
			"user's approval standing.\n", id, strings.Join(api.PriorityWords, "|"))
		return 2, nil
	}
	if spec.Title == "" && spec.Parent == "" && spec.Type == "" && spec.Tier == "" && spec.Description == "" && len(spec.Labels) == 0 {
		fmt.Fprintf(out, "nothing to change on %s\n%s\n", id, editTaskUsage)
		return 2, nil
	}
	ps := a.Store.For(c.Project)
	// Read the row before the write: it is half of what changed, and gone once the edit lands. It
	// also answers whether the task exists — the approval read here refused an absent id by accident.
	before, ok, err := ps.GetTask(id)
	if err != nil {
		return 1, err
	}
	if !ok {
		fmt.Fprintf(out, "no such task %q\n", id)
		return 1, nil
	}
	if err := a.EditTask(c.Project, id, spec); err != nil {
		// A rejected parent (unknown id, or a cycle) is the caller's to fix.
		fmt.Fprintf(out, "could not edit %s: %v\n", id, err)
		return 1, nil
	}
	after, _, err := ps.GetTask(id)
	if err != nil {
		return 1, err
	}
	changed := taskChanges(before, after)
	if len(changed) == 0 {
		fmt.Fprintf(out, "nothing changed on %s%s\n", id, unownedNote(ps.OwnsTask(id)))
		return 0, nil
	}
	// The verdict being cleared, carried into the record first: a rejection's reason lives in the
	// approval row and nowhere else, and this is the write that would erase it.
	verdict, why := ps.GetApproval(id)
	if err := ps.SetApproval(id, "pending", ""); err != nil {
		return 1, err
	}
	a.RefreshCachedTask(c.Project, id)
	_ = ps.Log(c.Agent, "edit-task", id+" — "+fieldNames(changed))
	a.Deps.Notify()
	// The holder first: it is the reader who acts on this, and it must not be the one left untold
	// on the path where the record fails.
	told := a.tellHolder(c.Project, id, changed)
	// Recorded on the task itself, where the user reads it: "this was edited" without the what is
	// not a record, and the planner's activity log is not where anyone looks for a task's history.
	if cerr := a.Deps.AddTaskComment(c.Project, id, c.Agent, editRecord(changed, verdict, why)); cerr != nil {
		fmt.Fprintf(out, "%s was edited (%s) and is back awaiting the user's approval, but recording "+
			"what changed on it failed: %v — the task carries no record of it, so say what changed in "+
			"the meeting room.%s\n", id, fieldNames(changed), cerr, told)
		return 1, nil
	}
	fmt.Fprintf(out, "%s updated (%s) — back to awaiting the user's approval, so it stays out of the claim "+
		"pools until they have seen the change.%s\n", id, fieldNames(changed), told)
	return 0, nil
}

// unownedNote explains an edit that wrote nothing: a mirrored task's content belongs to its own
// source, and what sindri keeps for it is where it sits in the tree.
func unownedNote(owned bool) string {
	if owned {
		return " — it already reads that way."
	}
	return ": its own source keeps its content, and what sindri owns for such a task is its place in " +
		"the tree, so `--parent` is what edit-task changes here."
}

// taskChange is one field an edit moved, in parts: a reply names fields, the record carries values.
type taskChange struct{ field, was, now string }

// taskChanges is what an edit ACTUALLY moved, read off the stored rows either side of the write —
// echoing the request back would report a change to a field the task's own source owns.
func taskChanges(before, after store.Task) []taskChange {
	all := []taskChange{
		{"title", before.Title, after.Title},
		{"type", before.Type, after.Type},
		{"tier", before.Tier, after.Tier},
		{"labels", before.Labels, after.Labels},
		{"parent", before.ParentID, after.ParentID},
		{"description", before.Description, after.Description},
	}
	var out []taskChange
	for _, ch := range all {
		if ch.was != ch.now {
			out = append(out, ch)
		}
	}
	return out
}

// fieldNames lists which fields moved — the short form, for a reply and a log line.
func fieldNames(changes []taskChange) string {
	names := make([]string, 0, len(changes))
	for _, ch := range changes {
		names = append(names, ch.field)
	}
	return strings.Join(names, ", ")
}

// editRecord is the comment left on the edited task: each field that moved, with the value it held
// before. That value survives nowhere else, and it is what answers "what changed".
func editRecord(changes []taskChange, verdict, why string) string {
	var b strings.Builder
	b.WriteString("edited " + fieldNames(changes) + ", and returned to you for approval.\n")
	if verdict != "" {
		fmt.Fprintf(&b, "\nThe verdict this replaces: %s%s\n", verdict, prefixed(" — ", why))
	}
	for _, ch := range changes {
		fmt.Fprintf(&b, "\n%s was:\n%s\n\n%s is now:\n%s\n", ch.field, dash(ch.was), ch.field, dash(ch.now))
	}
	return b.String()
}

// prefixed joins sep and s, or nothing at all when s is empty.
func prefixed(sep, s string) string {
	if s == "" {
		return ""
	}
	return sep + s
}

// tellHolder tells every agent whose UNIT OF WORK the edit touches, not just the holder of the
// edited row — a worker holding the enclosing feature must hear about it too.
func (a *Act) tellHolder(project, id string, changes []taskChange) string {
	ps := a.Store.For(project)
	roster, err := ps.Roster()
	if err != nil {
		return ""
	}
	units := enclosing(ps, id)
	var told []string
	for _, ag := range roster {
		st, _ := ps.GetState(ag.Name)
		var unit string
		switch {
		case units[st.Task]:
			unit = st.Task // the subtask it is on, or the edit landed below that
		case units[st.Container]:
			unit = st.Container // the feature the edit sits inside
		default:
			continue
		}
		told = append(told, a.tellOne(project, ag.Name, id, unit, fieldNames(changes)))
	}
	return strings.Join(told, "")
}

// enclosing is the edited task and every task above it, any depth — an edit three levels down is
// still an edit to the feature at the top. Stops on a stored loop rather than spinning.
func enclosing(ps *store.ProjectStore, id string) map[string]bool {
	out := map[string]bool{id: true}
	links, err := ps.ParentLinks()
	if err != nil {
		return out
	}
	for at := links[id]; at != "" && !out[at]; at = links[at] {
		out[at] = true
	}
	return out
}

// tellOne delivers the note to one holder, and says what happened for the planner's own reply.
func (a *Act) tellOne(project, agent, id, unit, fields string) string {
	// A down agent is not waited on (InjectWhenReady would sit there): the record on the task is
	// what reaches it when it comes back.
	if !a.Harness.Probe(project, agent).Up {
		return fmt.Sprintf(" %s holds %s but isn't running — it will read the change on the task.", agent, unit)
	}
	if err := a.Harness.Say(project, agent, prompts.MsgTaskEdited(id, unit, fields), mail.MailOnly); err != nil {
		return fmt.Sprintf(" %s holds %s and could not be told (%v) — say so in the meeting room.", agent, unit, err)
	}
	return fmt.Sprintf(" %s holds %s and was told what changed.", agent, unit)
}
