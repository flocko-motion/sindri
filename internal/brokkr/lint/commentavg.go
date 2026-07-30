// package: lint / commentavg
// type:    logic (comment-length trend)
// job:     hold each file's comments to a length TREND rather than a per-comment limit — the
// mean lines per comment block must stay at or under a configured maximum, so a long
// explanation is paid for by the short ones around it.
// limits:  reports only; comment scanning is lang.go's and the CLI wiring cmd/brokkr's.
package lint

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMaxCommentAvg is the mean lines per comment a file may average. A ceiling, not a target.
const DefaultMaxCommentAvg = 2.0

// DefaultMaxCommentLine caps one comment line's width. Without it the mean is gameable in the one
// direction that reads worst: fewer, longer lines pass a per-LINE budget while the prose grows.
const DefaultMaxCommentLine = 110

// trendSample is where a mean is taken at face value; bonusPerBlock is the slack each block short
// of it earns. Two comments are not a trend. Linear, so the rule is predictable before you write.
const (
	trendSample   = 10
	bonusPerBlock = 0.5
)

// systemicFiles is where long comments stop being a list of slips and become a house-style problem.
const systemicFiles = 10

// systemicBanner teaches the practice rather than restating the rule, which at this scale is
// plainly understood and plainly not followed. Rare, so it stays worth reading.
const systemicBanner = `
!! WARNING: comment length is a HOUSE-STYLE problem here, not a few long comments.

How to write them:
  - One line. Say what the thing is FOR, or why it is not done the obvious way.
  - Do not restate the signature, the types, or the control flow — the code has those.
  - A paragraph is for a decision a reader would otherwise undo: name the trap, once.
  - Delete rather than compress. Prose nobody reads costs more than prose that is missing.

What does NOT count as fixing it:
  - Trimming to land exactly on the limit. The next comment added fails again.
  - Moving prose out of a header, splitting one comment in two, padding with one-liners.
  - Raising ` + "`lint: max_comment_avg:`" + `. That is the maintainer's call, not a way past a finding.
`

// aimFor is the mean a fix should TARGET, below the ceiling it must clear: a file trimmed to the
// limit exactly fails again on the next comment added, so half a line of headroom is the goal.
func aimFor(allowed float64) float64 {
	aim := allowed - 0.5
	if aim < 1 {
		aim = 1 // one line per comment is the floor; below that there is nothing to aim at
	}
	if aim > allowed {
		return allowed // a ceiling already under a line leaves no room for margin
	}
	return aim
}

// allowanceFor is the mean a file with n comment blocks may reach, given the configured maximum.
func allowanceFor(base float64, n int) float64 {
	if n >= trendSample {
		return base
	}
	return base * (1 + bonusPerBlock*float64(trendSample-n))
}

