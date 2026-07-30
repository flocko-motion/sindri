// package: lint / comments
// type:    logic
// job:     the documentation linter — checks every non-test Go file opens with
// the canonical four-field header (package/type/job/limits, the same
// block code map reads) and that every exported func and type carries
// at least one line of doc comment.
// limits:  reports only; CLI wiring and exit codes live in cmd/sindri/lint.go.
// It checks the header's fields are PRESENT, not that the type value is
// one of the canonical kinds.
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

// canonicalHeaderFields are the four every header must carry (architecture spec, "File headers").
var canonicalHeaderFields = []string{"package", "type", "job", "limits"}

// DefaultMaxHeaderFieldLen bounds one field's content (continuations joined) so `brokkr map` stays
// compact. Only field values count, not free-form lines.
const DefaultMaxHeaderFieldLen = 300

// commentViol is one violation: where it is (line 0 = file-level) and what is missing.
type commentViol struct {
	path string
	line int
	msg  string
}

// Comments reports documentation violations in non-test source: a missing or incomplete canonical
// header, or an exported func or type with no doc comment.
func Comments(roots []string, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
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
		if !cap.Allow() {
			continue
		}
		fmt.Fprintln(w, v.msg)
	}
	cap.Note(w)
	return len(viols) > 0, nil
}

// checkTSHeader holds TS/JS to the same four-field header, so one convention covers the repo.
// Header only: the per-declaration rule needs a real parser, so eslint/tsc own that.
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
	header, _, _ = SplitHeader(header) // prose below the fields is measured, not folded into limits
	var viols []commentViol
	for _, stray := range strayFields(header.Text) {
		viols = append(viols, commentViol{path, header.Line, strayFieldMsg(path, header.Line, stray)})
	}
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

// headerFieldTooLong is the one wording, shared by the Go and TS checks so it cannot drift. It says
// what to do, because the tempting move is relocating the prose where the rule can't reach it.
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

// strayFieldMsg rejects an invented field: a fifth is prose the length rules cannot see.
func strayFieldMsg(path string, line int, field string) string {
	return fmt.Sprintf("%s:%d: non-standard header field %q — the header is exactly %s. Fold it into "+
		"one of those or drop it; an extra field is prose the length rules cannot see.",
		path, line, field, strings.Join(canonicalHeaderFields, "/"))
}

// strayFields finds labels that are not one of the four. An unknown `name:` folds into the field
// above and inflates its length: a 427-char `limits` was limits plus a `dev:`.
func strayFields(lines []string) []string {
	var out []string
	for i, l := range lines {
		name, _, found := strings.Cut(strings.TrimSpace(l), ":")
		if !found || name == "" || isHeaderFieldName(name) || !lowerWord(name) {
			continue
		}
		// A real field opens an entry, so the line above finishes one. Without this, "…the attach.
		// Client / side: it captures…" read `side:` as a fifth field.
		if i > 0 && !endsClause(lines[i-1]) {
			continue
		}
		out = append(out, name+":")
	}
	return out
}

// endsClause reports whether a line finishes what it was saying, so the next can open a new entry.
func endsClause(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return true
	}
	return strings.HasSuffix(t, ".") || strings.HasSuffix(t, ")") || strings.HasSuffix(t, ":")
}

// lowerWord reports whether s is a single lowercase identifier — the shape a field label has.
func lowerWord(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return s != ""
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

	if f.Doc != nil {
		ln := fset.Position(f.Doc.Pos()).Line
		for _, stray := range strayFields(docLinesNorm(f.Doc)) {
			viols = append(viols, commentViol{path, ln, strayFieldMsg(path, ln, stray)})
		}
	}
	// Only the four field values are bounded here — free-form prose after a blank line is not a
	// field, so headerFieldContent already stops folding at the gap. It is not exempt from
	// anything else, though: the comment-length average measures it like any other prose
	// (-> SplitHeader), so parking it above `package` no longer escapes that check.
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

// headerFieldContent joins each field with its continuations; a BLANK line ends it. Indentation does
// NOT mark a continuation — an indented one is what gofmt rewrites, so the style is flush-left.
func headerFieldContent(doc *ast.CommentGroup) map[string]string {
	out := map[string]string{}
	if doc == nil {
		return out
	}
	current := ""
	for _, c := range doc.List {
		for _, raw := range strings.Split(c.Text, "\n") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(raw, "//"))
			if field, val, ok := matchField(trimmed); ok {
				current = field
				out[field] = val
				continue
			}
			if current != "" && trimmed != "" {
				out[current] = strings.TrimSpace(out[current] + " " + trimmed)
			} else {
				current = "" // a blank line ends the field; what follows is free-form
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

// docLinesNorm strips comment markers (//, /* */, leading *) and surrounding whitespace.
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

// isGenerated finds the `Code generated … DO NOT EDIT.` line that exempts a file.
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

// recvTypeName is the bare type name from a T, *T, or generic T[P] receiver.
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
