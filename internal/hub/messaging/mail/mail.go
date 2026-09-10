// package: hub/messaging/mail / mail
// type:    assembly (the mailbox, and the one door into it)
// job:     hold what a message to an agent needs — the store it lives in, the session it may be
// pushed into, the board that counts it — so every send, read and reply is one package rather
// than four files beside the composition root.
// limits:  the mailbox. WHICH class a message is belongs to its sender (-> Delivery), the
// rows are the store's, and the words an agent reads are hub/prompts'.
package mail

import (
	"strings"

	"github.com/flo-at/sindri/internal/hub/store"
)

// Deps is the mailbox's seam back to the hub: the session a push is typed into, the board that
// carries the unread count, and what a repo is called when a sender has to be qualified.
type Deps interface {
	// Push types text into a live session, waiting for the prompt. Under the HUB's lifetime: a push
	// must land whether or not whoever triggered it is still there.
	Push(project, name, text string) error
	// Notify tells the board the unread count moved.
	Notify()
	// RepoName is a project's display name, for a sender addressed from another repo.
	RepoName(project string) string
	// Reachable is whether the agent is THERE, and nothing else — whether a push has anywhere to
	// land. Claude Code QUEUES what is typed mid-turn, so a notice sent to a busy agent arrives as
	// that turn ends, the moment it can be acted on; waiting for an idle prompt made the news stale.
	Reachable(project, name string) bool
	// MayWake is whether an unprompted wake is welcome — false for an agent the HUB itself told to
	// wait, since prodding one complains about a state the hub chose. A policy question, so it is
	// asked rather than re-derived here (-> situation.Situation.ParkedByTheHub).
	MayWake(project, name string) bool
}

// Box is the fleet's mailbox.
type Box struct {
	store *store.Store
	deps  Deps
}

// New builds the mailbox over the store it lives in.
func New(st *store.Store, d Deps) *Box { return &Box{store: st, deps: d} }

// qualify writes a sender as "repo/agent", which is what a message crossing repos needs to be
// answerable — the reply path resolves the bare name again.
func (b *Box) qualify(project, agent string) string {
	return b.deps.RepoName(project) + "/" + agent
}

// Recipient resolves a bare name fleet-wide, or "repo/agent" directly; ambiguity is refused.
func (b *Box) Recipient(to string) (project, name string, err error) {
	if repo, agent, ok := strings.Cut(to, "/"); ok {
		matches, merr := b.store.AgentsNamed(agent)
		if merr != nil {
			return "", "", merr
		}
		for _, a := range matches {
			if b.deps.RepoName(a.Project) == repo || a.Project == repo {
				return a.Project, a.Name, nil
			}
		}
		return "", "", errNoSuchIn(agent, repo)
	}
	matches, err := b.store.AgentsNamed(to)
	if err != nil {
		return "", "", err
	}
	switch len(matches) {
	case 0:
		return "", "", errNoSuchAnywhere(to)
	case 1:
		return matches[0].Project, matches[0].Name, nil
	}
	qualified := make([]string, len(matches))
	for i, a := range matches {
		qualified[i] = b.qualify(a.Project, a.Name)
	}
	return "", "", errAmbiguous(to, qualified)
}
