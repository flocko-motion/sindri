// package: arch / topics
// type:    test (architecture invariant)
// job:     fail the build if a declared topic is published by nobody, or listened for by nothing.
// The topic vocabulary claims both — "for the check that each is both published and listened for"
// — and the check the comment named did not exist, which is how topic.MailArrived came to be
// listened for by two conditions and published by no one.
// limits:  the two halves being present. Whether the RIGHT writer publishes a topic, and whether a
// condition should watch it, is the flow's business; a topic may only shorten latency, so a wrong
// one costs a beat.
package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub/flow/agent/cond"
	"github.com/flo-at/sindri/internal/hub/flow/machine"
	prflow "github.com/flo-at/sindri/internal/hub/flow/pr"
	runflow "github.com/flo-at/sindri/internal/hub/flow/run"
	taskflow "github.com/flo-at/sindri/internal/hub/flow/task"
	"github.com/flo-at/sindri/internal/hub/flow/topic"
)

// TestEveryTopicIsPublishedAndListenedFor holds the vocabulary to what it says of itself. A topic
// nobody publishes is a condition waiting out its poll for an event that never arrives; one nothing
// listens for is a wake that reaches no state, paid for on every write that sends it.
func TestEveryTopicIsPublishedAndListenedFor(t *testing.T) {
	published := publishedTopics(t)
	listened := listenedTopics()
	for _, name := range topic.All {
		if !published[name] {
			t.Errorf("topic %q is declared and listened for, but nothing publishes it — the states watching for it wait out their poll", name)
		}
		if !listened[name] {
			t.Errorf("topic %q is published, but no condition watches it — every wake it costs reaches nothing", name)
		}
	}
}

// publishedTopics is every topic named outside the files that DECLARE the vocabulary and the maps
// that subscribe to it. Read off the source, since publishing is a call and nothing but the text
// says it happened — and by file rather than by call shape, because a publisher may reach its topic
// through a helper that maps a state to one (-> fleet.taskTopicFor).
func publishedTopics(t *testing.T) map[machine.Topic]bool {
	root := moduleRoot(t)
	out := map[machine.Topic]bool{}
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if vocabSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !strings.HasSuffix(path, ".go") || declaresTopics[filepath.ToSlash(rel)] {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		files++
		for _, name := range topic.All {
			if strings.Contains(string(data), "topic."+identFor(name)) {
				out[name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if files == 0 {
		t.Fatalf("walked %s and found no .go files — the guard is not looking at the repo", root)
	}
	return out
}

// declaresTopics is the vocabulary itself, the four maps that say what they listen for, and this
// test. Every other file naming a topic is naming it to publish it — these are the only places where
// a topic appears because a state SUBSCRIBES to it, and counting them would pass by construction.
var declaresTopics = map[string]bool{
	"internal/hub/flow/topic/topic.go":     true,
	"internal/hub/flow/agent/cond/cond.go": true,
	"internal/hub/flow/pr/pr.go":           true,
	"internal/hub/flow/task/task.go":       true,
	"internal/hub/flow/run/run.go":         true,
	"internal/arch/topics_test.go":         true,
}

// identFor is the Go identifier a topic is written under, derived from its value the way the
// declaration derives the value from the name: "review-filed" is ReviewFiled.
func identFor(name machine.Topic) string {
	var b strings.Builder
	for _, part := range strings.Split(string(name), "-") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	// "pr" is an initialism, so the vocabulary spells it PR where this rule would say Pr.
	return strings.NewReplacer("Pr", "PR").Replace(b.String())
}

// listenedTopics is every topic some condition declares a Wake for, across all four flows.
func listenedTopics() map[machine.Topic]bool {
	out := map[machine.Topic]bool{}
	for _, c := range cond.All {
		for _, w := range c.Wake {
			out[w] = true
		}
	}
	for _, s := range prflow.Flow {
		collectWakes(s.Events, out)
	}
	for _, s := range taskflow.Flow {
		collectWakes(s.Events, out)
	}
	for _, s := range runflow.Flow {
		collectWakes(s.Events, out)
	}
	return out
}

// collectWakes folds one state's condition subscriptions in. Generic over the world each flow is
// written against, since a subscription is the same fact whatever the subject.
func collectWakes[W any](events []machine.Transition[W], out map[machine.Topic]bool) {
	for _, e := range events {
		c, ok := e.On.(machine.Condition[W])
		if !ok {
			continue
		}
		for _, w := range c.Wake {
			out[w] = true
		}
	}
}
