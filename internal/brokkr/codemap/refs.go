// package: codemap / refs
// type:    logic (symbol references)
// job:     answer "where is this used" — every reference to an exact identifier across a tree,
// classified (definition, call, plain reference) and ranked so the answer leads with what you
// asked for, each tagged with the declaration and arch header it sits in.
// limits:  syntactic (go/ast, no type check), so it matches a NAME, not a resolved object — two
// packages with the same identifier both answer. CLI wiring is cmd/brokkr/refs.go's.
package codemap

import (
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
	"strings"
)

// DefaultRefLimit bounds a reference report. Ranked output makes a limit safe: what falls off the
// end is the least relevant, which is the opposite of how truncating a file-ordered grep behaves.
const DefaultRefLimit = 200

// RefKind is what the symbol is DOING at one occurrence, which is what makes the answer
// rankable: a definition, a call, a mention by value, or prose.
type RefKind int

// The kinds, in the order a reader wants them.
const (
	RefDef     RefKind = iota // the declaration itself
	RefCall                   // called here: Foo(…) or x.Foo(…)
	RefOther                  // any other code reference: a value, a type, an assignment target
	RefComment                // named in a comment — prose, not a reference
)

// String names a kind for the report.
func (k RefKind) String() string {
	switch k {
	case RefDef:
		return "definition"
	case RefCall:
		return "call"
	case RefComment:
		return "comment"
	default:
		return "reference"
	}
}

// Ref is one occurrence of the symbol: where it is, what it is doing, and the context that says
// whose code it happened in.
type Ref struct {
	Path      string // display path, as the map labels files
	Line, Col int
	Kind      RefKind
	Text      string // the source line, trimmed
	Encl      string // the declaration it sits in ("func X"), "" outside every declaration
	Pkg       string // the arch header's `package:` field, "" when the file has no header
	Test      bool   // in a _test.go file: real code, but not what you asked about first
}

// RefQuery narrows a reference search. Symbol is an exact, case-sensitive Go identifier — NOT a
// regexp, unlike Query's Find/Grep — so Foo never answers for FooBar. File is a path substring.
type RefQuery struct {
	Symbol   string
	File     string
	Comments bool // include prose mentions, which are ranked last and off by default
}

// validIdent guards the promise that Symbol is an identifier: a pattern that could never be one
// would silently return nothing, which reads like "unused" instead of "wrong question".
var validIdent = regexp.MustCompile(`^[\p{L}_][\p{L}\p{Nd}_]*$`)

// Refs finds every reference to q.Symbol under roots, ranked. maxDepth bounds descent below each
// root (0 = root only, negative = unlimited). A root that cannot be walked is a loud error.
func Refs(roots []string, maxDepth int, q RefQuery) ([]Ref, error) {
	if !validIdent.MatchString(q.Symbol) {
		return nil, fmt.Errorf("brokkr refs %q: not an identifier — refs takes an exact symbol name, "+
			"not a pattern (for a regexp search use `brokkr map --grep`)", q.Symbol)
	}
	var out []Ref
	if err := walkGo(roots, maxDepth, strings.ToLower(q.File), func(disp, path string) {
		out = append(out, fileRefs(disp, path, q)...)
	}); err != nil {
		return nil, err
	}
	rank(out)
	return out, nil
}

// fileRefs collects one file's references. A file that will not parse yields none: a search
// reports matches, not the state of files it cannot read (the map already says that).
func fileRefs(disp, path string, q RefQuery) []Ref {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(src), "\n")
	ctx := refContext{
		disp:  disp,
		lines: lines,
		units: collectUnits(fset, f),
		pkg:   headerPackage(f),
		test:  strings.HasSuffix(path, "_test.go"),
		fset:  fset,
	}
	refs := ctx.code(f, q.Symbol)
	if q.Comments {
		refs = append(refs, ctx.prose(f, q.Symbol)...)
	}
	return refs
}

// refContext is what every hit in one file needs: how to label it, and where it sits.
type refContext struct {
	disp  string
	lines []string
	units []unit
	pkg   string
	test  bool
	fset  *token.FileSet
}

// at builds a Ref for a position, attaching the enclosing declaration and the file's arch header.
func (c refContext) at(pos token.Position, kind RefKind) Ref {
	text := ""
	if pos.Line-1 >= 0 && pos.Line-1 < len(c.lines) {
		text = strings.TrimSpace(c.lines[pos.Line-1])
	}
	return Ref{
		Path: c.disp, Line: pos.Line, Col: pos.Column, Kind: kind,
		Text: text, Encl: enclosing(c.units, pos.Line), Pkg: c.pkg, Test: c.test,
	}
}

