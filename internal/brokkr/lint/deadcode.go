// package: lint / deadcode
// type:    logic
// job:     the dead-code linter — a library port of x/tools/cmd/deadcode that
// reports source functions unreachable from any main (via RTA).
// limits:  reports only; CLI wiring and exit codes live in cmd/sindri/lint.go.
package lint

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Deadcode reports functions unreachable from a main's init+main, within the loaded modules only.
// Generated files, marker methods, //deadcode:keep and ig matches are excluded; tests count.
func Deadcode(patterns []string, tags string, cap *Cap, ig *Ignore, w io.Writer) (found bool, err error) {
	// The Go toolchain is optional, so its absence is a visible skip rather than a failure.
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(w, "deadcode: go toolchain not found on PATH — skipping (optional)")
		fmt.Fprintln(w, "    hint: install Go and the dead-code check runs.")
		return false, nil
	}
	// Go is one language brokkr lints, not a precondition. A TypeScript-only project matches no
	// packages, and loading it would fail the gate before the checks that DO apply ever ran.
	if !hasGoSources(".") {
		fmt.Fprintln(w, "deadcode: no Go sources here — skipping (not a Go project)")
		return false, nil
	}
	cfg := &packages.Config{
		BuildFlags: []string{"-tags=" + tags},
		Mode:       packages.LoadAllSyntax | packages.NeedModule,
		Tests:      true,
	}
	initial, err := packages.Load(cfg, patterns...)
	if err != nil {
		// A toolchain behind go.mod fails the load for a reason that is not in the code: say so.
		if old := ToolchainAdvice(err.Error()); old != "" {
			return false, errors.New(old)
		}
		return false, fmt.Errorf("load: %w", err)
	}
	if len(initial) == 0 {
		return false, fmt.Errorf("no packages match %v", patterns)
	}
	if packages.PrintErrors(initial) > 0 {
		if old := ToolchainAdvice(loadErrorText(initial)); old != "" {
			return false, errors.New(old)
		}
		return false, fmt.Errorf("packages contain errors")
	}

	// Restrict reporting to the loaded module(s); never flag dependencies.
	filter, err := moduleFilter(initial)
	if err != nil {
		return false, err
	}

	// Build SSA and locate main packages (the reachability roots).
	prog, pkgs := ssautil.AllPackages(initial, ssa.InstantiateGenerics)
	prog.Build()

	// Reachability needs roots, and a main package is where they come from. A library — or any
	// subdirectory scoped without one — has none, so there is nothing this analysis can say. SKIPPED,
	// not failed: a valid package shape must not fail a gate, the same reason a TypeScript-only tree
	// is skipped above. It exited 1 with "no main packages", which reads as a finding about the code.
	mains := ssautil.MainPackages(pkgs)
	if len(mains) == 0 {
		fmt.Fprintf(w, "deadcode: no main package under %v — skipping (nothing to trace reachability from; "+
			"run it where a main lives, or over the whole module)\n", patterns)
		return false, nil
	}
	var roots []*ssa.Function
	for _, main := range mains {
		roots = append(roots, main.Func("init"), main.Func("main"))
	}

	// Source funcs, generated files, per-package interfaces. Wrappers and nested funcs are skipped:
	// an unreachable literal is only ever a consequence of its parent.
	var (
		sourceFuncs    []*ssa.Function
		generated      = make(map[string]bool)
		interfaceTypes = make(map[*types.Package][]*types.Interface)
	)
	packages.Visit(initial, nil, func(p *packages.Package) {
		var interfaces []*types.Interface
		scope := p.Types.Scope()
		for _, name := range scope.Names() {
			if tn, ok := scope.Lookup(name).(*types.TypeName); ok && types.IsInterface(tn.Type()) {
				interfaces = append(interfaces, tn.Type().Underlying().(*types.Interface))
			}
		}
		interfaceTypes[p.Types] = interfaces

		for _, file := range p.Syntax {
			for _, decl := range file.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					obj := p.TypesInfo.Defs[fd.Name].(*types.Func)
					sourceFuncs = append(sourceFuncs, prog.FuncValue(obj))
				}
			}
			if ast.IsGenerated(file) {
				generated[p.Fset.File(file.Pos()).Name()] = true
			}
		}
	})

	res := rta.Analyze(roots, false)

	// De-duplicate test variants of the same source function by position: if
	// any variant is reachable, treat them all as reachable.
	reachablePosn := make(map[token.Position]bool)
	for fn := range res.Reachable {
		if fn.Pos().IsValid() || fn.Name() == "init" {
			reachablePosn[prog.Fset.Position(fn.Pos())] = true
		}
	}

	// Group unreachable functions by package path.
	byPkgPath := make(map[string]map[*ssa.Function]bool)
	for _, fn := range sourceFuncs {
		posn := prog.Fset.Position(fn.Pos())
		if reachablePosn[posn] {
			continue
		}
		reachablePosn[posn] = true // suppress duplicates sharing a position

		pkgpath := fn.Pkg.Pkg.Path()
		m := byPkgPath[pkgpath]
		if m == nil {
			m = make(map[*ssa.Function]bool)
			byPkgPath[pkgpath] = m
		}
		m[fn] = true
	}

	var count int
	for _, pkgpath := range slices.Sorted(maps.Keys(byPkgPath)) {
		if !filter.MatchString(pkgpath) {
			continue
		}
		fns := slices.Collect(maps.Keys(byPkgPath[pkgpath]))
		sort.Slice(fns, func(i, j int) bool {
			x := prog.Fset.Position(fns[i].Pos())
			y := prog.Fset.Position(fns[j].Pos())
			if x.Filename != y.Filename {
				return x.Filename < y.Filename
			}
			return x.Line < y.Line
		})
		for _, fn := range fns {
			posn := prog.Fset.Position(fn.Pos())
			if generated[posn.Filename] {
				continue // skip generated files
			}
			if ig.Match(relFilename(posn.Filename)) {
				continue // excluded by --ignore
			}
			if isMarkerMethod(fn, interfaceTypes[fn.Pkg.Pkg]) {
				continue
			}
			if hasKeepDirective(fn) {
				continue // explicitly kept by the author
			}
			count++ // counted even when withheld, so the total below stays honest
			if !cap.Allow() {
				continue
			}
			fmt.Fprintf(w, "%s: unreachable func: %s\n", relPosition(posn), prettyName(fn))
		}
	}
	if count > 0 {
		cap.Note(w)
		fmt.Fprintf(w, "\n%d unreachable function(s) found.\n", count)
		fmt.Fprintln(w, "note: add a //deadcode:keep comment directly above a function to keep it (excludes it from this report).")
	}
	return count > 0, nil
}

