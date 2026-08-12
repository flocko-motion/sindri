// package: hub/task / ids
// type:    logic (task-id scheme)
// job:     own the whole question of what a task id means — which source it belongs to, whether
// sindri owns it, and what a new one looks like. The prefixes live here and nowhere else.
// limits:  the id string only; who may act on a task is the source's own business.
package task

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Owner is the system a task id belongs to. Every id carries its owner in its prefix, so routing a
// call never has to ask each source in turn whether it recognises the thing.
type Owner int

const (
	OwnerUnknown  Owner = iota
	OwnerSindri         // sindri's own store — the hub is the source of truth
	OwnerOpenSpec       // an openspec change, mirrored
	OwnerGitHub         // a GitHub issue, mirrored
)

// The prefixes. THIS IS THE ONLY PLACE THEY ARE WRITTEN — a caller that compares one itself is the
// drift this package exists to end.
const (
	mintPrefix     = "sd-" // what a new sindri-owned id gets
	legacyTDPrefix = "td-" // sindri-owned ids minted before mintPrefix, and the ids td itself used
	specPrefix     = "os-"
	githubPrefix   = "gh-"
)

// legacyOwnedPrefixes are prefixes that still mean "sindri owns this" but are no longer minted.
// Nothing rewrites an existing id: it is embedded in that task's PR id, its git branch name and its
// agent's worktree path, so renaming it would orphan a live branch and a running workspace. Old ids
// keep working until their tasks close; only new ones take the current prefix.
var legacyOwnedPrefixes = []string{legacyTDPrefix}

// MintID returns a new id for a task sindri owns: the mint prefix plus six hex characters, the
// shape td used and openspec still uses. Random rather than sequential, so two repos never collide
// and an id carries no ordering anyone could read meaning into.
func MintID() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}
	return mintPrefix + hex.EncodeToString(b[:]), nil
}

// OwnerOf reports which system an id belongs to, OwnerUnknown for anything unrecognised.
func OwnerOf(id string) Owner {
	switch {
	case IsOwned(id):
		return OwnerSindri
	case strings.HasPrefix(id, specPrefix):
		return OwnerOpenSpec
	case strings.HasPrefix(id, githubPrefix):
		return OwnerGitHub
	}
	return OwnerUnknown
}

// IsOwned reports whether sindri owns this task — true for the current mint prefix AND for every
// legacy one, because ownership did not change when the prefix did.
func IsOwned(id string) bool {
	if strings.HasPrefix(id, mintPrefix) {
		return true
	}
	for _, p := range legacyOwnedPrefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// IsID reports whether s is a whole task id, rather than prose that merely starts like one — the
// question asked of an argument that MIGHT be an id, where OwnerOf's prefix match is too loose.
// `comment` takes an optional id ahead of free text, and "sd-1c3041 is the task this corrects"
// carries the mint prefix while being a sentence.
func IsID(s string) bool {
	if OwnerOf(s) == OwnerUnknown {
		return false
	}
	_, rest, _ := strings.Cut(s, "-") // every prefix ends in "-", so OwnerOf matching guarantees the cut
	if rest == "" {
		return false
	}
	return !strings.ContainsFunc(rest, func(r rune) bool {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_':
			return false
		}
		return true
	})
}

// IsLegacyTD reports whether an id came from the td tool's own numbering.
//
// This is NOT "does sindri own it", and the two must not be swapped. The td import reads rows out
// of a td database and keeps the td-prefixed ones; asking it with the mint prefix instead would
// make the one code path whose entire purpose is carrying a td backlog across silently import
// nothing, on the day the mint prefix changes.
func IsLegacyTD(id string) bool { return strings.HasPrefix(id, legacyTDPrefix) }

// SpecID builds an openspec task id from an already-hashed change name.
func SpecID(hash string) string { return specPrefix + hash }

// GitHubID is the stable task id for a GitHub issue number.
func GitHubID(number int) string { return githubPrefix + strconv.Itoa(number) }

// GitHubNumber reverses GitHubID; ok=false for an id belonging to any other source.
func GitHubNumber(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, githubPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return 0, false
	}
	return n, true
}
