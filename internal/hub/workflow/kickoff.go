// package: hub/workflow / kickoff
// type:    logic (what a relaunched agent is told to resume with)
// job:     pendingKickoff — the claimed directive a model-switch's relaunch should wake into,
// not the generic "run sindri" nudge rehydrate falls back to otherwise.
// limits:  the message and its handoff; injecting it into the fresh session is the hub's own
// rehydrate (-> hub.rehydrate), which the workflow package never reaches into.
package workflow

import "sync"

// pendingKickoff holds, per agent, the directive its next relaunch should wake into instead of a
// bare "run sindri". In memory only: a hub restart mid-relaunch just costs one round trip.
type pendingKickoff struct {
	mu  sync.Mutex
	set map[string]string
}

func (p *pendingKickoff) arm(project, name, dir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.set == nil {
		p.set = map[string]string{}
	}
	p.set[project+"/"+name] = dir
}

// Take returns the armed directive for project/name and clears it, ok=false when none is pending
// — the caller's cue to fall back to the generic kickoff.
func (p *pendingKickoff) Take(project, name string) (dir string, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := project + "/" + name
	dir, ok = p.set[key]
	delete(p.set, key)
	return dir, ok
}

// TakePendingKickoff is pendingKickoff.Take for the hub's own rehydrate, the one caller outside
// this package (-> hub.rehydrate).
func (e *Engine) TakePendingKickoff(project, name string) (string, bool) {
	return e.kickoff.Take(project, name)
}
