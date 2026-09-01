package workflow

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestARejectionIsAboutTheWorkInHand is hepti's misrouting: told "your PR for sd-57e895 was REJECTED"
// over feedback belonging to sd-92c80a, a PR from earlier work. prRejected matched ANY rejected PR by
// that author, so a stale one was served for ever, paired with whatever task was held at the time.
func TestARejectionIsAboutTheWorkInHand(t *testing.T) {
	st, ps := poolFixture(t)
	e := newEngine(st, &stubDeps{root: t.TempDir(), alive: true})
	old := store.PR{ID: "pr-old", Task: "sd-old", Agent: "hepti", Branch: "b", Base: "main",
		Status: "rejected", Feedback: "about the OLD task"}
	if err := ps.PutPR(old); err != nil {
		t.Fatal(err)
	}

	// Holding a different task: the stale rejection must not be reported as this task's.
	if _, rejected, err := e.prRejected("repo", "hepti", "sd-new"); err != nil || rejected {
		t.Errorf("a rejection of other work was served for the task in hand (rejected=%v, err=%v)", rejected, err)
	}
	// The task it really belongs to still gets it.
	fb, rejected, err := e.prRejected("repo", "hepti", "sd-old")
	if err != nil || !rejected {
		t.Fatalf("the rejection of sd-old went missing: rejected=%v err=%v", rejected, err)
	}
	if !strings.Contains(fb, "OLD task") {
		t.Errorf("wrong feedback returned: %q", fb)
	}
	// Holding nothing: there is no rejection of nothing.
	if _, rejected, _ := e.prRejected("repo", "hepti", ""); rejected {
		t.Error("an agent holding nothing was told something of its was rejected")
	}
}
