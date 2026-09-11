// package: arch / onewriter
// type:    test (architecture invariant)
// job:     fail the build if more than one place in the product performs an act UPON an agent —
// moving it, handing it a review, closing its reviews, or claiming work for it. Each of these names
// the agent who will do a thing, which is the machine's decision and nobody else's.
// limits:  the count, over the product. `internal/hub/flowtest` is not the product: it stands in for
// the hub so a test can build a world, and it is the ONE door fixtures go through (-> flowtest.Place).
package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// actsUponAnAgent is every operation that names the agent who will do a thing. The list is closed
// and carries no exemptions: a second caller is not something to record and live with — it is the
// defect, because a second caller decides from a second copy of the rules, and two copies drift.
//
// Each is allowed exactly ONE caller outside the file that declares it. Whose caller that should be
// is in the words beside each entry, so a failure says what went wrong rather than only that it did.
var actsUponAnAgent = map[string]string{
	"SetPhase":       "where an agent stands is written once, by the machine that decided it (-> fleet.writeState)",
	"AssignReview":   "a review is handed over by the reviewer's own map (-> fleet.doTakeReview)",
	"CloseReviews":   "a pull request's reviews are closed where the request is (-> pr.ReleaseReviewers)",
	"ClaimLeaf":      "work is claimed by the worker's own map (-> fleet.doPickWork)",
	"ClaimContainer": "a feature is claimed by the worker's own map (-> fleet.doPickWork)",
}

// notTheProduct is code that exists to test the product rather than to be it. flowtest stands in for
// the hub, so a fixture reaching one of these through it is a test placing a subject, not a second
// decider — and it is one door rather than the hundred call sites it replaced.
var notTheProduct = map[string]bool{"flowtest": true}

// TestOneCallerForEveryActUponAnAgent counts, per operation, the files that call it. Counted by FILE
// and not by call, since a function may reach the same operation twice for one decision — the defect
// is a second PLACE deciding, never a second line.
func TestOneCallerForEveryActUponAnAgent(t *testing.T) {
	callers := map[string][]string{}
	declared := map[string]map[string]bool{}
	root := moduleRoot(t)
	seen := 0
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if vocabSkipDirs[d.Name()] || notTheProduct[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		seen++
		rel, _ := filepath.Rel(root, path)
		for name := range actsUponAnAgent {
			if declares(string(data), name) {
				if declared[name] == nil {
					declared[name] = map[string]bool{}
				}
				declared[name][rel] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if seen == 0 {
		t.Fatalf("walked %s and found no product files — the guard is not looking at the repo", root)
	}
	// A second pass, so a file that DECLARES an operation is never counted as calling it — the store
	// and the acting half both spell AssignReview, and each reaching the other is one decision.
	err = filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if vocabSkipDirs[d.Name()] || notTheProduct[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		for name := range actsUponAnAgent {
			if declared[name][rel] || !calls(string(data), name) {
				continue
			}
			callers[name] = append(callers[name], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	for name, why := range actsUponAnAgent {
		files := callers[name]
		sort.Strings(files)
		if len(files) != 1 {
			t.Errorf("%s has %d callers (%s) — it must have exactly one: %s",
				name, len(files), strings.Join(files, ", "), why)
		}
	}
}

// declares reports a file defining a function of this name, whatever its receiver.
func declares(src, name string) bool {
	return regexp.MustCompile(`func (\([^)]*\) )?` + regexp.QuoteMeta(name) + `\(`).MatchString(src)
}

// calls reports a file invoking this name as a method. Method calls only, since every one of these
// is reached through a receiver — the store's row writer or an acting half.
func calls(src, name string) bool {
	return regexp.MustCompile(`\.` + regexp.QuoteMeta(name) + `\(`).MatchString(src)
}
