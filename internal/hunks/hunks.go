// Package hunks parses zero-context unified diffs into hunks with stable
// identifiers, and applies a chosen subset of those hunks to the old content.
package hunks

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Hunk is one contiguous change of a zero-context diff.
type Hunk struct {
	ID       string   `json:"id"`
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Removed  []string `json:"-"` // old lines, each with its line ending
	Added    []string `json:"-"` // new lines, each with its line ending
	CopyID   string   `json:"-"` // the ID of h as one of several copies; see assignIDs
}

// Header returns the unified diff header of h.
func (h Hunk) Header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
}

// Lines returns the diff lines of h, prefixed with "-" or "+", without line endings.
// A line that has no final newline ends with "\ No newline at end of file".
func (h Hunk) Lines() []string {
	var out []string
	add := func(prefix string, l string) {
		if before, ok := strings.CutSuffix(l, "\n"); ok {
			out = append(out, prefix+before)
		} else {
			out = append(out, prefix+l, `\ No newline at end of file`)
		}
	}
	for _, l := range h.Removed {
		add("-", l)
	}
	for _, l := range h.Added {
		add("+", l)
	}
	return out
}

var headerRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// Side names the side of a diff whose start lines place identical copies.
type Side int

const (
	// NewSide places a copy by its start line in the new content.
	NewSide Side = iota
	// OldSide places a copy by its start line in the old content.
	OldSide
)

// ErrBinary means that Git reported the content as binary.
var ErrBinary = errors.New("binary content has no hunks")

// Parse parses the output of `git diff -U0` for one file and assigns each
// hunk an identifier derived from path and the hunk's changed lines. When
// several hunks have the same lines, their identifiers also depend on their
// start lines on the given side and on versions, which must change whenever
// that side can change other than by applying hunks of this diff.
func Parse(path string, diff []byte, versions string, side Side) ([]Hunk, error) {
	var hs []Hunk
	var cur *Hunk
	lines := strings.SplitAfter(string(diff), "\n")
	for i := range lines {
		l := lines[i]
		if l == "" {
			continue
		}
		switch {
		case strings.HasPrefix(l, "Binary files ") || strings.HasPrefix(l, "GIT binary patch"):
			return nil, ErrBinary
		case strings.HasPrefix(l, "@@ "):
			m := headerRE.FindStringSubmatch(l)
			if m == nil {
				return nil, fmt.Errorf("bad hunk header %q", l)
			}
			hs = append(hs, Hunk{
				OldStart: atoi(m[1]), OldLines: count(m[2]),
				NewStart: atoi(m[3]), NewLines: count(m[4]),
			})
			cur = &hs[len(hs)-1]
		case cur == nil:
			// File headers before the first hunk.
		case l[0] == '-':
			cur.Removed = append(cur.Removed, l[1:])
		case l[0] == '+':
			cur.Added = append(cur.Added, l[1:])
		case l[0] == '\\':
			// "\ No newline at end of file" belongs to the previous line.
			if i == 0 {
				continue
			}
			prev := lines[i-1]
			target := &cur.Added
			if prev[0] == '-' {
				target = &cur.Removed
			}
			if n := len(*target); n > 0 {
				(*target)[n-1] = strings.TrimSuffix((*target)[n-1], "\n")
			}
		default:
			return nil, fmt.Errorf("unexpected diff line %q", l)
		}
	}
	for i := range hs {
		h := &hs[i]
		if len(h.Removed) != h.OldLines || len(h.Added) != h.NewLines {
			return nil, fmt.Errorf("hunk %s has %d/%d lines", h.Header(), len(h.Removed), len(h.Added))
		}
	}
	assignIDs(path, hs, versions, side)
	return hs, nil
}

// assignIDs gives each hunk a 12-hex-digit hash of its path and changed
// lines, so the ID survives line shifts. Several hunks of one file can have
// the same lines; each such copy hashes, in addition, its start line on side
// and versions, which names the versions of the diffed content. Any
// change to those versions makes the IDs of copies unknown, so a selection
// cannot silently move to another copy. The ID of a copy does not depend on
// the other copies, so staging one copy keeps the IDs of the others.
//
// CopyID is the ID that a hunk has as a copy. A hunk whose last other copy
// was staged keeps it, so that selections can still name it by that ID.
func assignIDs(path string, hs []Hunk, versions string, side Side) {
	hash := func(h Hunk, place string) string {
		sum := sha256.New()
		sum.Write([]byte(path))
		sum.Write([]byte{0})
		for _, l := range h.Removed {
			sum.Write([]byte("-" + l + "\x00"))
		}
		for _, l := range h.Added {
			sum.Write([]byte("+" + l + "\x00"))
		}
		sum.Write([]byte(place))
		return hex.EncodeToString(sum.Sum(nil))[:12]
	}
	count := map[string]int{}
	for i := range hs {
		hs[i].ID = hash(hs[i], "")
		count[hs[i].ID]++
	}
	for i := range hs {
		// Hunks of one diff are apart by unchanged lines, so no two copies
		// start at the same line on either side.
		start := hs[i].NewStart
		if side == OldSide {
			start = hs[i].OldStart
		}
		hs[i].CopyID = hash(hs[i], fmt.Sprintf("@%d\x00%s", start, versions))
		if count[hs[i].ID] > 1 {
			hs[i].ID = hs[i].CopyID
		}
	}
}

// SplitLines splits content into lines that keep their line endings.
func SplitLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	ls := strings.SplitAfter(string(content), "\n")
	if ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// Apply applies the chosen hunks, which must come from a diff of old, to old.
func Apply(old []byte, chosen []Hunk) ([]byte, error) {
	src := SplitLines(old)
	hs := append([]Hunk{}, chosen...)
	sort.Slice(hs, func(i, j int) bool { return hs[i].OldStart < hs[j].OldStart })
	var b strings.Builder
	pos := 0 // next unconsumed line of src, 0-based
	for _, h := range hs {
		start := h.OldStart - 1 // first removed line, 0-based
		if h.OldLines == 0 {
			start = h.OldStart // insertion after line OldStart
		}
		if start < pos || start+h.OldLines > len(src) {
			return nil, fmt.Errorf("hunk %s does not fit the old content", h.ID)
		}
		for _, l := range src[pos:start] {
			b.WriteString(l)
		}
		for k, l := range h.Removed {
			if src[start+k] != l {
				return nil, fmt.Errorf("hunk %s does not match the old content", h.ID)
			}
		}
		for _, l := range h.Added {
			b.WriteString(l)
		}
		pos = start + h.OldLines
	}
	for _, l := range src[pos:] {
		b.WriteString(l)
	}
	return []byte(b.String()), nil
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func count(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}