// code walks the syntax tree for identifiers spelled exactly sym, classifying each by what
// encloses it. The parent stack is kept by hand because that is the only way to tell a call from
// a value: the identifier itself looks identical either way.
func (c refContext) code(f *ast.File, sym string) []Ref {
	var (
		out   []Ref
		stack []ast.Node
	)
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1] // pop: Inspect signals the end of a node with nil
			return false
		}
		stack = append(stack, n)
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != sym {
			return true
		}
		out = append(out, c.at(c.fset.Position(id.Pos()), classify(stack)))
		return true
	})
	return out
}

// classify decides what the identifier on top of stack is doing. Only top-level declarations
// count as definitions: a local `foo := …` is a use of the name, not the thing being searched
// for, and calling it a definition would put it above the call sites you asked for.
func classify(stack []ast.Node) RefKind {
	id, _ := stack[len(stack)-1].(*ast.Ident)
	parent := ancestor(stack, 2)
	switch p := parent.(type) {
	case *ast.FuncDecl:
		if p.Name == id {
			return RefDef
		}
	case *ast.TypeSpec:
		if p.Name == id {
			return RefDef
		}
	case *ast.ValueSpec:
		// A var/const at file scope; inside a function the same node is a local, so the
		// grandparent's kind is what separates the two.
		if isTopLevel(stack) && containsIdent(p.Names, id) {
			return RefDef
		}
	case *ast.Field:
		if containsIdent(p.Names, id) {
			return RefDef // a struct field or an interface method: the name is declared here
		}
	case *ast.CallExpr:
		if p.Fun == id {
			return RefCall
		}
	case *ast.SelectorExpr:
		// x.Foo — a call only when the selector as a whole is what is being called.
		if p.Sel == id {
			if call, ok := ancestor(stack, 3).(*ast.CallExpr); ok && call.Fun == p {
				return RefCall
			}
		}
	}
	return RefOther
}

// ancestor is stack[len-n], or nil when the stack is shorter — the walk starts at the file, so a
// hit near the top has no grandparent.
func ancestor(stack []ast.Node, n int) ast.Node {
	if len(stack) < n {
		return nil
	}
	return stack[len(stack)-n]
}

// isTopLevel reports whether the declaration under the cursor is the file's own, not one nested
// inside a function body. The stack is file → GenDecl → ValueSpec → Ident when it is.
func isTopLevel(stack []ast.Node) bool {
	_, ok := ancestor(stack, 4).(*ast.File)
	return ok
}

// containsIdent reports whether ids holds exactly this identifier (by pointer: two names can be
// spelled the same in one spec, and only the one we walked to is the hit).
func containsIdent(ids []*ast.Ident, id *ast.Ident) bool {
	for _, n := range ids {
		if n == id {
			return true
		}
	}
	return false
}

// prose finds the symbol named in comments, as a whole word — the doc that explains it, or a
// note that survived the code it described. Ranked last: prose is not a reference.
func (c refContext) prose(f *ast.File, sym string) []Ref {
	word := regexp.MustCompile(`\b` + regexp.QuoteMeta(sym) + `\b`)
	var out []Ref
	for _, group := range f.Comments {
		for _, cm := range group.List {
			// One Ref per comment LINE, not per match, so a line naming it twice lands once.
			for i, line := range strings.Split(cm.Text, "\n") {
				if word.MatchString(line) {
					pos := c.fset.Position(cm.Pos())
					pos.Line += i
					out = append(out, c.at(pos, RefComment))
				}
			}
		}
	}
	return out
}

