package arch

import (
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEverySenderDeclaresBothProperties is the feature's own DONE WHEN, made structural rather than
// asserted: fleet.Deps has no bare injection to call any more, so a sender CANNOT send without
// stating both properties. This test guards the other half — that nobody reintroduces one by reaching
// past the port (a raw InjectWhenReady, a direct Inject), which is how push-only-by-omission would
// come back and take the guarantee with it.
func TestEverySenderDeclaresBothProperties(t *testing.T) {
	fset := token.NewFileSet()
	// The whole acting tree, not just this package: the senders moved out to the subjects that own
	// what they say, and a guard that kept looking here alone would pass over an empty room.
	var files []string
	for _, dir := range actingTrees(t) {
		found, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, found...)
	}
	if len(files) < 40 {
		t.Fatalf("only %d files found — the scan is not reading the acting tree", len(files))
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
		t.Errorf("only %d Say call sites found — the acting tree sends more than that", senders)
	}
}

// TestTheClassesAreDistinctAndNamed: the three combinations are the whole vocabulary, and a sender
// picking one is picking both answers at once. Neither-set is not a message (-> Sends).
func TestTheClassesAreDistinctAndNamed(t *testing.T) {
	for _, c := range []struct {
		name string
		d    mail.Delivery
		mail bool
		push bool
	}{
		{"mail.MailAndPush", mail.MailAndPush, true, true},
		{"mail.MailOnly", mail.MailOnly, true, false},
		{"mail.PushOnly", mail.PushOnly, false, true},
	} {
		if c.d.Mail != c.mail || c.d.Push != c.push {
			t.Errorf("%s = %+v, want mail=%v push=%v", c.name, c.d, c.mail, c.push)
		}
		if !c.d.Sends() {
			t.Errorf("%s must be a message", c.name)
		}
	}
	if (mail.Delivery{}).Sends() {
		t.Error("neither property set is not a message — a sender that forgot to classify")
	}
}

// actingTrees are the directories that may SEND — every acting half, plus the mailbox itself. Named
// here rather than globbed: a new subject that sends is a deliberate addition to this list, which is
// the same bargain every other guard in this package makes.
func actingTrees(t *testing.T) []string {
	root := moduleRoot(t)
	var out []string
	for _, p := range []string{
		"internal/hub/flow/fleet", "internal/hub/flow/pr", "internal/hub/flow/task",
		"internal/hub/flow/run", "internal/hub/flow/agent", "internal/hub/flow/agent/verbs",
		"internal/hub/messaging/mail",
	} {
		out = append(out, filepath.Join(root, p))
	}
	return out
}