// moduleFilter builds a regexp matching the import paths of the modules that
// own the initial packages, so only first-party code is reported.
func moduleFilter(initial []*packages.Package) (*regexp.Regexp, error) {
	seen := make(map[string]bool)
	var patterns []string
	for _, pkg := range initial {
		if pkg.Module != nil && pkg.Module.Path != "" && !seen[pkg.Module.Path] {
			seen[pkg.Module.Path] = true
			patterns = append(patterns, regexp.QuoteMeta(pkg.Module.Path))
		}
	}
	if patterns == nil {
		return regexp.Compile("") // match anything
	}
	return regexp.Compile("^(" + strings.Join(patterns, "|") + ")\\b")
}

// prettyName drops go/ssa's punctuation: "(*pkg.T).F" becomes "T.F".
func prettyName(fn *ssa.Function) string {
	var buf strings.Builder
	var format func(*ssa.Function)
	format = func(fn *ssa.Function) {
		if parent := fn.Parent(); parent != nil {
			format(parent)
			fmt.Fprintf(&buf, "$%d", slices.Index(parent.AnonFuncs, fn)+1)
			return
		}
		if recv := fn.Signature.Recv(); recv != nil {
			if _, named := receiverNamed(recv); named != nil {
				buf.WriteString(named.Obj().Name())
				buf.WriteByte('.')
			}
		}
		buf.WriteString(fn.Name())
	}
	format(fn)
	return buf.String()
}

// receiverNamed unwraps a receiver of the form N or *N (or an alias) to its named type.
func receiverNamed(recv *types.Var) (isPtr bool, named *types.Named) {
	t := recv.Type()
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		isPtr = true
		t = ptr.Elem()
	}
	named, _ = types.Unalias(t).(*types.Named)
	return
}

// isMarkerMethod reports whether fn is an unexported empty method implementing a local interface.
// Nothing calls one directly, so reporting it as dead would mislead.
func isMarkerMethod(fn *ssa.Function, interfaceTypes []*types.Interface) bool {
	if !(fn.Signature.Recv() != nil &&
		!ast.IsExported(fn.Name()) &&
		fn.Signature.Params() == nil &&
		fn.Signature.Results() == nil) {
		return false
	}
	syntax, ok := fn.Syntax().(*ast.FuncDecl)
	if !ok || syntax.Body == nil || len(syntax.Body.List) > 0 {
		return false
	}
	return slices.ContainsFunc(interfaceTypes, func(iface *types.Interface) bool {
		return types.Implements(fn.Signature.Recv().Type(), iface)
	})
}

// hasKeepDirective reports whether fn carries //deadcode:keep — reached only by reflection, or
// kept as future API. Directly above the declaration, as Go's own directives are.
func hasKeepDirective(fn *ssa.Function) bool {
	decl, ok := fn.Syntax().(*ast.FuncDecl)
	if !ok || decl.Doc == nil {
		return false
	}
	for _, c := range decl.Doc.List {
		if strings.HasPrefix(strings.TrimSpace(c.Text), "//deadcode:keep") {
			return true
		}
	}
	return false
}

var cwd, _ = os.Getwd()

// relPosition renders a position with a cwd-relative filename when possible.
func relPosition(posn token.Position) string {
	return fmt.Sprintf("%s:%d:%d", relFilename(posn.Filename), posn.Line, posn.Column)
}

// relFilename is the cwd-relative form the report and --ignore matching both use.
func relFilename(filename string) string {
	if rel, err := filepath.Rel(cwd, filename); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filename
}
