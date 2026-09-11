// package: hub/flow/pr / submitgate_act
// type:    logic (what an author must answer before a submit is taken)
// job:     the questions put to an author at submit, which two a given tree draws, and reading an
// interview's rows for the one question still standing — asked so the answering finds what a
// reviewer would otherwise find, at a fraction of the cost.
// limits:  the questions and the reading. Conducting the exchange belongs to the state the author
// sits in (-> worker/interviewing), and the rows are the store's.
package pr

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/world/store"
)

// EMPTY OF CONTENT, specific in shape. The hub never saw the diff, so "that pattern" means whatever
// the author decides — and deciding is the search this provokes. DO NOT make these smarter: naming
// files would supply the frame and get a confirmation back, and a checklist gets a checklist answer.
const (
	// QGeneralise is asked of every submit. Over half of this project's rejections are one failure —
	// a rule applied in the place that prompted it and nowhere else.
	QGeneralise = "Regarding your submit: tell me where else that pattern applies, and what you did about each place."

	// QEmpty replaces it when the branch carries no diff. Nothing was changed, so asking where else
	// it applies is meaningless; the open question is whether the work is genuinely done.
	QEmpty = "Regarding your submit: this branch changes nothing against its base. Make the case that the " +
		"task is finished anyway — what did the work, and why is there nothing left for this branch to carry?"
)

// rotatingQuestions is the second question, drawn by tree. Small on purpose: a padded pool teaches
// an author the exercise is arbitrary, and one that reads as arbitrary is answered as filler.
var rotatingQuestions = []string{
	"Regarding your submit: what does that do on empty, nil, or zero — and which test proves it?",
	"Regarding your submit: which existing comment, doc or spec does that make untrue?",
	"Regarding your submit: which test fails if you revert just that change?",
	"Regarding your submit: what did you decide against, and why?",
	"Regarding your submit: which part of that are you least sure about?",
}

// SubmitQuestions is what this tree must answer: the anchored one, then one drawn by the tree's own
// name. The same tree always draws the same angle, and a REWORK — a different tree — draws a fresh one.
func SubmitQuestions(tree string, emptyDiff bool) []string {
	first := QGeneralise
	if emptyDiff {
		first = QEmpty
	}
	return []string{first, rotatingQuestions[treeIndex(tree, len(rotatingQuestions))]}
}

// treeIndex maps a tree's name to a stable slot. A sum, not a hash: the name is already a digest,
// and what matters is that the same tree always draws the same question.
func treeIndex(tree string, n int) int {
	if n <= 0 {
		return 0
	}
	sum := 0
	for i := 0; i < len(tree); i++ {
		sum += int(tree[i])
	}
	return sum % n
}

// minAnswer is the shortest answer taken seriously. A floor rather than a judgement — the hub cannot
// tell a good answer from a bad one and must not try, but "yes" is not an answer to any of these.
const minAnswer = 40

// TooShort reports an answer below the floor. The question stays standing on one, so nothing is
// recorded and the author is asked again.
func TooShort(answer string) bool { return len([]rune(answer)) < minAnswer }

// Standing is the question this interview is waiting on: the first row with no answer, and how many
// there are in all. ok is false when every question has been answered, which is what ends it.
func Standing(rows []store.SubmitAnswer) (seq int, question string, of int, ok bool) {
	for _, r := range rows {
		if r.Seq > 0 {
			of++
		}
	}
	for _, r := range rows {
		if r.Seq > 0 && r.Answer == "" {
			return r.Seq, r.Question, of, true
		}
	}
	return 0, "", of, false
}

// InterviewOpen reports an author still owing an answer. The rows OUTLIVE the exchange — they are
// what the reviewer reads on the pull request — so the interview being over and the record being
// closed are two different moments, and this is the first of them.
func InterviewOpen(rows []store.SubmitAnswer) bool {
	_, _, _, ok := Standing(rows)
	return ok
}

// MsgSubmitQuestion puts one question and says how to answer it. Pushed into the session rather than
// returned from a command: a question put again because the last one went unanswered has no command
// to ride back on, and one delivery path for both is one fewer thing to disagree.
func MsgSubmitQuestion(n, of int, question string) string {
	return fmt.Sprintf("[hub] Before this submit is taken — question %d of %d.\n\n%s\n\n"+
		"Answer with `sindri submit \"<your answer>\"`. Your answer goes on the PR for the reviewer to "+
		"read. Answer from the code, not from memory: if you have to go and look, that is the point of "+
		"the question. Answer from the tree as it stands — EDITING IT ABANDONS THIS SUBMIT, because the "+
		"answers would then describe a tree that is gone, so fix what you find and submit again.", n, of, question)
}

// ReplyAnswerTooShort refuses a token answer. It states the floor rather than judging the content,
// which is all the hub can honestly do.
func ReplyAnswerTooShort(question string) string {
	return fmt.Sprintf("That is too short to be an answer, so the question stands:\n\n%s\n\n"+
		"Answer with `sindri submit \"<your answer>\"` — in prose, from what you find in the code.", question)
}

// ReplyAnswerRecorded acknowledges one answer. It names no next question: what follows is the
// interview's to put, and saying it here would be a second account of where the exchange has got to.
func ReplyAnswerRecorded(n, of int) string {
	if n >= of {
		return fmt.Sprintf("Answer %d of %d recorded. That was the last one — your submit is being taken now.", n, of)
	}
	return fmt.Sprintf("Answer %d of %d recorded. The next question is on its way.", n, of)
}

// ReplyInterviewOpened acknowledges the submit itself. The questions follow as their own messages,
// so this says what has happened and what to expect rather than carrying the first one.
const ReplyInterviewOpened = "Your submit is recorded. Before it is taken you have questions to " +
	"answer — they arrive in your session, one at a time, and each is answered with " +
	"`sindri submit \"<your answer>\"`. Leave the tree as it stands until the last one lands."