// CommentAvg reports each file whose mean comment exceeds maxAvg; blocks adds the per-comment
// listing. Only the four-field header proper is excluded (it MUST be multi-line, so counting it
// would charge everyone) — free-form prose parked after it is measured like any other comment,
// so hiding a long explanation there no longer escapes the trend (-> SplitHeader).
func CommentAvg(roots []string, maxAvg float64, maxLine int, blocks bool, cap *Cap, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if maxAvg <= 0 {
		maxAvg = DefaultMaxCommentAvg
	}
	if maxLine <= 0 {
		maxLine = DefaultMaxCommentLine
	}

	type viol struct {
		path    string
		avg     float64
		allowed float64
		blocks  int
		lines   int
		worst   CommentBlock
		over    []CommentBlock // above the limit, longest first — what to cut
	}
	var viols []viol
	var wide []string // over-wide comment lines, reported alongside the trend

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
			if LangOf(path) == LangNone || IsTestFile(path) || ig.Match(path) {
				return nil
			}
			src, ok := readSource(path)
			if !ok {
				return nil
			}
			wide = append(wide, wideLines(path, src, maxLine)...)
			bs := ScanComments(src)
			if h, hasHeader := HeaderBlock(bs); hasHeader {
				// The header is mandated, so it is not evidence of a trend — but only the header
				// itself. Free-form prose parked below the fields is measured like the ordinary
				// comment it is, or the exemption would pay for hiding prose there (-> SplitHeader).
				bs = bs[1:]
				if _, prose, hasProse := SplitHeader(h); hasProse {
					bs = append([]CommentBlock{prose}, bs...)
				}
			}
			if len(bs) == 0 {
				return nil
			}
			total, worst := 0, CommentBlock{}
			for _, b := range bs {
				total += b.Lines
				if b.Lines > worst.Lines {
					worst = b
				}
			}
			allowed := allowanceFor(maxAvg, len(bs))
			avg := float64(total) / float64(len(bs))
			if avg <= allowed {
				return nil
			}
			v := viol{path: path, avg: avg, allowed: allowed, blocks: len(bs), lines: total, worst: worst}
			for _, b := range bs { // longest first: the default line names where to start
				if float64(b.Lines) > allowed {
					v.over = append(v.over, b)
				}
			}
			sort.Slice(v.over, func(i, j int) bool { return v.over[i].Lines > v.over[j].Lines })
			viols = append(viols, v)
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	// From the CONFIGURED max, so it is one number per run. Only the max adapts per file; an ideal
	// that moved with it would read "ideal 10.5" on a two-comment file.
	aim := aimFor(maxAvg)
	// Worst mean first, so a capped run withholds the files that need it least.
	sort.Slice(viols, func(i, j int) bool { return viols[i].avg > viols[j].avg })
	for _, v := range viols {
		if !cap.Allow() {
			continue
		}
		// Measured against the TARGET, not the ceiling: the bare number needed to pass anchors
		// people to the ceiling, so it is deliberately not shown.
		cut := v.lines - int(aim*float64(v.blocks))
		// One actionable line: how far over the target, what the ceiling is, and WHERE to edit. No
		// excerpt — you are going to open the file regardless, and truncated prose finds nothing.
		fmt.Fprintf(w, "%s: %.1f avg over %d blocks — %d line(s) over the %.1f target (%.1f is the ceiling); fix %s\n",
			v.path, v.avg, v.blocks, cut, aim, v.allowed, blockRanges(v.over, 8))
		if !blocks {
			continue
		}
		fmt.Fprintf(w, "    %d block(s) run over %.1f:\n", len(v.over), v.allowed)
		for _, b := range v.over {
			fmt.Fprintf(w, "      %-11s %2d lines  %s\n",
				fmt.Sprintf(":%d-%d", b.Line, b.End), b.Lines, trimTo(firstProse(b), 58))
		}
	}
	if len(viols) > 0 {
		cap.Note(w)
		// The band between target and ceiling is a judgement about whether prose earns its length,
		// which no mean can make — so name it as a choice with a reason owed, not as spare room.
		fmt.Fprintf(w, "%d file(s) over the comment-length trend — cut words, don't move them.\n"+
			"  Cut to %.1f unless these comments genuinely earn their length. Stopping between %.1f and %.1f\n"+
			"  is a claim that they do, and this check cannot judge that for you — so say which you chose\n"+
			"  and why. \"It passes\" is not that answer.\n", len(viols), aim, aim, maxAvg)
		if len(viols) >= systemicFiles && !cap.Quiet() {
			fmt.Fprint(w, systemicBanner)
		}
	}
	// Over-wide lines share the budget: they are findings in the same report, not a second pass.
	for _, msg := range wide {
		if !cap.Allow() {
			continue
		}
		fmt.Fprintln(w, msg)
	}
	if len(wide) > 0 {
		fmt.Fprintf(w, "%d comment line(s) over %d chars — wrap them; a long line is not a short comment.\n",
			len(wide), maxLine)
	}
	return len(viols) > 0 || len(wide) > 0, nil
}

// wideLines reports lines wider than max, headers included — the trend rule excuses those entirely.
func wideLines(path, src string, max int) []string {
	var out []string
	for i, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "/*") && !strings.HasPrefix(line, "*") {
			continue
		}
		if n := len([]rune(raw)); n > max {
			// Say HOW to wrap: an aligned continuation looks right and is what gofmt rewrites, so
			// the tool's advice would otherwise contradict the formatter.
			out = append(out, fmt.Sprintf("%s:%d: comment line is %d chars (max %d) — wrap it, "+
				"continuation FLUSH LEFT at column 3 (indenting it makes gofmt rewrite the comment)",
				path, i+1, n, max))
		}
	}
	return out
}

// blockRanges lists the ranges to edit, longest first — line numbers are what you act on.
func blockRanges(over []CommentBlock, max int) string {
	var b strings.Builder
	for i, blk := range over {
		if i == max {
			fmt.Fprintf(&b, " (+%d more)", len(over)-i)
			break
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, ":%d-%d", blk.Line, blk.End)
	}
	return b.String()
}

// firstProse is a block's first line that says something; it can open on a blank one.
func firstProse(b CommentBlock) string {
	for _, t := range b.Text {
		if strings.TrimSpace(t) != "" {
			return t
		}
	}
	return ""
}

// trimTo shortens s to at most n characters, for a one-line excerpt.
func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
