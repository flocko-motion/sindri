package workflow

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestHubMailSaysItCannotBeAnswered: an agent learned that only by drafting a reply and having it
// refused — two turns, the first spent cutting its answer to a length limit for a recipient that
// does not exist. The fact belongs where the message is READ, before any drafting.
func TestHubMailSaysItCannotBeAnswered(t *testing.T) {
	got := DirMail([]store.Mail{{ID: 1087, Sender: "hub", Body: "the quality gate failed"}})
	if !strings.Contains(got, "nobody behind it") {
		t.Errorf("hub mail should say there is no correspondent:\n%s", got)
	}
	for _, want := range []string{"sindri escalate", "sindri comment"} {
		if !strings.Contains(got, want) {
			t.Errorf("it should name what to do instead (%s):\n%s", want, got)
		}
	}
	// An agent's own mail is untouched: it HAS a correspondent, and the note would be a lie.
	if peer := DirMail([]store.Mail{{ID: 9, Sender: "dvalin", Body: "have a look"}}); strings.Contains(peer, "nobody behind it") {
		t.Errorf("a peer's message can be answered:\n%s", peer)
	}
}
