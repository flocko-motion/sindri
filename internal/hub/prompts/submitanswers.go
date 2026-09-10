// package: hub/prompts / submitanswers
// job:     render an author's submit answers as the comments a reviewer reads first.
// type:    rendering (agent-facing text)
// limits:  rendering. Which questions were asked is hub/flow/pr's.
package prompts

import (
	"fmt"
	"strings"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// Seq 0 holds the submit summary, which is the PR's own text already, so it is never a comment.

// SubmitAnswerComments renders one comment per exchange, each naming who spoke. A pair per comment
// rather than one block: the thread then reads as the conversation it was, and a reviewer can answer
// or quote a single exchange instead of the whole transcript.
func SubmitAnswerComments(answers []store.SubmitAnswer, agent string) []string {
	var out []string
	for _, a := range answers {
		if a.Seq == 0 {
			continue // the summary is the PR's own text already
		}
		out = append(out, fmt.Sprintf("[hub] %s\n\n[%s] %s", a.Question, agent, strings.TrimSpace(a.Answer)))
	}
	return out
}
