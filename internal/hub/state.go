// package: hub / state
// type:    logic (the single read surface + change notifications)
// job:     assemble the whole board the UIs render — agents across every project
// with live workflow state, merge-intents, and orphaned runtime, plus the
// tasks of the selected project — and a tiny pub/sub so clients live-update
// over /events. The central store is the read model; this is its projection.
// limits:  read-only assembly + notify; mutations live in their own methods.
package hub

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/commands"
	"github.com/flo-at/sindri/internal/hub/situation"
	"github.com/flo-at/sindri/internal/hub/store"
)

// statsTimeout bounds one `stats` sample, slower than a probe (the runtime samples over a window).
const statsTimeout = 8 * time.Second

// AgentView is an agent as the UIs see it; it crosses the wire, so it is
// internal/api.AgentView under the name every existing caller here already uses.
type AgentView = api.AgentView

// BoardState is the whole board; it crosses the wire, so it is internal/api.BoardState
// under the name every existing caller here already uses.
type BoardState = api.BoardState

// AgentMail is one message in an agent's mailbox; it crosses the wire, so it is internal/api.Mail,
// named here for what it is to the hub.
type AgentMail = api.Mail

// observedAt stamps a reading for the board, "" where nothing has looked yet — which the board must
// not render as a time, since it would read as a look that happened.
func observedAt(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

// stillLabel is how long the display had stood unchanged, "" when nothing was seen to stand still.
func stillLabel(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Round(time.Second).String()
}

// projectsOf is the distinct projects a roster spans, in first-seen order.
func projectsOf(agents []store.Agent) []string {
	seen, out := map[string]bool{}, []string{}
	for _, a := range agents {
		if !seen[a.Project] {
			seen[a.Project] = true
			out = append(out, a.Project)
		}
	}
	return out
}

// State assembles the board; an empty selected tag means no project is chosen, so no tasks.
func (h *Hub) State(selected string) (BoardState, error) {
	agentsRow, err := h.store.AllAgents()
	if err != nil {
		return BoardState{}, err
	}
	prs, err := h.store.AllPRs()
	if err != nil {
		return BoardState{}, err
	}
	// Only registered repos surface in the global views: a forgotten repo's PRs stay in the db (keyed
	// by its stable tag, so re-adding reactivates them) but drop off the fleet tab. Read the registry
	// ONCE — twice doubled load on the single store connection and let one snapshot disagree with itself.
	projects, err := h.projects.Known()
	if err != nil {
		return BoardState{}, err // never render "no repos" from an unreadable registry
	}
	registered := map[string]bool{}
	for _, p := range projects {
		registered[p.Tag] = true
	}
	kept := prs[:0]
	for _, pr := range prs {
		if registered[pr.Project] {
			kept = append(kept, pr)
		}
	}
	prs = kept
	h.fillReviewers(prs)
	h.fillAttempts(prs)
	// Fleet-wide and position-ranked already (-> FleetRuns), so the board never re-derives either.
	runs, err := h.wf.FleetRuns()
	if err != nil {
		return BoardState{}, err
	}
	var tasks []store.Task
	if selected != "" {
		if tasks, err = h.store.For(selected).AllTasks(); err != nil {
			return BoardState{}, err
		}
	}
	// What each repo says about itself — its architecture doc, and whether a task source wants a CLI
	// that isn't installed — from the watchdog's sample. Read here it was a config file and a PATH
	// lookup per repo per render, which is how `sindri task info` came to time out past 120s.
	repos := h.watch.repoDocs()

	// Every per-agent reading comes from the watchdog's last observation — a board read REPORTS them,
	// never takes one. Probing per request scaled cost with readers (overlapping polls, SSE,
	// post-mutation refetches); probes then lost their deadline and rendered as "down", flickering
	// healthy agents (-> watchdog.go).
	obs := make([]liveness, len(agentsRow))
	// observed is carried separately because the zero value of an observation is a CLAIM: an agent
	// registered since the last sweep has been looked at by nothing, and reading its absent
	// observation as "not running" is the same error as trusting a stale listing.
	observed := make([]bool, len(agentsRow))
	for i, a := range agentsRow {
		obs[i], observed[i] = h.watch.get(a.Project, a.Name)
	}
	// The orphan scan wants the pod list, which the sweep takes fleet-wide in one spawn.
	existing := h.watch.pods()

	// One query for the fleet's unread tallies: a count per agent row would be paid per render.
	unreadMail, err := h.store.UnreadMailByAgent()
	if err != nil {
		return BoardState{}, err
	}
	// One gather per PROJECT, not per agent: the claimable pool behind it is one query for a whole
	// roster. Keyed off the ROSTER, so an agent of a forgotten repo still renders.
	sits := map[agentKey]situation.Situation{}
	for _, tag := range projectsOf(agentsRow) {
		roster, serr := h.sit.Roster(tag)
		if serr != nil {
			return BoardState{}, serr
		}
		for _, s := range roster {
			sits[agentKey{tag, s.Name}] = s
		}
	}
	known := map[string]bool{}
	agents := make([]AgentView, 0, len(agentsRow))
	for i, a := range agentsRow {
		// Not named `container`: that shadows the package of the same name, which is how a probe
		// smuggled into this loop would read as a local call rather than a runtime operation.
		pod := h.container(a.Project, a.Name)
		known[pod] = true
		ps := h.store.For(a.Project)
		st, _ := ps.GetState(a.Name)
		// A reviewer authors no PR, so fall back to the one it's reviewing — that's what it works on.
		// store.Store's ReviewingPR: a pooled reviewer's held review is never filed under its own
		// project, so a.Project-scoped alone would show it holding nothing while it plainly is.
		pr := openPRFor(prs, a.Project, a.Name)
		if pr == "" {
			_, pr, _ = h.store.ReviewingPR(a.Project, a.Name)
		}
		l := obs[i]
		sit := sits[agentKey{a.Project, a.Name}]
		allowed := sit.Allowed()
		// The word is the surface's now. Retiring an intent reality has caught up with is still a
		// WRITE, and the board is the caller that owns making it (-> agent.Service.SettleIntent).
		h.agents.SettleIntent(a.Project, a.Name, l.up, observed[i])
		status := allowed.Status
		agents = append(agents, AgentView{
			// Carried, never re-derived: a front-end links no hub package, and deciding it here off
			// `status` would be a second copy inside the hub (-> situation.Surface).
			NeedsUser:  allowed.NeedsUser,
			ObservedAt: observedAt(sit.TakenAt), StillFor: stillLabel(sit.StillFor),
			Project: a.Project, Repo: h.repoName(a.Project), Name: a.Name, Role: a.Role,
			Status:  status,
			Runtime: l.state.String(),
			Task:    st.Task, Feature: st.Container, Branch: st.Branch, PR: pr, Workspace: a.Workspace,
			Clients: l.clients, Container: pod, Memory: a.Memory, Retired: a.Retired,
			ClearArmed:    a.ClearArmed,
			ContextTokens: l.tokens, ContextWindow: l.window, Escalation: st.Escalation,
			// The transcript sees a model switched by hand inside Claude Code before the roster does,
			// so the detected one wins while the agent is up — both readings off the same sample.
			Model:      agent.ModelInUse(a.Model, l.model, l.up),
			UnreadMail: unreadMail[a.Project][a.Name],
		})
	}

	// Orphans: sindri pods with no roster entry, from the listing the liveness probe already took.
	var orphans []string
	for _, p := range existing {
		if !known[p] {
			orphans = append(orphans, p)
		}
	}
	chat, err := h.chatView()
	if err != nil {
		return BoardState{}, err
	}
	// Carried in the snapshot so the TUI's recommendation matches the one hub startup prints.
	docs := make(map[string]RepoDocState, len(projects))
	for _, p := range projects {
		docs[p.Tag] = repos[p.Tag].docs
	}
	mail, mailTotal, mailUnread, mailUnreadUser, unreadByRepo, err := h.mailWindow()
	if err != nil {
		return BoardState{}, err
	}
	board := BoardState{
		RuntimeHint: h.watch.runtimeHint(),
		Agents:      agents, Tasks: tasks, PRs: prs, Runs: runs, Projects: projects, Orphans: orphans, Chat: chat,
		RepoDocs: docs, SpecCLIMissing: repos[selected].specMissing, StartedAt: h.startedAt.UTC().Format(time.RFC3339),
		DefaultMemory: agent.MemoryOrDefault(""),
		// Reported from the watchdog's last reading, like liveness and for the same reason: taking
		// one here would put a process spawn on every board read, and there are many.
		Memory: h.watch.headroom(),
		Mail:   mail, MailTotal: mailTotal, MailUnread: mailUnread, MailUnreadByRepo: unreadByRepo,
		MailUnreadUser: mailUnreadUser,
	}
	return withSections(board), nil
}

// MailWindow bounds how many READ messages the board carries — the mailbox is never pruned, so what
// needs bounding is the render. Every unread message rides along regardless of age (-> AllMail):
// read mail already has an owner who dealt with it, so it is the part safe to let age out of view,
// and the fleet's older read mail is still reached one message at a time (-> MailBody).
const MailWindow = 200

// mailPreview is how much of a body the window carries: a rejection arrives with its whole findings,
// so a row carries an opening and says it was cut rather than putting hundreds of lines on the board.
const mailPreview = 240

// mailWindow reads the board's mail — every unread message plus the newest read ones filling out to
// MailWindow (-> AllMail) — each body cut to a preview, plus the tallies of the WHOLE mailbox: the
// total, the unread count, and unread per repo for a repo-scoped view.
func (h *Hub) mailWindow() (window []AgentMail, total, unread, userUnread int, unreadByRepo map[string]int, err error) {
	if window, err = h.store.AllMail(MailWindow); err != nil {
		return nil, 0, 0, 0, nil, err
	}
	for i, m := range window {
		window[i].Repo = h.repoName(m.Project)
		if len(m.Body) > mailPreview {
			window[i].Body, window[i].Truncated = m.Body[:mailPreview], true
		}
	}
	if total, unread, userUnread, unreadByRepo, err = h.store.MailTallies(); err != nil {
		return nil, 0, 0, 0, nil, err
	}
	return window, total, unread, userUnread, unreadByRepo, nil
}

// MailBody returns one message with its full body — what a detail view or `mail show` asks for. A
// PURE read: marking is a separate, deliberate act (-> MarkMailReadForUser), not a side effect of a look.
func (h *Hub) MailBody(id int64) (AgentMail, bool, error) {
	m, ok, err := h.store.MailByID(id)
	if err != nil || !ok {
		return m, ok, err
	}
	// The lifecycle rides along here and nowhere else: a listing wants the state, and only somebody
	// asking about ONE message is asking what became of it (-> api.Mail.History).
	m.History, _ = h.store.For(m.Project).MailEvents(id)
	return m, true, nil
}

// MarkMailReadForUser marks one message read, but ONLY when addressed to the user — the one
// deliberate act (a dwell, an ENTER, `mail show`) that may retire a message from the Mail tab.
func (h *Hub) MarkMailReadForUser(id int64) error {
	m, ok, err := h.store.MailByID(id)
	if err != nil || !ok || m.Read() || !api.MailToUser(m) {
		return err
	}
	if err := h.store.For(m.Project).MarkMailRead(id); err != nil {
		return err
	}
	h.notify()
	return nil
}

// withSections stamps the board with its own tabs — each count, and how many of its rows wait on
// the user — resolved against the board they describe. A front-end renders what it finds here, so
// a board that left this out would silently drop every marker.
func withSections(b BoardState) BoardState {
	b.Sections = commands.Resolved(b)
	return b
}

// fillReviewers stamps each PR with the agent holding an open review of it. Whether a PR is being
// looked at, and by whom, is the hub's answer: a front-end deriving it from the reviews would be
// deciding rather than rendering, and the two would drift the first time the rule moved.
func (h *Hub) fillReviewers(prs []api.PR) {
	byProject := map[string]map[string]string{}
	for i, pr := range prs {
		active, ok := byProject[pr.Project]
		if !ok {
			var err error
			if active, err = h.store.For(pr.Project).ActiveReviewers(); err != nil {
				log.Printf("hub: active reviewers for %s: %v", pr.Project, err)
				active = map[string]string{}
			}
			byProject[pr.Project] = active
		}
		prs[i].Reviewer = active[pr.ID]
	}
}

// fillAttempts stamps each PR with which submission of it is standing, counted from its own events.
// Per project and in one query, as fillReviewers is: every PR list wants it, so a lookup per row
// would be paid on every render.
func (h *Hub) fillAttempts(prs []api.PR) {
	byProject := map[string]map[string]int{}
	for i, pr := range prs {
		counts, ok := byProject[pr.Project]
		if !ok {
			var err error
			if counts, err = h.store.For(pr.Project).SubmitCounts(); err != nil {
				log.Printf("hub: submit counts for %s: %v", pr.Project, err)
				counts = map[string]int{}
			}
			byProject[pr.Project] = counts
		}
		prs[i].Attempt = counts[pr.ID]
	}
}

// AgentStatsView is one agent's resource snapshot; it crosses the wire, so it is
// internal/api.AgentStatsView under the name every existing caller here already uses.
type AgentStatsView = api.AgentStatsView

// StatsReport is the `agent stats` payload; it crosses the wire, so it is
// internal/api.StatsReport under the name every existing caller here already uses.
type StatsReport = api.StatsReport

// Stats returns the engine name and a resource snapshot for every running agent.
func (h *Hub) Stats(ctx context.Context) (StatsReport, error) {
	views, err := h.AllStats(ctx)
	return StatsReport{Engine: container.Name(), Agents: views}, err
}

// AllStats snapshots every RUNNING agent concurrently — each sample is slow, so serial would be N×that.
// Down agents are omitted; a per-agent failure lands in that row's Err rather than being dropped.
func (h *Hub) AllStats(ctx context.Context) ([]AgentStatsView, error) {
	agentsRow, err := h.store.AllAgents()
	if err != nil {
		return nil, err
	}
	views := make([]AgentStatsView, len(agentsRow))
	var wg sync.WaitGroup
	for i, a := range agentsRow {
		wg.Add(1)
		go func(i int, a store.Agent) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, statsTimeout)
			defer cancel()
			c := h.container(a.Project, a.Name)
			if !container.RunningContext(ctx, c) {
				return // down — no VM to sample; filtered out below (Name stays "")
			}
			v := AgentStatsView{Name: a.Name, Repo: h.repoName(a.Project)}
			if s, serr := container.Stats(ctx, c); serr != nil {
				v.Err = serr.Error()
			} else {
				v.MemUsageBytes, v.MemLimitBytes = s.MemoryUsageBytes, s.MemoryLimitBytes
			}
			views[i] = v
		}(i, a)
	}
	wg.Wait()

	out := make([]AgentStatsView, 0, len(views))
	for _, v := range views {
		if v.Name != "" { // running agents only
			out = append(out, v)
		}
	}
	return out, nil
}

