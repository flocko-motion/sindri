// package: hub/workflow / commitmsg
// type:    logic (Conventional Commits formatting)
// job:     compose the message for every commit sindri creates: infer a type from a
// task's declared type, scope it to the task id, and normalize the description (an
// agent's summary, or the task's title) into a shape a Conventional Commits linter accepts.
// limits:  pure formatting — no store reads, no git.
package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// headerMax is commitlint's default header-max-length; over it, desc moves to the body
// instead of truncating silently.
const headerMax = 100

// ccPrefix matches a Conventional Commits prefix an agent already wrote on its own summary,
// stripped so sindri's own type always wins rather than nesting "fix(sd-1): fix: retry".
var ccPrefix = regexp.MustCompile(`(?i)^(feat|fix|chore|docs|style|refactor|perf|test|build|ci|revert)(\([^)]*\))?!?:\s*`)

// ccType maps a task's declared type to the Conventional Commits type it signals: bug and an
// imported issue as a fix, feature and epic as a feat, anything else as a chore.
func ccType(taskType string) string {
	switch taskType {
	case "bug", "issue":
		return "fix"
	case "feature", "epic":
		return "feat"
	default:
		return "chore"
	}
}

// conventionalCommit composes type(taskID): desc against commitlint's default rules, not just
// the bare spec — real agent prose and task titles routinely trip its case, punctuation and
// length checks. taskID is the scope, kept grep-able; empty (the openspec branch, not one task)
// omits it rather than leaving it blank.
func conventionalCommit(taskType, taskID, desc string) string {
	desc = normalizeDesc(desc)
	t := ccType(taskType)
	prefix := t + ": "
	if taskID != "" {
		prefix = fmt.Sprintf("%s(%s): ", t, taskID)
	}
	if len(prefix)+len(desc) <= headerMax {
		return prefix + desc
	}
	// Header gets a word-safe fragment; body carries the full desc, so nothing is lost.
	return prefix + truncateAtWord(desc, headerMax-len(prefix)) + "\n\n" + desc
}

// normalizeDesc turns free-form prose into a valid Conventional Commits subject: any existing
// prefix stripped, a trailing full stop dropped, the leading word decapitalized unless it looks
// like an identifier or acronym.
func normalizeDesc(desc string) string {
	desc = strings.TrimSpace(desc)
	desc = ccPrefix.ReplaceAllString(desc, "")
	desc = strings.TrimRight(desc, ".")
	desc = strings.TrimSpace(desc)
	return lowerFirst(desc)
}

// lowerFirst decapitalizes s's first rune, unless the first word has a second uppercase letter
// of its own (URL, OAuth, gRPC) — an identifier, not sentence case.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if !unicode.IsUpper(r) {
		return s
	}
	firstWord := s
	if i := strings.IndexFunc(s, unicode.IsSpace); i >= 0 {
		firstWord = s[:i]
	}
	for _, c := range firstWord[size:] {
		if unicode.IsUpper(c) {
			return s
		}
	}
	return string(unicode.ToLower(r)) + s[size:]
}

// truncateAtWord returns the longest prefix of s that fits max bytes without splitting a word.
func truncateAtWord(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, ' '); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ")
}
