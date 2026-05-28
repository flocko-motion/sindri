// Package lint provides in-process static-analysis linters.
//
// The deadcode linter is a port of the glue in golang.org/x/tools/cmd/deadcode,
// reduced to the pieces this project needs and reworked to run as a library
// (returning errors instead of calling log.Fatal) with no dependency on
// x/tools internal packages.
package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"maps"
	"os"
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

// Deadcode loads the given package patterns, computes reachability from every
// main package's init+main via Rapid Type Analysis, and writes one
// "file:line:col: unreachable func: Name" line per unreachable source function
// to w (sorted by package, then file, then line). Reporting is limited to the
// module(s) of the loaded packages so dependencies are never flagged.
//
// Generated files, marker interface methods, and functions annotated with a
// //deadcode:keep directive are excluded. When anything is reported, a trailing
// note reminds the reader that the directive exists.
//
// It returns true if any unreachable function was reported, which callers can
// use as a non-zero exit gate.
func Deadcode(patterns []string, tags string, includeTests bool, w io.Writer) (found bool, err error) {
	cfg := &packages.Config{
		BuildFlags: []string{"-tags=" + tags},
		Mode:       packages.LoadAllSyntax | packages.NeedModule,
		Tests:      includeTests,
	}
	initial, err := packages.Load(cfg, patterns...)
	if err != nil {
		return false, fmt.Errorf("load: %w", err)
	}
	if len(initial) == 0 {
		return false, fmt.Errorf("no packages match %v", patterns)
	}
	if packages.PrintErrors(initial) > 0 {
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

	mains := ssautil.MainPackages(pkgs)
	if len(mains) == 0 {
		return false, fmt.Errorf("no main packages among %v", patterns)
	}
	var roots []*ssa.Function
	for _, main := range mains {
		roots = append(roots, main.Func("init"), main.Func("main"))
	}

	// Gather source-level functions and note generated files and the
	// interfaces declared per package (for marker-method detection). We ignore
	// synthetic wrappers and nested functions: an unreachable literal is always
	// a consequence of its parent being unreachable.
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
			if isMarkerMethod(fn, interfaceTypes[fn.Pkg.Pkg]) {
				continue
			}
			if hasKeepDirective(fn) {
				continue // explicitly kept by the author
			}
			fmt.Fprintf(w, "%s: unreachable func: %s\n", relPosition(posn), prettyName(fn))
			count++
		}
	}
	if count > 0 {
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

// prettyName renders a function's name without go/ssa's punctuation, e.g.
// "(*pkg.T).F" becomes "T.F".
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

// receiverNamed returns the named type associated with a method receiver of
// the form N or *N (or an alias thereof), and whether a pointer was present.
func receiverNamed(recv *types.Var) (isPtr bool, named *types.Named) {
	t := recv.Type()
	if ptr, ok := types.Unalias(t).(*types.Pointer); ok {
		isPtr = true
		t = ptr.Elem()
	}
	named, _ = types.Unalias(t).(*types.Named)
	return
}

// isMarkerMethod reports whether fn is a marker method: an unexported,
// empty-bodied method with no params or results that implements some named
// interface declared in the same package. These are intentionally never
// called directly, so reporting them as dead would be misleading.
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

// hasKeepDirective reports whether fn's declaration carries a //deadcode:keep
// directive, used to intentionally exclude an unreachable function from the
// report (e.g. it is called only via reflection, or kept as future API). The
// directive must sit directly above the func declaration, like other Go
// tool directives.
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
	filename := posn.Filename
	if rel, err := filepath.Rel(cwd, filename); err == nil && !strings.HasPrefix(rel, "..") {
		filename = rel
	}
	return fmt.Sprintf("%s:%d:%d", filename, posn.Line, posn.Column)
}