// projectPath resolves a project tag to its path, logging loudly on a real store error (as opposed to
// an unknown project) — the string-returning callers can't thread one, so it must surface here.
func (h *Hub) projectPath(project string) (string, bool) {
	path, ok, err := h.store.ProjectPath(project)
	if err != nil {
		log.Printf("hub: resolve project path for %q failed: %v", project, err)
	}
	return path, ok
}

// repoName is a project's directory name from the registry, falling back to the tag.
func (h *Hub) repoName(project string) string {
	if path, ok := h.projectPath(project); ok {
		return filepath.Base(path)
	}
	return project
}

// container is the podman container name for an agent, resolved via the registry.
func (h *Hub) container(project, name string) string {
	root, _ := h.projectPath(project)
	return Container(root, name)
}

// overlayUnreachable says "unreachable" where pushes stopped showing up in an agent's pane. Last of
// all, over a stall or an escalation: those name what an agent waits for, this says it cannot be told
// anything either. Three words outrank it, each naming a remedy where this one names none: not-up,
// signed-out, and blocked — whose remedy IS a message, into a dialog that draws none of what it takes.
func overlayUnreachable(status string, unreachable bool) string {
	if !unreachable || api.AgentNotUp(status) || status == api.StatusSignedOut || status == api.StatusBlocked {
		return status
	}
	return api.StatusUnreachable
}

