package main

import (
	"strings"
	"testing"

	"github.com/flo-at/sindri/internal/hub"
)

// TestResolveRepo: a selector resolves by repo name or tag; a miss and an ambiguous
// name both error (the latter tells the user to use the tag).
func TestResolveRepo(t *testing.T) {
	repos := []hub.RepoSummary{
		{Tag: "aaa111", Name: "sindri", Path: "/repos/sindri"},
		{Tag: "bbb222", Name: "herdr", Path: "/repos/herdr"},
		{Tag: "ccc333", Name: "herdr", Path: "/other/herdr"}, // same basename, different repo
	}

	if tag, err := resolveRepo(repos, "sindri"); err != nil || tag != "aaa111" {
		t.Fatalf("by name: got %q, %v", tag, err)
	}
	if tag, err := resolveRepo(repos, "ccc333"); err != nil || tag != "ccc333" {
		t.Fatalf("by tag: got %q, %v", tag, err)
	}
	if _, err := resolveRepo(repos, "nope"); err == nil {
		t.Fatal("a non-matching selector should error")
	}
	_, err := resolveRepo(repos, "herdr")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("an ambiguous name should error with a hint, got %v", err)
	}
}
