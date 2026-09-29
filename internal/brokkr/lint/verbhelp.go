// package: lint / verbhelp
// type:    logic (a verb is named the same everywhere)
// job:     report a verb whose catalogue summary tells the agent to type a name other than its
// own, and a binding whose key no catalogue entry carries — the two ways a verb comes to
// be offered under one name and served under another.
// limits:  the names, read out of the source. Whether the help READS well is nobody's linter to
// judge, and what a verb does is the binding's own tests'.
package lint

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// catalogueFile and bindingFile are where the two lists live. Named rather than searched: there is
// exactly one of each by design, and a second copy appearing is the failure this exists to catch.
const (
	catalogueFile = "internal/hub/api/agents/verb/verb.go"
	bindingFile   = "internal/hub/commands.go"
)

// bindingKey matches one row of the binding table: a quoted verb name followed by its binding
// literal. Read from the source rather than by importing the hub, so the linter stays a tool over
// the tree instead of a consumer of it.
var bindingKey = regexp.MustCompile(`(?m)^\s*"([a-z][a-z-]*)":\s*\{`)

// VerbHelp reports every verb whose catalogue summary names a different verb, and every mismatch
// between the catalogue and the binding table. A summary reads "<what it does>: <how to type it>",
// so the word after the colon is what the agent will actually type — when that disagreed with the
// name the registry served, four role maps offered `chat` against a registered `meeting` and every
// directive listed a verb that answered "unknown".
func VerbHelp(roots []string, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
	root, err := moduleRootFrom(roots)
	if err != nil {
		return false, err
	}
	cat, err := catalogueNames(filepath.Join(root, catalogueFile))
	if errors.Is(err, fs.ErrNotExist) {
		// A repository holding no catalogue offers no verbs, so there is nothing here to be
		// named inconsistently — brokkr runs over other trees than this one.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if len(cat) == 0 {
		return false, fmt.Errorf("%s declares no verbs — the linter is not reading the catalogue", catalogueFile)
	}
	bound, err := bindingNames(filepath.Join(root, bindingFile))
	if err != nil {
		return false, err
	}

	var bad []string
	for _, v := range cat {
		if _, ok := bound[v.name]; !ok {
			bad = append(bad, fmt.Sprintf("%s: verb %q is in the catalogue with nothing bound to it — "+
				"an agent offered it would be told the verb is unknown", catalogueFile, v.name))
		}
		typed, ok := typedName(v.summary)
		if !ok {
			bad = append(bad, fmt.Sprintf("%s: verb %q has a summary with no \": <how to type it>\" tail (%q) — "+
				"the agent reads that tail as the command to run", catalogueFile, v.name, v.summary))
			continue
		}
		if typed != v.name {
			bad = append(bad, fmt.Sprintf("%s: verb %q tells the agent to type %q — a verb offered under one "+
				"name and served under another answers \"unknown\"", catalogueFile, v.name, typed))
		}
	}
	declared := map[string]bool{}
	for _, v := range cat {
		declared[v.name] = true
	}
	var orphans []string
	for name := range bound {
		if !declared[name] {
			orphans = append(orphans, name)
		}
	}
	sort.Strings(orphans)
	for _, name := range orphans {
		bad = append(bad, fmt.Sprintf("%s: %q is bound to an implementation but absent from the catalogue — "+
			"nothing offers it, so nothing can reach it", bindingFile, name))
	}

	for _, msg := range bad {
		if !cap.Allow() {
			continue
		}
		fmt.Fprintln(w, msg)
	}
	if len(bad) > 0 {
		cap.Note(w)
		if cap.Quiet() {
			return true, nil
		}
		fmt.Fprintf(w, "%d verb(s) are named inconsistently. The catalogue is the one list: every entry "+
			"needs a binding, every binding needs an entry, and a summary's tail must start with the "+
			"verb's own name.\n", len(bad))
	}
	return len(bad) > 0, nil
}

// catalogueEntry is one verb as the catalogue source declares it.
type catalogueEntry struct{ name, summary string }

// catalogueNames reads Name/Summary out of the catalogue's composite literals.
func catalogueNames(path string) ([]catalogueEntry, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	var out []catalogueEntry
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		var e catalogueEntry
		for _, el := range lit.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			val, ok := kv.Value.(*ast.BasicLit)
			if !ok || val.Kind != token.STRING {
				continue
			}
			s, uerr := strconv.Unquote(val.Value)
			if uerr != nil {
				continue
			}
			switch key.Name {
			case "Name":
				e.name = s
			case "Summary":
				e.summary = s
			}
		}
		if e.name != "" {
			out = append(out, e)
		}
		return true
	})
	return out, nil
}

// bindingNames reads the keys of the binding table.
func bindingNames(path string) (map[string]bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, m := range bindingKey.FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	return out, nil
}

// typedName is the first word after a summary's ": " — what the agent will type.
func typedName(summary string) (string, bool) {
	_, usage, found := strings.Cut(summary, ": ")
	if !found {
		return "", false
	}
	first, _, _ := strings.Cut(strings.TrimSpace(usage), " ")
	return first, first != ""
}

// moduleRootFrom finds the module root from the scanned paths, so the two files can be named
// relative to it whatever directory the linter was pointed at.
func moduleRootFrom(roots []string) (string, error) {
	start := "."
	if len(roots) > 0 {
		start = roots[0]
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", start)
		}
		dir = parent
	}
}
