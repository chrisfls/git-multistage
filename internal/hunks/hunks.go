// Package hunks parses zero-context unified diffs into hunks with stable
// identifiers, and applies a chosen subset of those hunks to the old content.
package hunks

import (
	"crypto/sha256"
	"encoding/hex"
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
		if strings.HasSuffix(l, "\n") {
			out = append(out, prefix+strings.TrimSuffix(l, "\n"))
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

// ErrBinary means that Git reported the content as binary.
var ErrBinary = fmt.Errorf("binary content has no hunks")

// Parse parses the output of `git diff -U0` for one file and assigns each
// hunk an identifier derived from path and the hunk's changed lines.
func Parse(path string, diff []byte) ([]Hunk, error) {
	var hs []Hunk
	var cur *Hunk
	lines := strings.SplitAfter(string(diff), "\n")
	for i := 0; i < len(lines); i++ {
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
	assignIDs(path, hs)
	return hs, nil
}

// assignIDs gives each hunk a 12-hex-digit hash of its path and changed lines.
// Identical hunks in one file get "-2", "-3", ... in file order.
func assignIDs(path string, hs []Hunk) {
	seen := map[string]int{}
	for i := range hs {
		sum := sha256.New()
		sum.Write([]byte(path))
		sum.Write([]byte{0})
		for _, l := range hs[i].Removed {
			sum.Write([]byte("-" + l + "\x00"))
		}
		for _, l := range hs[i].Added {
			sum.Write([]byte("+" + l + "\x00"))
		}
		id := hex.EncodeToString(sum.Sum(nil))[:12]
		seen[id]++
		if n := seen[id]; n > 1 {
			id += "-" + strconv.Itoa(n)
		}
		hs[i].ID = id
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
