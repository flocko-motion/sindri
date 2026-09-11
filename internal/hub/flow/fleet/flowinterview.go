// package: hub/flow/fleet / flowinterview
// type:    logic (conducting the submit interview)
// job:     put the submit questions to an author one at a time and wait for each answer, re-posing
// one the author has plainly not seen, and abandoning the whole exchange the moment the tree it is
// about stops being the tree in front of it.
// limits:  the exchange. Which questions there are is hub/flow/pr's, and where its outcomes lead is
// the worker's map (-> roles/worker's interviewing).
package fleet

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/flo-at/sindri/internal/adapter/git"
	"github.com/flo-at/sindri/internal/hub/flow"
	"github.com/flo-at/sindri/internal/hub/flow/agent/act"
	flowpr "github.com/flo-at/sindri/internal/hub/flow/pr"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"github.com/flo-at/sindri/internal/hub/world/store"
)

// interviewBeat is how often the exchange looks for an answer — two local reads, so it can be brisk.
const interviewBeat = time.Second

// interviewGrace is the lag between typing into a session and that session reporting a turn. NOT a
// judgement about how long an answer should take: an author reading code reports working, not a prompt.
const interviewGrace = 30 * time.Second

// doInterview conducts the whole exchange as one process: put the question, wait, take the answer,
// put the next. A cancel on the way out needs no unwinding — every step of it is a row.
func (e *Engine) doInterview(ctx context.Context, w flow.World) (flow.Outcome, error) {
	ps := e.Store.For(w.Project)
	tree, rows, err := ps.OpenSubmitAnswers(w.Name)
	if err != nil {
		return act.Failed, err
	}
	if tree == "" {
		// Nothing written down to conduct. The request goes back, or this repeats for ever.
		e.abandonSubmit(ps, w.Name, "no interview was recorded for it")
		return act.Failed, nil
	}
	for {
		seq, question, of, standing := flowpr.Standing(rows)
		if !standing {
			return act.Done, nil // every question answered; what it holds can go up
		}
		asked := time.Now()
		e.putQuestion(w, seq, of, question)
		answered, err := e.awaitAnswer(ctx, w, tree, seq, of, question, asked)
		if err != nil {
			return act.Failed, err
		}
		if !answered {
			e.abandonSubmit(ps, w.Name, "its tree moved while the questions stood")
			return act.Stale, nil
		}
		if tree, rows, err = ps.OpenSubmitAnswers(w.Name); err != nil {
			return act.Failed, err
		}
	}
}

// awaitAnswer waits out one question, reporting false on a tree that moved. The tree is examined
// when an answer arrives and before a question is put again, never on the beat: a git invocation per
// author per pass is the cost the standing observer exists to avoid.
func (e *Engine) awaitAnswer(ctx context.Context, w flow.World, tree string, seq, of int, question string, asked time.Time) (bool, error) {
	ps := e.Store.For(w.Project)
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(interviewBeat):
		}
		_, rows, err := ps.OpenSubmitAnswers(w.Name)
		if err != nil {
			return false, err
		}
		if got, _, _, standing := flowpr.Standing(rows); !standing || got != seq {
			return e.treeStands(w, tree) // the answer landed; it describes this tree or none
		}
		// An author at a prompt has ended its turn, so it is not still answering. The reading must be
		// NEWER than the question, or a stale one re-poses instantly and for ever.
		if obs := e.Harness.Observe(w.Project, w.Name); obs.AtPrompt() && obs.TakenAt.After(asked.Add(interviewGrace)) {
			stands, terr := e.treeStands(w, tree)
			if terr != nil || !stands {
				return false, terr
			}
			e.putQuestion(w, seq, of, question)
			asked = time.Now()
		}
	}
}

// putQuestion pushes one question into the author's session. Pushed rather than mailed: unread mail
// is a state of its own, which would pull the author out of the interview it is standing in.
func (e *Engine) putQuestion(w flow.World, seq, of int, question string) {
	if err := e.Harness.Say(w.Project, w.Name, flowpr.MsgSubmitQuestion(seq, of, question), mail.PushOnly); err != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "submit-question", fmt.Sprintf("%d of %d did not land: %v", seq, of, err))
	}
}

// treeStands reports the author's worktree still being the one the interview opened on. A failure to
// LOOK is not a moved tree: it would throw away answers given in good faith.
func (e *Engine) treeStands(w flow.World, tree string) (bool, error) {
	ag, ok, err := e.Store.For(w.Project).GetAgent(w.Name)
	if err != nil || !ok {
		return true, err
	}
	now, ferr := git.TreeFingerprint(filepath.Join(e.Deps.ProjectRoot(w.Project), ag.Workspace))
	if ferr != nil {
		_ = e.Store.For(w.Project).Log(w.Name, "submit-tree", "could not be read: "+ferr.Error())
		return true, nil
	}
	return now == tree, nil
}

// abandonSubmit takes the request back and closes the interview. Marked finished rather than
// deleted: the rows are the record of what was asked, and the next attempt draws its own.
//
// It answers for its own failures rather than returning them: an action that returns an error moves
// nobody, which would leave the author standing in an interview that has already been called off.
func (e *Engine) abandonSubmit(ps *store.ProjectStore, agent, why string) {
	if tree, _, err := ps.OpenSubmitAnswers(agent); err == nil && tree != "" {
		_ = ps.FinishSubmitAnswers(agent, tree)
	}
	_ = ps.Log(agent, "submit-abandoned", why)
	if err := ps.AnswerSubmitRequest(agent); err != nil {
		log.Printf("hub: taking back %s's submit request: %v", agent, err)
	}
}
