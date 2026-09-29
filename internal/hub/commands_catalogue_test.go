package hub

import (
	"sort"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/api/agents/verb"
)

// TestEveryVerbIsBoundBothWays is what makes the catalogue the single list. A Def with nothing
// behind it is a verb an agent is offered and cannot run; a binding under a name the catalogue does
// not carry is an implementation nothing offers. Both had happened at once: the maps offered `chat`
// and `plan` while the registry served `meeting` and `openspec`, so every agent's directive listed
// two verbs that answered "unknown".
func TestEveryVerbIsBoundBothWays(t *testing.T) {
	h := newHub(t)
	bound := h.bindings()

	var unbound []string
	for _, d := range verb.Catalogue {
		if _, ok := bound[d.Name]; !ok {
			unbound = append(unbound, d.Name)
		}
	}
	for _, name := range unbound {
		t.Errorf("verb %q is in the catalogue with nothing bound to it — an agent would be offered a "+
			"verb that answers \"unknown\"", name)
	}

	var undeclared []string
	for name := range bound {
		if _, ok := verb.Lookup(name); !ok {
			undeclared = append(undeclared, name)
		}
	}
	sort.Strings(undeclared)
	for _, name := range undeclared {
		t.Errorf("%q is bound to an implementation but absent from the catalogue — nothing offers it, "+
			"so nothing can reach it", name)
	}

	if len(verb.Catalogue) < 30 {
		t.Fatalf("only %d verbs in the catalogue — it is not being read", len(verb.Catalogue))
	}
}

// TestEverySummaryNamesItsOwnVerb pins the half a set comparison cannot see: a Def whose summary
// tells the agent to type something else. The summaries are written as "<what it does>: <how to
// type it>", and the tail must start with the verb's own name — `plan`'s said "openspec submit"
// while its name was "plan", which is how the divergence hid in plain sight.
func TestEverySummaryNamesItsOwnVerb(t *testing.T) {
	for _, d := range verb.Catalogue {
		_, usage, found := strings.Cut(d.Summary, ": ")
		if !found {
			t.Errorf("verb %q: summary %q must end in \": <how to type it>\", so the agent reads the "+
				"name it types beside what the verb does", d.Name, d.Summary)
			continue
		}
		if first, _, _ := strings.Cut(strings.TrimSpace(usage), " "); first != d.Name {
			t.Errorf("verb %q: its summary tells the agent to type %q — a verb offered under one name "+
				"and served under another answers \"unknown\"", d.Name, first)
		}
	}
}
