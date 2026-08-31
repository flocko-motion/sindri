// package: arch / delivery
// type:    test (architecture invariant)
// job:     fail the build if any code puts text into an agent's session without declaring
// whether that message must be READ — every send states both properties
// (-> workflow.Delivery), and the few sites that inject directly are listed here
// with the reason each is push-only.
// limits:  the call sites only; which class a message is belongs to its sender, and the
// workflow's own guard (-> workflow/classification_test.go) covers that package
// in more detail.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// declaredInjectors are the files allowed to inject directly, each with WHY it is push-only. A send
// that is not on this list must go through the delivery primitive and state both properties.
//
// The list exists because "every sender declares both properties" cannot be enforced by the compiler
// outside the workflow package: the injection mechanism has to live somewhere, and the two modules
// that own a live session (chat, the agent lifecycle) hold their own handles to it. It was written
// after an audit that flagged one such sender and missed another — a list that fails the build is the
// difference between having weighed them all and remembering to.
var declaredInjectors = map[string]string{
	// The primitive itself: the one place a push is performed, for a sender that declared it.
	"internal/hub/deliver.go": "hub.Deliver — carries out a classified delivery",
	// The mechanism, plus Tell, which is push-only DELIBERATELY: it is synchronous and its caller is a
	// person, so a failure comes straight back to the terminal that typed it rather than being lost,
	// and conversational steering must not accumulate in a mailbox that is never pruned.
	"internal/hub/agent/inject.go": "the injection mechanism; Tell is push-only (see its doc)",
	// The /clear injection itself: the hub's own decision rather than a message from anyone, and
	// nothing worth keeping for an agent that was not there to receive it.
	"internal/hub/agent/clearcontext.go": "the /clear injection — push-only",
	// The /compact injection itself: the same class as clear's — nothing worth keeping for an agent
	// that was not there to receive it, and the hub's own decision rather than a message from anyone.
	"internal/hub/agent/compact.go": "the /compact injection — push-only, like clear's",
	// A model switch's /clear and /model: the same class again, and the instruction that follows the
	// switch is delivered by whoever asked for it rather than sent from here.
	"internal/hub/agent/model.go": "the model-switch sequence — push-only, like clear's and compact's",
	// A broadcast is push-only BY CONSTRUCTION: the chat module holds a Delivery port that can only
	// inject, so it cannot mail even by mistake. Correct for a stream a newcomer catches up on.
	"internal/hub/chat/service.go": "chat.deliver — push-only by construction (its port cannot mail)",
	// The seam that hands chat that port.
	"internal/hub/wiring.go": "the chatDelivery adapter",
}

// TestNothingInjectsWithoutDeclaringItsClass walks the module and finds every call that types into an
// agent's session. Each must sit in a file on the declared list; anything else is a sender that could
// be lost without a record, which is exactly what the mailbox exists to prevent.
func TestNothingInjectsWithoutDeclaringItsClass(t *testing.T) {
	root := moduleRoot(t)
	var undeclared []string
	seen := 0
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
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		f, perr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if perr != nil {
			return nil // not parseable is brokkr lint's business, not this guard's
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
			if sel.Sel.Name != "Inject" && sel.Sel.Name != "InjectWhenReady" {
				return true
			}
			seen++
			if _, declared := declaredInjectors[filepath.ToSlash(rel)]; !declared {
				undeclared = append(undeclared, rel+": "+sel.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	// Anti-vacuity: the injectors on the list are real call sites, so a scan finding none is a broken
	// scan rather than a clean tree — which is how a guard becomes decoration.
	if seen < 5 {
		t.Fatalf("only %d injection call sites found — the scan is not reading the module", seen)
	}
	for _, u := range undeclared {
		t.Errorf("%s injects into a session without declaring whether the message must be READ — send "+
			"it through the delivery primitive (workflow.Delivery), or add the file to "+
			"declaredInjectors with the reason it is push-only", u)
	}
}
