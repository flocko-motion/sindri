// package: lint / comments
// type:    logic
// job:     the documentation linter — checks every non-test Go file opens with
//          the canonical four-field header (package/type/job/limits, the same
//          block code map reads) and that every exported func and type carries
//          at least one line of doc comment.
// limits:  reports only; CLI wiring and exit codes live in cmd/sindri/lint.go.
//          It checks the header's fields are PRESENT, not that the type value is
//          one of the canonical kinds.
package lint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// canonicalHeaderFields are the four fields every source file's header must
// carry, per the architecture spec's "File headers" requirement.
var canonicalHeaderFields = []string{"package", "type", "job", "limits"}

// DefaultMaxHeaderFieldLen bounds one header field's content (continuations joined) so headers
// stay compact in `brokkr map`. Only field values count, not free-form lines.
const DefaultMaxHeaderFieldLen = 300

// commentViol is one documentation violation: a file (and line, 0 = file-level)
// and the message describing what's missing.
type commentViol struct {
	path string
	line int
	msg  string
}

// Comments walks the given roots (default ".") for non-test .go files and reports
// documentation violations: a missing or incomplete canonical header, or an
// exported func or type with no doc comment. Paths matched by ig are skipped.
// Returns true if any was found.
func Comments(roots []string, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	var viols []commentViol
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			// Test files are exempt from the header rule (their subject is the file
			// they test); their decls are not public API either.
			if IsTestFile(path) || ig.Match(path) {
				return nil
			}
			switch LangOf(path) {
			case LangGo:
				viols = append(viols, checkFileComments(path)...)
			case LangTS:
				viols = append(viols, checkTSHeader(path)...)
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	sort.Slice(viols, func(i, j int) bool {
		if viols[i].path != viols[j].path {
			return viols[i].path < viols[j].path
		}
		return viols[i].line < viols[j].line
	})
	for _, v := range viols {
		fmt.Fprintln(w, v.msg)
	}
	return len(viols) > 0, nil
}

// checkTSHeader holds a TS/JS file to the same four-field header as a Go one, so one
// convention covers the repo. Header only: the per-declaration doc rule needs a real parser,
// so that is left to eslint/tsc.
func checkTSHeader(path string) []commentViol {
	src, ok := readSource(path)
	if !ok {
		return nil
	}
	if strings.Contains(src, "@generated") || strings.Contains(src, "eslint-disable -- generated") {
		return nil // generated files are not hand-authored — exempt
	}
	header, has := HeaderBlock(ScanComments(src))
	if !has {
		return []commentViol{{path, 1, fmt.Sprintf("%s: missing canonical header (a package/type/job/limits comment block at the top of the file)", path)}}
	}
	var viols []commentViol
	fields := headerFieldsFromLines(header.Text)
	var missing []string
	for _, f := range canonicalHeaderFields {
		if _, ok := fields[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		viols = append(viols, commentViol{path, header.Line,
			fmt.Sprintf("%s:%d: header missing field(s): %s", path, header.Line, strings.Join(missing, ", "))})
	}
	for _, f := range canonicalHeaderFields {
		if n := len(fields[f]); n > DefaultMaxHeaderFieldLen {
			viols = append(viols, commentViol{path, header.Line, headerFieldTooLong(path, header.Line, f, n)})
		}
	}
	return viols
}

// headerFieldTooLong is the one wording for an over-long header field, shared by the Go and TS
// checks so the instruction cannot drift. It says what to do because the tempting move is to
// relocate the prose below the imports, which the rule stops reaching but nobody benefits from.
func headerFieldTooLong(path string, line int, field string, n int) string {
	return fmt.Sprintf("%s:%d: header field %q is %d chars (max %d) — cut it down, don't move it. "+
		"The maximum is a ceiling, not a target: a good field is one short phrase, well under it. "+
		"Prose relocated out of the header just lands where a reader has less context and "+
		"`brokkr map` won't show it; the header is read first, so it has to be brief.",
		path, line, field, n, DefaultMaxHeaderFieldLen)
}

// headerFieldsFromLines reads `field: value` entries out of a header's lines, joining a
// continuation (an indented line with no `field:` of its own) onto the field above it.
func headerFieldsFromLines(lines []string) map[string]string {
	fields := map[string]string{}
	current := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		name, rest, found := strings.Cut(line, ":")
		if found && isHeaderFieldName(name) {
			current = strings.ToLower(strings.TrimSpace(name))
			fields[current] = strings.TrimSpace(rest)
			continue
		}
		if current != "" && line != "" {
			fields[current] += " " + line
		}
	}
	return fields
}

// isHeaderFieldName reports whether a `name:` prefix is one of the canonical fields.
func isHeaderFieldName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, f := range canonicalHeaderFields {
		if n == f {
			return true
		}
	}
	return false
}