// headerPackage is the arch header's `package:` field — the canonical short name for the file's
// home, which a path only approximates. "" when the file carries no header.
func headerPackage(f *ast.File) string {
	if f.Doc == nil {
		return ""
	}
	for _, cm := range f.Doc.List {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(cm.Text), "// package:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// rank orders the answer by how likely each hit is to be the one being looked for: the
// definition, then calls, then other references — production code before tests within each,
// because "who calls this" is usually asked about the shipping path. Path and line break ties,
// so the order is stable and diffable.
func rank(refs []Ref) {
	sort.SliceStable(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if ra, rb := tier(a), tier(b); ra != rb {
			return ra < rb
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
}

// tier is a Ref's rank band: kind first, and test files one step behind their own kind. A
// definition keeps its place wherever it lives — there is normally only one, and it is context
// for everything below it.
func tier(r Ref) int {
	if r.Kind == RefDef {
		return 0
	}
	var base int
	switch r.Kind {
	case RefCall:
		base = 1
	case RefOther:
		base = 3
	default:
		base = 5
	}
	if r.Test {
		base++ // a test hit sits one step behind its own kind, never ahead of a production one
	}
	return base
}

// WriteRefs renders a ranked reference report: a one-line tally, then the hits in sections, most
// relevant first. limit caps the hits printed (0 = all) and says how many were withheld, so an
// over-broad symbol still answers instead of flooding the terminal.
func WriteRefs(w io.Writer, roots []string, maxDepth int, q RefQuery, limit int) error {
	refs, err := Refs(roots, maxDepth, q)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		fmt.Fprintf(w, "no references to %s under %s\n", q.Symbol, strings.Join(roots, " "))
		fmt.Fprintf(w, "note: refs matches an exact, case-sensitive identifier — for a pattern use `brokkr map --grep`")
		if !q.Comments {
			fmt.Fprint(w, ", and --comments also searches prose")
		}
		fmt.Fprintln(w, ".")
		return nil
	}
	shown := refs
	if limit > 0 && len(refs) > limit {
		shown = refs[:limit]
	}
	// The tally counts EVERYTHING found, not just what fits the limit: "40 calls" followed by a
	// withheld note is the true answer, where tallying the printed rows would understate it.
	parts := make([]string, 0, 4)
	for _, g := range group(refs) {
		parts = append(parts, fmt.Sprintf("%d %s", len(g.refs), g.label))
	}
	fmt.Fprintf(w, "%s: %s\n", q.Symbol, strings.Join(parts, " · "))
	for _, g := range group(shown) {
		// The heading names the category (always plural); the tally above carries the counts.
		fmt.Fprintf(w, "\n── %s ──\n", label(g.refs[0], 0))
		for _, r := range g.refs {
			// path:line:col, as the linters report positions: the column separates two distinct
			// identifiers that share a line (`var Foo = pkg.Foo` is a definition AND a reference).
			fmt.Fprintf(w, "%s:%d:%d: %s%s\n", r.Path, r.Line, r.Col, r.Text, context(r))
		}
	}
	if n := len(refs) - len(shown); n > 0 {
		fmt.Fprintf(w, "\n… %d less-relevant hit(s) withheld (--limit 0 for all).\n", n)
	}
	return nil
}

// refGroup is one ranked section: its heading and the hits under it.
type refGroup struct {
	label string
	refs  []Ref
}

// group splits ranked refs into sections, preserving the ranking — so the heading order IS the
// relevance order, and each label is pluralised for what it actually holds.
func group(refs []Ref) []refGroup {
	var out []refGroup
	for _, r := range refs {
		key := sectionKey(r)
		if len(out) == 0 || sectionKey(out[len(out)-1].refs[0]) != key {
			out = append(out, refGroup{refs: []Ref{r}})
			continue
		}
		g := &out[len(out)-1]
		g.refs = append(g.refs, r)
	}
	for i := range out {
		out[i].label = label(out[i].refs[0], len(out[i].refs))
	}
	return out
}

// sectionKey identifies the band a hit belongs to, independent of how it is worded.
func sectionKey(r Ref) string {
	if r.Test {
		return r.Kind.String() + "/test"
	}
	return r.Kind.String()
}

// label words a section for the n hits it holds: "1 call", "6 calls", "1 call in tests".
func label(r Ref, n int) string {
	noun := r.Kind.String()
	if n != 1 {
		noun += "s"
	}
	if r.Test {
		noun += " in tests"
	}
	return noun
}

// context is the trailing "« where you landed": the file's arch package and the declaration the
// reference sits in. Both are omitted when absent rather than rendered as empty punctuation.
func context(r Ref) string {
	parts := make([]string, 0, 2)
	if r.Pkg != "" {
		parts = append(parts, r.Pkg)
	}
	if r.Encl != "" {
		parts = append(parts, r.Encl)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  « " + strings.Join(parts, " · ")
}

// walkGo calls fn for every .go file under each root, with the display path the map would use.
//
// codemap.go walks the same trees, but that walk is inlined inside write() and a concurrent task
// is editing it; folding the two together is a follow-up rather than a conflict today.
func walkGo(roots []string, maxDepth int, fileFilter string, fn func(disp, path string)) error {
	cwd, _ := os.Getwd()
	multi := len(roots) > 1
	for _, root := range roots { // fail loud before emitting anything, as the map does
		if _, err := os.Stat(root); err != nil {
			return err
		}
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] || (maxDepth >= 0 && dirDepth(root, path) > maxDepth) {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			disp := displayPath(root, cwd, path, multi)
			if fileFilter != "" && !strings.Contains(strings.ToLower(disp), fileFilter) {
				return nil
			}
			fn(disp, path)
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
