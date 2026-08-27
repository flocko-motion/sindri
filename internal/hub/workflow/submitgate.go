// package: hub/workflow / submit questions
// type:    logic (what an author must answer before a submit is taken)
// job:     the questions put to an author at submit, and which two a given commit draws — asked so
// the answering finds what a reviewer would otherwise find, at a fraction of the cost.
// limits:  the questions and the choosing; the flow that asks them is CmdSubmit's, and the answers
// are the store's.
package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/hub/store"
)

// EMPTY OF CONTENT, specific in shape. The hub never saw the diff, so "that pattern" means whatever
// the author decides — and deciding is the search this provokes. DO NOT make these smarter: naming
// files would supply the frame and get a confirmation back, and a checklist gets a checklist answer.
const (
	// qGeneralise is asked of every submit. Over half of this project's rejections are one failure —
	// a rule applied in the place that prompted it and nowhere else.
	qGeneralise = "Regarding your submit: tell me where else that pattern applies, and what you did about each place."

	// qEmpty replaces it when the branch carries no diff. Nothing was changed, so asking where else
	// it applies is meaningless; the open question is whether the work is genuinely done.
	qEmpty = "Regarding your submit: this branch changes nothing against its base. Make the case that the " +
		"task is finished anyway — what did the work, and why is there nothing left for this branch to carry?"
)

// rotatingQuestions is the second question, drawn by commit. Small on purpose: a padded pool teaches
// an author the exercise is arbitrary, and one that reads as arbitrary is answered as filler.
var rotatingQuestions = []string{
	"Regarding your submit: what does that do on empty, nil, or zero — and which test proves it?",
	"Regarding your submit: which existing comment, doc or spec does that make untrue?",
	"Regarding your submit: which test fails if you revert just that change?",
	"Regarding your submit: what did you decide against, and why?",
	"Regarding your submit: which part of that are you least sure about?",
}

// submitQuestions is what this commit must answer: the anchored one, then one drawn by sha. A retry
// on the same tree asks the same thing; a REWORK is a new sha, so it draws a fresh angle.
func submitQuestions(sha string, emptyDiff bool) []string {
	first := qGeneralise
	if emptyDiff {
		first = qEmpty
	}
	return []string{first, rotatingQuestions[shaIndex(sha, len(rotatingQuestions))]}
}

// shaIndex maps a commit to a stable slot. A sum, not a hash: the sha is already random enough, and
// what matters is that the same tree always draws the same question.
func shaIndex(sha string, n int) int {
	if n <= 0 {
		return 0
	}
	sum := 0
	for i := 0; i < len(sha); i++ {
		sum += int(sha[i])
	}
	return sum % n
}

// minAnswer is the shortest answer taken seriously. A floor rather than a judgement — the hub cannot
// tell a good answer from a bad one and must not try, but "yes" is not an answer to any of these.
const minAnswer = 40

// ReplySubmitQuestion puts one question and says how to answer it. The verb is `submit` because an
// agent has no dialogue channel — each call is its own process, and the hub knows where it is.
func ReplySubmitQuestion(n, of int, question string) string {
	return fmt.Sprintf("Before this submit is taken — question %d of %d.\n\n%s\n\n"+
		"Answer with `sindri submit \"<your answer>\"`. Your answer goes on the PR for the reviewer to "+
		"read. Answer from the code, not from memory: if you have to go and look, that is the point of "+
		"the question — and if answering means writing a test or fixing what you find, do that first. "+
		"Your answers keep; what you submit is the tree as it stands when the last one lands.", n, of, question)
}

// ReplyAnswerTooShort refuses a token answer. It states the floor rather than judging the content,
// which is all the hub can honestly do.
func ReplyAnswerTooShort(question string) string {
	return fmt.Sprintf("That is too short to be an answer, so the question stands:\n\n%s\n\n"+
		"Answer with `sindri submit \"<your answer>\"` — in prose, from what you find in the code.", question)
}

// submitAnswerComments renders one comment per exchange, each naming who spoke. A pair per comment
// rather than one block: the thread then reads as the conversation it was, and a reviewer can answer
// or quote a single exchange instead of the whole transcript.
func submitAnswerComments(answers []store.SubmitAnswer, agent string) []string {
	var out []string
	for _, a := range answers {
		if a.Question == summaryRow {
			continue // the summary is the PR's own text already
		}
		out = append(out, fmt.Sprintf("[hub] %s\n\n[%s] %s", a.Question, agent, strings.TrimSpace(a.Answer)))
	}
	return out
}

// askSubmitQuestions runs the questionnaire for one submit ATTEMPT, reporting whether it asked
// something — then the submit has not happened and the caller stops. The FIRST call carries the
// summary and later ones answers, told apart by how many rows exist.
//
// It stays on the sha the attempt opened at. Keyed on the CURRENT tree it reset itself: answering
// these sends an author into the code, so doing it honestly asked everything again while answering
// from memory sailed through — the gate rewarded the shallower answer.
func (e *Engine) askSubmitQuestions(ps *store.ProjectStore, agent, sha, text string, emptyDiff bool, out io.Writer) (asked bool, err error) {
	open, rows, err := ps.OpenSubmitAnswers(agent)
	if err != nil {
		return false, err
	}
	if open != "" {
		sha = open
	}
	qs := submitQuestions(sha, emptyDiff)
	// Row 0 is the SUMMARY, which is also what says the questionnaire has begun — without it a second
	// call is indistinguishable from the first, and the same question comes round for ever.
	if len(rows) == 0 {
		if err := ps.AddSubmitAnswer(agent, sha, 0, summaryRow, text); err != nil {
			return false, err
		}
		fmt.Fprintln(out, ReplySubmitQuestion(1, len(qs), qs[0]))
		return true, nil
	}
	if given := len(rows) - 1; given >= len(qs) {
		return false, nil // every question answered for this tree: the submit proceeds
	}
	q := qs[len(rows)-1]
	if len([]rune(text)) < minAnswer {
		fmt.Fprintln(out, ReplyAnswerTooShort(q))
		return true, nil
	}
	if err := ps.AddSubmitAnswer(agent, sha, len(rows), q, text); err != nil {
		return false, err
	}
	if len(rows) >= len(qs) { // that was the last one
		_ = ps.Log(agent, "submit-questions", fmt.Sprintf("%d answered for %s", len(qs), shortSHA(sha)))
		return false, nil
	}
	fmt.Fprintln(out, ReplySubmitQuestion(len(rows)+1, len(qs), qs[len(rows)]))
	return true, nil
}

// summaryRow marks row 0, which holds the submit summary rather than an answer.
const summaryRow = "(the submit summary)"
