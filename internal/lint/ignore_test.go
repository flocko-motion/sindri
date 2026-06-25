package lint

import "testing"

func TestIgnoreMatch(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		path     string
		want     bool
	}{
		{"basename glob matches at root", []string{"*.gen.go"}, "foo.gen.go", true},
		{"basename glob matches nested", []string{"*.gen.go"}, "internal/x/foo.gen.go", true},
		{"basename glob normalizes dot-slash", []string{"*.gen.go"}, "./internal/foo.gen.go", true},
		{"basename glob misses non-match", []string{"*.gen.go"}, "internal/foo.go", false},
		{"basename glob stays within a segment", []string{"*.go"}, "a/b.go", true},

		{"path glob matches direct child", []string{"internal/gen/*"}, "internal/gen/a.go", true},
		{"single star doesn't cross a slash", []string{"internal/gen/*"}, "internal/gen/sub/a.go", false},
		{"double star crosses slashes", []string{"internal/gen/**"}, "internal/gen/sub/a.go", true},
		{"leading **/ allows zero dirs", []string{"**/mock.go"}, "mock.go", true},
		{"leading **/ allows many dirs", []string{"**/mock.go"}, "a/b/mock.go", true},
		{"path glob anchored — no partial", []string{"gen/*.go"}, "internal/gen/a.go", false},

		{"regex prefix searches unanchored", []string{`re:\.pb\.go$`}, "api/v1/svc.pb.go", true},
		{"regex prefix misses non-match", []string{`re:\.pb\.go$`}, "api/v1/svc.go", false},

		{"any pattern matching wins", []string{"*.gen.go", "internal/gen/**"}, "internal/gen/x.go", true},
		{"no patterns matches nothing", nil, "anything.go", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ig, err := NewIgnore(c.patterns)
			if err != nil {
				t.Fatalf("NewIgnore(%v): %v", c.patterns, err)
			}
			if got := ig.Match(c.path); got != c.want {
				t.Errorf("Match(%q) with %v = %v, want %v", c.path, c.patterns, got, c.want)
			}
		})
	}
}

// A nil *Ignore must be safe to call and match nothing — the linters pass nil
// when --ignore is unset.
func TestIgnoreNilMatchesNothing(t *testing.T) {
	var ig *Ignore
	if ig.Match("anything.go") {
		t.Error("nil *Ignore must match nothing")
	}
}

// A malformed pattern must fail loudly at compile time, not silently match
// nothing.
func TestIgnoreBadPatternErrors(t *testing.T) {
	if _, err := NewIgnore([]string{"re:("}); err == nil {
		t.Error("expected an error for an invalid regexp pattern")
	}
}