// checkFileComments parses one file and returns its header and exported-doc
// violations.
func checkFileComments(path string) []commentViol {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return []commentViol{{path, 0, fmt.Sprintf("%s: parse error: %v", path, err)}}
	}
	if isGenerated(f) {
		return nil // generated files are not hand-authored — exempt
	}

	var viols []commentViol
	if missing := missingHeaderFields(f.Doc); len(missing) > 0 {
		if f.Doc == nil {
			viols = append(viols, commentViol{path, 1,
				fmt.Sprintf("%s: missing canonical header (a package/type/job/limits comment block directly above `package`)", path)})
		} else {
			viols = append(viols, commentViol{path, fset.Position(f.Doc.Pos()).Line,
				fmt.Sprintf("%s: header missing field(s): %s", path, strings.Join(missing, ", "))})
		}
	}

	// Only the four field values are bounded; extra free-form header lines are free.
	if f.Doc != nil {
		ln := fset.Position(f.Doc.Pos()).Line
		fc := headerFieldContent(f.Doc)
		for _, field := range canonicalHeaderFields {
			if n := len(fc[field]); n > DefaultMaxHeaderFieldLen {
				viols = append(viols, commentViol{path, ln, headerFieldTooLong(path, ln, field, n)})
			}
		}
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Public means reachable as API: an exported func, or an exported method on an
			// exported type. An exported method on a private struct is not.
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv != nil && !ast.IsExported(recvTypeName(d.Recv.List[0].Type)) {
				continue
			}
			if !hasDocText(d.Doc) {
				ln := fset.Position(d.Pos()).Line
				viols = append(viols, commentViol{path, ln,
					fmt.Sprintf("%s:%d: exported %s %s has no doc comment", path, ln, funcKind(d), funcName(d))})
			}
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, sp := range d.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				// The doc may sit on the GenDecl (single `type Foo …`) or on the spec
				// itself (inside a `type ( … )` group).
				if hasDocText(d.Doc) || hasDocText(ts.Doc) {
					continue
				}
				ln := fset.Position(ts.Pos()).Line
				viols = append(viols, commentViol{path, ln,
					fmt.Sprintf("%s:%d: exported type %s has no doc comment", path, ln, ts.Name.Name)})
			}
		}
	}
	return viols
}

// missingHeaderFields returns the canonical fields absent from a file header. package and type
// must carry a value on their label line; job and limits need only be present.
func missingHeaderFields(doc *ast.CommentGroup) []string {
	value := map[string]string{}
	present := map[string]bool{}
	for _, line := range docLinesNorm(doc) {
		for _, field := range canonicalHeaderFields {
			if v, ok := fieldValue(line, field); ok {
				present[field] = true
				value[field] = v
			}
		}
	}
	var missing []string
	for _, field := range canonicalHeaderFields {
		switch {
		case !present[field]:
			missing = append(missing, field)
		case (field == "package" || field == "type") && value[field] == "":
			missing = append(missing, field+" (empty)")
		}
	}
	return missing
}

// headerFieldContent maps each field to its value plus aligned continuations, joined. A blank
// or un-aligned line ends the field, so free-form lines aren't charged to it.
func headerFieldContent(doc *ast.CommentGroup) map[string]string {
	out := map[string]string{}
	if doc == nil {
		return out
	}
	current := ""
	for _, c := range doc.List {
		for _, raw := range strings.Split(c.Text, "\n") {
			body := strings.TrimPrefix(raw, "//") // keep leading indentation
			trimmed := strings.TrimSpace(body)
			if field, val, ok := matchField(trimmed); ok {
				current = field
				out[field] = val
				continue
			}
			indent := len(body) - len(strings.TrimLeft(body, " \t"))
			if current != "" && trimmed != "" && indent >= 4 { // aligned continuation
				out[current] = strings.TrimSpace(out[current] + " " + trimmed)
			} else {
				current = "" // blank or un-aligned comment — the field's content ends
			}
		}
	}
	return out
}

// matchField reports whether a normalized line is a "<field>: value" header line.
func matchField(line string) (field, value string, ok bool) {
	for _, f := range canonicalHeaderFields {
		if strings.HasPrefix(line, f+":") {
			return f, strings.TrimSpace(line[len(f)+1:]), true
		}
	}
	return "", "", false
}

// fieldValue returns the text after "<field>:" on a normalized header line.
func fieldValue(line, field string) (string, bool) {
	prefix := field + ":"
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(line[len(prefix):]), true
	}
	return "", false
}

// docLinesNorm returns a comment group's lines with their comment markers (//,
// /* */, leading *) and surrounding whitespace stripped.
func docLinesNorm(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	var out []string
	for _, c := range doc.List {
		for _, raw := range strings.Split(c.Text, "\n") {
			t := strings.TrimSpace(raw)
			t = strings.TrimPrefix(t, "//")
			t = strings.TrimPrefix(t, "/*")
			t = strings.TrimSuffix(t, "*/")
			t = strings.TrimPrefix(t, "*")
			out = append(out, strings.TrimSpace(t))
		}
	}
	return out
}

// hasDocText reports whether a comment group has at least one non-empty line.
func hasDocText(doc *ast.CommentGroup) bool {
	for _, l := range docLinesNorm(doc) {
		if l != "" {
			return true
		}
	}
	return false
}

// isGenerated reports whether a file is machine-generated (a `// Code generated …
// DO NOT EDIT.` line before the package clause), which exempts it.
func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			t := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if strings.HasPrefix(t, "Code generated ") && strings.HasSuffix(t, " DO NOT EDIT.") {
				return true
			}
		}
	}
	return false
}

// funcKind labels a func declaration as a "func" or a "method".
func funcKind(d *ast.FuncDecl) string {
	if d.Recv != nil {
		return "method"
	}
	return "func"
}

// funcName renders a func's name, qualified by its receiver type for a method.
func funcName(d *ast.FuncDecl) string {
	if d.Recv != nil && len(d.Recv.List) > 0 {
		if r := recvTypeName(d.Recv.List[0].Type); r != "" {
			return r + "." + d.Name.Name
		}
	}
	return d.Name.Name
}

// recvTypeName extracts the bare receiver type name from T, *T, or a generic
// T[P] / T[P, Q] receiver.
func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return ""
}
