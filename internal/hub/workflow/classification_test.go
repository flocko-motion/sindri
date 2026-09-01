package workflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flo-at/sindri/internal/hub/store"
)

// TestEverySenderDeclaresBothProperties is the feature's own DONE WHEN, made structural rather than
// asserted: workflow.Deps has no bare injection to call any more, so a sender CANNOT send without
// stating both properties. This test guards the other half — that nobody reintroduces one by reaching
// past the port (a raw InjectWhenReady, a direct Inject), which is how push-only-by-omission would
// come back and take the guarantee with it.
func TestEverySenderDeclaresBothProperties(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 20 {
		t.Fatalf("only %d files found — the scan is not reading the workflow package", len(files))
	}
	var senders int
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "InjectWhenReady", "Inject":
				t.Errorf("%s:%d: %s reaches past the delivery port — every message states whether it "+
					"must be READ and whether it should WAKE (-> Harness.Say, delivery.go)",
					path, fset.Position(call.Pos()).Line, sel.Sel.Name)
			case "Say":
				senders++
			}
			return true
		})
	}
	// And prove the scan sees the senders it is meant to be checking: an empty result would pass the
	// loop above vacuously, which is exactly how this guard could become decoration.
	if senders < 15 {
		t.Errorf("only %d Say call sites found — the workflow sends more than that", senders)
	}
}

// TestTheClassesAreDistinctAndNamed: the three combinations are the whole vocabulary, and a sender
// picking one is picking both answers at once. Neither-set is not a message (-> Sends).
func TestTheClassesAreDistinctAndNamed(t *testing.T) {
	for _, c := range []struct {
		name string
		d    Delivery
		mail bool
		push bool
	}{
		{"MailAndPush", MailAndPush, true, true},
		{"MailOnly", MailOnly, true, false},
		{"PushOnly", PushOnly, false, true},
	} {
		if c.d.Mail != c.mail || c.d.Push != c.push {
			t.Errorf("%s = %+v, want mail=%v push=%v", c.name, c.d, c.mail, c.push)
		}
		if !c.d.Sends() {
			t.Errorf("%s must be a message", c.name)
		}
	}
	if (Delivery{}).Sends() {
		t.Error("neither property set is not a message — a sender that forgot to classify")
	}
}

// TestARejectionIsMailedAndTheNudgeIsNot walks two real senders end to end, which is what the rule
// actually claims. A rejection with its feedback must not be lost to an agent that was away, so it is
// mailed AND pushed. A stall nudge is the opposite case: waking is its entire purpose, it fires again
// next tick, and mailing it would keep chatter for ever — which is the argument that makes unbounded
// retention affordable in the first place.
func TestARejectionIsMailedAndTheNudgeIsNot(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ps := st.For("proj")
	if err := ps.PutAgent(store.Agent{Name: "bombur", Role: "worker", Workspace: ".worktrees/bombur"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.PutPR(store.PR{ID: "pr-1", Task: "td-1", Agent: "bombur", Branch: "td-1", Base: "main", Status: "open"}); err != nil {
		t.Fatal(err)
	}
	if err := ps.SetState(store.AgentState{Agent: "bombur", Task: "td-1", Branch: "td-1", Phase: "submitted"}, store.ReasonClaimed, "test setup"); err != nil {
		t.Fatal(err)
	}
	deps := &stubDeps{root: t.TempDir(), alive: true}
	e := newEngine(st, deps)

	if err := e.RejectPR("proj", "pr-1", "needs another pass"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if len(deps.delivered) != 1 || !deps.delivered[0].Mail || !deps.delivered[0].Push {
		t.Fatalf("a rejection must be mailed and pushed, got %+v", deps.delivered)
	}
	// And it says who rejected it, which is the half sd-bc3a1f added: an agent weights a message by
	// its sender, and "the hub" would be a worse answer than the truth.
	if deps.delivered[0].Sender == "" {
		t.Error("a rejection should name its author as the sender")
	}

	if !e.NudgeStalled("proj", "bombur", saying("idle"), StallDwell+time.Minute) {
		t.Fatal("a rejected worker gone quiet past the dwell should be nudged")
	}
	if len(deps.delivered) != 2 || deps.delivered[1].Mail || !deps.delivered[1].Push {
		t.Fatalf("a stall nudge must be push-only, got %+v", deps.delivered)
	}
}
