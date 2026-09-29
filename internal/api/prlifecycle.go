// package: api / prlifecycle
// type:    logic (a rule over the exchange types)
// job:     which PR events tell the story a human reads at a glance, and that story as
// data — so both front-ends summarise a PR the same way.
// limits:  classification and a pure projection; how a line is drawn is each UI's.
package api

import (
	"sort"
	"strings"
)

// prEventMilestone classifies EVERY event type logged against a PR: true tells the story, false is
// diagnostic detail for the full log. Exhaustive on purpose — a missing type is a build failure
// rather than a silent default (hidden loses a state, shown clutters the summary).
var prEventMilestone = map[string]bool{
	"created":      true,
	"resubmitted":  true,
	"renewed":      true,
	"approved":     true,
	"endorsed":     true,
	"rejected":     true,
	"reopened":     true,
	"withdrawn":    true,
	"merged":       true,
	"milestone":    true, // it landed over work its task gained, so it was a milestone after all
	"merge-failed": true,
	"scrapped":     true,

	"precheck-pass":      false,
	"precheck-skipped":   false,
	"precheck-conflict":  false,
	"precheck-gate-fail": false,
	"checkout-failed":    false,
	// review-prepare-failed and reject-prepare-failed are gone with the preparation that produced
	// them: a hand-over hands over, and the session is the receiving agent's map to clear.
	"scrap-branch-failed": false,
	"review-amended":      false,
	"review-requested":    false,
	"conflict":            false,
	"warning":             false,
	// A merge asked for and a merge that could not run: both are steps toward the one milestone the
	// story wants, which is "merged" or "merge-failed". The refusal's words are what a human reads
	// to know what to fix, and they are in the full log where the rest of the diagnostics are.
	"merge-asked":   false,
	"merge-refused": false,

	// The flow machine's own account of this pull request (-> hub/flow/pr). Diagnostics, not
	// milestones: the states it moves through say the same things "merged" and "scrapped" already
	// say, and a lifecycle listing both would tell the story twice.
	"resumed": false, // the author was brought onto the base its own milestone moved
	"state":   false,
	"pass":    false,
}

// PREventKind reports whether a type belongs in the summary, and whether the vocabulary knows it.
// An unknown one is shown: a line too many gets noticed and classified, a missing state does not.
func PREventKind(typ string) (milestone, known bool) {
	m, ok := prEventMilestone[typ]
	return m || !ok, ok
}

// PREventTypes lists every classified event type, so a caller can check the vocabulary against what
// the code actually logs in both directions.
func PREventTypes() []string {
	out := make([]string, 0, len(prEventMilestone))
	for typ := range prEventMilestone {
		out = append(out, typ)
	}
	sort.Strings(out)
	return out
}

// PRMilestone is one step of a PR's life: what happened, who did it, when, and any detail the
// event type does not already say.
type PRMilestone struct {
	Event string `json:"event"`
	Who   string `json:"who,omitempty"`
	At    string `json:"at"`
	Note  string `json:"note,omitempty"`
}

// PRLifecycle is a PR's story in order, oldest first — created, verdicts, merged. A second view of
// the same data, never a replacement: the diagnostics it drops are what a failure needs. Verdicts
// take their author and time from the REVIEW records, which hold structurally what the payload says
// as prose, each event consuming the next matching review so two approvals read as two authors.
func PRLifecycle(history []Event, reviews []Review) []PRMilestone {
	verdicts := newVerdictQueue(reviews)
	var out []PRMilestone
	for _, e := range history {
		milestone, _ := PREventKind(e.Type)
		if !milestone {
			continue
		}
		m := PRMilestone{Event: e.Type, At: e.TS}
		m.Who, m.Note = splitActor(e.Payload)
		if want := verdictFor(e.Type); want != "" {
			if r, ok := verdicts.take(want); ok {
				m.Who, m.At = r.Author, r.VerdictAt
				if r.Advisory {
					m.Who += " (advisory)"
				}
			}
		}
		out = append(out, m)
	}
	return out
}

// verdictFor maps a milestone event to the verdict word a review record would carry for it, "" for
// the events that are not verdicts at all.
func verdictFor(typ string) string {
	switch typ {
	case "approved", "endorsed":
		return "pass"
	case "rejected":
		return "changes"
	}
	return ""
}

// verdictQueue holds the verdicted reviews in the order they were given, to be matched off against
// the verdict events one for one.
type verdictQueue struct{ left []Review }

func newVerdictQueue(reviews []Review) *verdictQueue {
	q := &verdictQueue{}
	for _, r := range reviews {
		if r.Verdict != "" {
			q.left = append(q.left, r)
		}
	}
	return q
}

// take removes and returns the first review carrying this verdict, ok=false when none is left — a
// verdict event with no record behind it keeps whatever its payload said.
func (q *verdictQueue) take(verdict string) (Review, bool) {
	for i, r := range q.left {
		if r.Verdict == verdict {
			q.left = append(q.left[:i], q.left[i+1:]...)
			return r, true
		}
	}
	return Review{}, false
}

// splitActor pulls the actor out of a payload written as "by <who>[: <note>]", the shape every
// authored PR event uses. Anything else is all note: guessing would put a wrong name on a real event.
func splitActor(payload string) (who, note string) {
	lead, rest, found := strings.Cut(payload, "by ")
	if !found {
		return "", payload
	}
	actor, tail, hasNote := strings.Cut(rest, ": ")
	// What stood before the "by" is part of the note, not of the name: "milestone by dvalin" and
	// "interim, by bombur: partial work" both name one actor and describe the work around it.
	note = strings.TrimSpace(lead)
	if hasNote {
		note = strings.TrimSpace(note + " " + tail)
	} else {
		note = strings.TrimRight(note, ",") // "interim," alone reads as a fragment
	}
	return actor, note
}
