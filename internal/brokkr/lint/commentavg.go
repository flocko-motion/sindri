// package: lint / commentavg
// type:    logic (comment-length trend)
// job:     hold each file's comments to a length TREND rather than a per-comment limit — the
//
//	mean lines per comment block must stay at or under a configured maximum, so a long
//	explanation is paid for by the short ones around it.
//
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

// DefaultMaxCommentAvg is the mean lines per comment block a file may average once it has enough
// comments to show a trend. A CEILING, not a target: one line answers most comments, and the
// occasional paragraph is paid for by them, because prose outweighing its code stops being read.
// Per repo via `lint: max_comment_avg:`.
const DefaultMaxCommentAvg = 2.0

// trendSample is the number of comment blocks at which a file's mean is taken at face value,
// and bonusPerBlock is how much slack each block short of that earns.
//
// A mean over two comments is not evidence of a trend, so a thin sample is forgiven and the
// limit tightens as the sample grows: at ten blocks the configured maximum applies exactly, at
// two it is five times as generous. Linear on purpose — a rule people have to predict before
// they write is worth more than a curve that fits a nicer shape.
const (
	trendSample   = 10
	bonusPerBlock = 0.5
)

// allowanceFor is the mean length a file with n comment blocks may reach, given the configured
// maximum.
func allowanceFor(base float64, n int) float64 {
	if n >= trendSample {
		return base
	}
	return base * (1 + bonusPerBlock*float64(trendSample-n))
}

// CommentAvg walks the given roots (default ".") and reports each file whose mean comment block
// runs longer than maxAvg lines. A non-positive maxAvg uses DefaultMaxCommentAvg. With blocks set,
// each reported file also lists every comment over the limit — line range, length and excerpt —
// so a fix is one pass instead of read, guess, edit, re-run.
//
// The file header is excluded from the statistic: it is a REQUIRED multi-line block (see
// Comments), so counting it would charge every file for obeying the header rule.
func CommentAvg(roots []string, maxAvg float64, blocks bool, ig *Ignore, w io.Writer) (bool, error) {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if maxAvg <= 0 {
		maxAvg = DefaultMaxCommentAvg
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
			bs := ScanComments(src)
			if _, hasHeader := HeaderBlock(bs); hasHeader {
				bs = bs[1:] // the header is mandated; it is not evidence of a trend
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
			if blocks {
				for _, b := range bs {
					if float64(b.Lines) > allowed {
						v.over = append(v.over, b)
					}
				}
				sort.Slice(v.over, func(i, j int) bool { return v.over[i].Lines > v.over[j].Lines })
			}
			viols = append(viols, v)
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	sort.Slice(viols, func(i, j int) bool { return viols[i].avg > viols[j].avg })
	for _, v := range viols {
		fmt.Fprintf(w, "%s: comments average %.1f lines (max %.1f over %d blocks) / %d comment lines; longest is %d lines at :%d\n",
			v.path, v.avg, v.allowed, v.blocks, v.lines, v.worst.Lines, v.worst.Line)
		if !blocks {
			if ex := firstProse(v.worst); ex != "" {
				fmt.Fprintf(w, "    %s…\n", trimTo(ex, 72))
			}
			continue
		}
		// The budget is allowed × blocks, so this is exactly how many comment lines have to go.
		// Printing it turns "trim and re-run until it passes" into one edit.
		fmt.Fprintf(w, "    cut %d comment line(s); %d block(s) run over the %.1f limit:\n",
			v.lines-int(v.allowed*float64(v.blocks)), len(v.over), v.allowed)
		for _, b := range v.over {
			fmt.Fprintf(w, "      %-11s %2d lines  %s\n",
				fmt.Sprintf(":%d-%d", b.Line, b.End), b.Lines, trimTo(firstProse(b), 58))
		}
	}
	if len(viols) > 0 {
		fmt.Fprintf(w, "%d file(s) over the comment-length trend — cut words, don't move them.\n"+
			"The maximum is a CEILING, not a target. A single line is enough for most comments: "+
			"name what the thing is for, or why it is not the obvious way. Aim well under the limit "+
			"— a file trimmed to sit exactly on it fails again the moment anyone adds a comment, and "+
			"the number passing is not the same as the prose being worth reading.\n"+
			"Relocating a comment, splitting one into several, or padding the file with one-liners "+
			"only shifts the average. `lint: max_comment_avg:` in .sindri/config.yaml is the "+
			"maintainer's setting, not a way past a finding.\n", len(viols))
	}
	return len(viols) > 0, nil
}

// firstProse returns a block's first line that says something, for the excerpt that names which
// comment is the long one. A block can open on a blank line, and "…" tells the reader nothing.
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
