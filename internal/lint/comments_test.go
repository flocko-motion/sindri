package lint

import (
	"strings"
	"testing"
)

// goodHeader is a complete canonical four-field header.
const goodHeader = `// package: x
// type:    logic
// job:     does the thing
// limits:  doesn't do other things (-> elsewhere)
package x
`

func runComments(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := writeModule(t, files)
	var sb strings.Builder
	if _, err := Comments([]string{dir}, &sb); err != nil {
		t.Fatalf("Comments: %v", err)
	}
	return sb.String()
}

func TestCommentsCleanFilePasses(t *testing.T) {
	out := runComments(t, map[string]string{
		"x.go": goodHeader + `
// Foo does a thing.
func Foo() {}

// Bar is a thing.
type Bar struct{}

// Method does a thing.
func (b Bar) Method() {}
`,
	})
	if out != "" {
		t.Fatalf("clean file should report nothing, got:\n%s", out)
	}
}

func TestCommentsMissingHeaderAndField(t *testing.T) {
	out := runComments(t, map[string]string{
		"nohdr.go": "package x\nfunc f() {}\n",
		"partial.go": `// package: x
// type:    logic
// job:     does the thing
package x
`,
	})
	if !strings.Contains(out, "nohdr.go: missing canonical header") {
		t.Errorf("expected missing-header report, got:\n%s", out)
	}
	if !strings.Contains(out, "partial.go: header missing field(s): limits") {
		t.Errorf("expected missing-limits report, got:\n%s", out)
	}
}

func TestCommentsUndocumentedExports(t *testing.T) {
	out := runComments(t, map[string]string{
		"x.go": goodHeader + `
func Exported() {}

type Public struct{}

// documented stays quiet.
func Documented() {}

func unexported() {}
`,
	})
	if !strings.Contains(out, "exported func Exported has no doc comment") {
		t.Errorf("expected undocumented func report, got:\n%s", out)
	}
	if !strings.Contains(out, "exported type Public has no doc comment") {
		t.Errorf("expected undocumented type report, got:\n%s", out)
	}
	if strings.Contains(out, "Documented") || strings.Contains(out, "unexported") {
		t.Errorf("documented/unexported decls must not be reported, got:\n%s", out)
	}
}

func TestCommentsHeaderFieldTooLong(t *testing.T) {
	long := strings.Repeat("blah ", 80) // ~400 chars, over the budget
	out := runComments(t, map[string]string{
		"x.go": "// package: x\n// type:    logic\n// job:     " + long + "\n// limits:  y\npackage x\n",
	})
	if !strings.Contains(out, `header field "job"`) || !strings.Contains(out, "max 300") {
		t.Fatalf("expected a job-too-long violation, got:\n%s", out)
	}
}

func TestCommentsHeaderExtraCommentsNotCounted(t *testing.T) {
	// Short fields, plus a long free-form note in the header — the note must not
	// count against any field, so there should be no length violation.
	note := strings.Repeat("note ", 80)
	out := runComments(t, map[string]string{
		"x.go": "// package: x\n// type:    logic\n// job:     short\n// limits:  short\n//\n// " + note + "\npackage x\n",
	})
	if strings.Contains(out, "header field") {
		t.Fatalf("free-form header comments must not count against field length, got:\n%s", out)
	}
}

func TestCommentsMethodReceiverVisibility(t *testing.T) {
	out := runComments(t, map[string]string{
		"x.go": goodHeader + `
// Pub is exported.
type Pub struct{}

func (p Pub) Exposed() {}

type priv struct{}

func (p priv) Exposed() {}
`,
	})
	// An exported method on an exported type is public API → flagged.
	if !strings.Contains(out, "exported method Pub.Exposed has no doc comment") {
		t.Errorf("expected Pub.Exposed report, got:\n%s", out)
	}
	// An exported method on an unexported type is not public → not flagged.
	if strings.Contains(out, "priv.Exposed") {
		t.Errorf("method on an unexported type must not be reported, got:\n%s", out)
	}
}

func TestCommentsSkipsTestFiles(t *testing.T) {
	out := runComments(t, map[string]string{
		"x_test.go": "package x\nfunc Foo() {}\n", // no header, exported func
	})
	if out != "" {
		t.Fatalf("test files are exempt, got:\n%s", out)
	}
}

func TestCommentsTypeGroupPerSpecDoc(t *testing.T) {
	out := runComments(t, map[string]string{
		"x.go": goodHeader + `
type (
	// Documented is fine.
	Documented struct{}

	Bare struct{}
)
`,
	})
	if !strings.Contains(out, "exported type Bare has no doc comment") {
		t.Errorf("expected Bare report, got:\n%s", out)
	}
	if strings.Contains(out, "type Documented") {
		t.Errorf("Documented (per-spec doc) must not be reported, got:\n%s", out)
	}
}