// Refresh re-syncs tasks and notifies watchers; being the user's explicit refresh it forces the
// GitHub scan past its TTL.
func (h *Hub) Refresh(project string) error {
	err := h.wf.ForceSyncTasks(project)
	h.notify()
	return err
}

// Log returns an agent's recent activity-log entries (oldest-first).
func (h *Hub) Log(project, name string) ([]store.Event, error) {
	return h.store.For(project).Events(name, 50)
}

// StateLog returns an agent's debug state log, newest first — every stored write's reason and every
// distinct derived-status change. Unbounded here: state_log's own write-time cap (-> stateLogCap)
// already bounds it, and the CLI applies its own --limit on top (-> sd-4be9f8's convention).
func (h *Hub) StateLog(project, name string) ([]store.StateEvent, error) {
	return h.store.For(project).StateLog(name, 0)
}

// openPRFor returns the id of an agent's still-open PR in its project, if any. Open-ness is PROpen's
// to define, the same rule the PRs tab lists by — deciding it here instead left a scrapped PR
// attributed to its author on the Agents tab while the PRs tab, correctly, showed nothing.
func openPRFor(prs []store.PR, project, agent string) string {
	for _, p := range prs {
		if p.Project == project && p.Agent == agent && PROpen(p) {
			return p.ID
		}
	}
	return ""
}

// --- change notifications (pub/sub for /events) ---

type bus struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newBus() *bus { return &bus{subs: map[chan struct{}]bool{}} }

// subscribe returns a buffered channel that ticks on every notify, plus an unsubscribe func.
func (b *bus) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = true
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

// publish wakes every subscriber (non-blocking; a full buffer already means "refresh pending").
func (b *bus) publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// notify signals that board state changed (called after every mutation).
func (h *Hub) notify() {
	if h.events != nil {
		h.events.publish()
	}
}
