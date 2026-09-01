// package: hub/situation / situation
// job:     gather everything the hub knows about one agent into a single value — its roster row,
// its workflow state, the observer's last reading and what its project could hand it — so the
// rules deciding what may happen to it (-> Surface) all read one thing.
// type:    logic (where an agent stands, assembled once)
// limits:  gathering only; it decides nothing and it never touches the container runtime.
package situation

import (
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/store"
)

// Reading is the observer's last look at an agent. Zero where nothing has looked yet, which Observed
// tells apart from a genuine "down" — reading an absent observation as down is how a freshly
// registered agent came to be reported dead.
type Reading struct {
	Observed  bool
	Up        bool
	Status    string // the board's own word for it (-> api.AgentView.Status)
	Runtime   string // Claude's live runtime: working|blocked|idle|signed-out|api-error|""
	Clients   int    // humans dialed into its session
	StillFor  time.Duration
	Fill      int // context tokens in use, and the window they fill
	Window    int
	Launching bool
}

// Observer hands over that reading. A port because the situation must not probe: the watchdog holds
// all of this in memory already, and a fresh look would put a container call behind every rule.
type Observer interface {
	Reading(project, name string) Reading
}

// Pool is what a project could hand out right now. It describes the PROJECT rather than any one
// agent, so it is read once for a whole roster and every situation in that roster refers to this
// one reading — a per-agent read would turn one query into one per agent on every sweep.
type Pool struct {
	Packages []store.Task
	Leaves   []store.Task
	All      []store.Task
}

// GatedUnder is open work under a feature still awaiting a verdict, at any depth — the reason a
// feature worker can be idle with its own tree unfinished.
func (p Pool) GatedUnder(container string) []store.Task {
	var out []store.Task
	for _, d := range api.Descendants(p.All, container) {
		if api.Open(d) && d.Approval == "pending" {
			out = append(out, d)
		}
	}
	return out
}

// Situation is where one agent stands: the whole state the hub has about it, in one value. Every
// rule about what may happen to that agent reads this and nothing else (-> Surface).
type Situation struct {
	Project string
	Name    string
	Reading

	// The roster row — what a human has decided about this agent.
	Role       string
	Retired    bool
	ClearArmed bool
	Stopped    bool

	// The workflow state — what the hub has given it.
	Phase      string
	Task       string
	Container  string
	Branch     string
	Escalation string
	LastNudge  string

	// Holds that live outside the state row: a PR of its own still to land, a review it is reading,
	// and whether the feature it holds has already gone in without it.
	AwaitingPR   string
	AwaitingTask string
	ReviewingPR  string
	// ReviewOpen: that review's PR is still open. A review row outlives its PR, and one whose PR has
	// left "open" is released by the reviewer's own next ask — it holds nothing in any sense that
	// stops it being handed something else.
	ReviewOpen    bool
	FeatureLanded bool
	WaitingOnRun  bool // queued behind the fleet's shared run gate, not idling on its own account

	Pool Pool
}

// Gatherer assembles situations. One per hub, holding the store and the observer so a caller passes
// nothing but an identity.
type Gatherer struct {
	store *store.Store
	obs   Observer
}

// NewGatherer builds one over the hub's store and its observer.
func NewGatherer(st *store.Store, obs Observer) *Gatherer {
	return &Gatherer{store: st, obs: obs}
}

// Of is one agent's situation, reading the project's pool for it alone. Roster is the form to reach
// for when more than one agent is judged.
func (g *Gatherer) Of(project, name string) (Situation, error) {
	pool, err := g.pool(project)
	if err != nil {
		return Situation{}, err
	}
	return g.in(project, name, pool)
}

// Roster is every agent in a project, sharing one read of the pool between them.
func (g *Gatherer) Roster(project string) ([]Situation, error) {
	rows, err := g.store.For(project).Roster()
	if err != nil {
		return nil, err
	}
	pool, err := g.pool(project)
	if err != nil {
		return nil, err
	}
	out := make([]Situation, 0, len(rows))
	for _, a := range rows {
		s, err := g.in(project, a.Name, pool)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// pool reads what a project could hand out, once for whoever asked.
func (g *Gatherer) pool(project string) (Pool, error) {
	ps := g.store.For(project)
	packages, err := ps.OpenContainers()
	if err != nil {
		return Pool{}, err
	}
	leaves, err := ps.OpenLeaves()
	if err != nil {
		return Pool{}, err
	}
	all, err := ps.AllTasks()
	if err != nil {
		return Pool{}, err
	}
	return Pool{Packages: packages, Leaves: leaves, All: all}, nil
}

func (g *Gatherer) in(project, name string, pool Pool) (Situation, error) {
	ps := g.store.For(project)
	a, ok, err := ps.GetAgent(name)
	if err != nil {
		return Situation{}, err
	}
	st, err := ps.GetState(name)
	if err != nil {
		return Situation{}, err
	}
	s := Situation{
		Project: project, Name: name, Reading: g.obs.Reading(project, name),
		Role: a.Role, Retired: a.Retired, ClearArmed: a.ClearArmed, Stopped: a.Stopped,
		Phase: st.Phase, Task: st.Task, Container: st.Container, Branch: st.Branch,
		Escalation: st.Escalation, LastNudge: st.LastNudge, Pool: pool,
	}
	if !ok {
		return s, nil // not on the roster: an identity answer, and every rule below reads as "nothing"
	}
	if s.AwaitingPR, s.AwaitingTask, err = ps.AwaitingPR(name); err != nil {
		return Situation{}, err
	}
	// The global store's ReviewingPR, not the project's: a pooled reviewer's held review is filed
	// under the PR's project, never its own, so a project-scoped read reports it free mid-review.
	held, reviewing, err := g.store.ReviewingPR(project, name)
	if err != nil {
		return Situation{}, err
	}
	s.ReviewingPR = reviewing
	if reviewing != "" {
		pr, found, perr := g.store.For(held).GetPR(reviewing)
		if perr != nil {
			return Situation{}, perr
		}
		s.ReviewOpen = found && pr.Status == "open"
	}
	if s.WaitingOnRun, err = ps.AgentWaitingOnRun(name); err != nil {
		return Situation{}, err
	}
	if st.Container != "" {
		t, found, terr := ps.GetTask(st.Container)
		if terr != nil {
			return Situation{}, terr
		}
		s.FeatureLanded = !found || featureLanded(ps, t)
	}
	return s, nil
}

// featureLanded reports a feature an agent should no longer hold: closed at its source, or carried in
// by a merged PR that is not an interim contribution's — that milestone is not the feature's own end,
// and counting it as one stranded a worker mid-feature the moment its own `contribute` merged.
func featureLanded(ps *store.ProjectStore, t store.Task) bool {
	if t.Status == "closed" || t.Status == "approved" || t.Status == "merged" {
		return true
	}
	prs, err := ps.PRs()
	if err != nil {
		return false
	}
	for _, p := range prs {
		if p.Task == t.ID && p.Status == "merged" && p.Kind != "interim" {
			return true
		}
	}
	return false
}
