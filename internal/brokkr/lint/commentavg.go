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

// DefaultMaxCommentAvg is the mean lines per comment block a file may average once it has
// enough comments to show a trend. Most comments should be a line or two; one that needs a
// paragraph is paid for by them, because prose outweighing the code it describes stops being
// read. Per repo via `lint: max_comment_avg:`.
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

// CommentAvg walks the given roots (default ".") and reports each file whose mean comment
// block runs longer than maxAvg lines. A non-positive maxAvg uses DefaultMaxCommentAvg.
//
// The file header is excluded from the statistic: it is a REQUIRED multi-line block (see
// Comments), so counting it would charge every file for obeying the header rule.
func CommentAvg(roots []string, maxAvg float64, ig *Ignore, w io.Writer) (bool, error) {
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
			blocks := ScanComments(src)
			if _, hasHeader := HeaderBlock(blocks); hasHeader {
				blocks = blocks[1:] // the header is mandated; it is not evidence of a trend
			}
			if len(blocks) == 0 {
				return nil
			}
			total, worst := 0, CommentBlock{}
			for _, b := range blocks {
				total += b.Lines
				if b.Lines > worst.Lines {
					worst = b
				}
			}
			allowed := allowanceFor(maxAvg, len(blocks))
			if avg := float64(total) / float64(len(blocks)); avg > allowed {
				viols = append(viols, viol{path, avg, allowed, len(blocks), total, worst})
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	sort.Slice(viols, func(i, j int) bool { return viols[i].avg > viols[j].avg })
	for _, v := range viols {
		fmt.Fprintf(w, "%s: comments average %.1f lines (allowed %.1f over %d blocks) / %d comment lines; longest is %d lines at :%d\n",
			v.path, v.avg, v.allowed, v.blocks, v.lines, v.worst.Lines, v.worst.Line)
		if ex := firstProse(v.worst); ex != "" {
			fmt.Fprintf(w, "    %s…\n", trimTo(ex, 72))
		}
	}
	if len(viols) > 0 {
		fmt.Fprintf(w, "%d file(s) over the comment-length trend. Tune it per repo with `lint: max_comment_avg:` in .sindri/config.yaml.\n", len(viols))
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
