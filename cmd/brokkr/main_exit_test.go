package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoRawOsExit enforces that os.Exit appears exactly once in brokkr's source —
// inside the exit() wrapper, which flushes the --tail buffer first. A raw os.Exit
// anywhere else would terminate before that flush and swallow buffered output.
func TestNoRawOsExit(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue // tests may exit however they like
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(string(src), "os.Exit("); n > 0 {
			count += n
			if f != "main.go" {
				t.Errorf("%s calls os.Exit directly — route exits through main.go's exit() so the --tail buffer flushes", f)
			}
		}
	}
	if count != 1 {
		t.Errorf("want exactly one os.Exit (the exit() wrapper), found %d", count)
	}
}
