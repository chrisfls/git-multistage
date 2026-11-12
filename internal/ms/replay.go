package ms

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chrisfls/git-multistage/internal/gitx"
	"github.com/chrisfls/git-multistage/internal/hunks"
)

// Git's three-way merge treats changes on adjacent lines as a conflict. Two
// agents that edit neighbouring lines of one file would then never both
// commit. replayLines covers that case: it replays each changed block of the
// staged file onto the tip at the same lines, and succeeds only when every
// block lands on lines that the tip left unchanged. A block whose lines the
// tip changed, or an insertion at a point the tip also changed, still
// conflicts.

// replayLines replays mineHunks, the change from base to the staged file,
// onto theirs. It returns false when a block cannot be placed unambiguously.
func replayLines(base, theirs []byte, mineHunks, theirHunks []hunks.Hunk) ([]byte, bool) {
	// changed[i] is set for base lines that theirs removed or replaced.
	baseLen := len(hunks.SplitLines(base))
	changed := make([]bool, baseLen)
	for _, t := range theirHunks {
		if t.OldLines > 0 {
			for i := t.OldStart - 1; i < t.OldStart-1+t.OldLines; i++ {
				changed[i] = true
			}
		}
	}
	// tipLine maps an unchanged base line to its line in theirs.
	tipLine := func(i int) int {
		off := 0
		for _, t := range theirHunks {
			end := t.OldStart // first base line after the hunk, 0-based
			if t.OldLines > 0 {
				end = t.OldStart - 1 + t.OldLines
			}
			if end <= i {
				off += t.NewLines - t.OldLines
			}
		}
		return i + off
	}
	// conflictAtGap reports whether theirs inserted lines at base gap g
	// (between base lines g-1 and g), or changed lines on both sides of it.
	conflictAtGap := func(g int) bool {
		for _, t := range theirHunks {
			if t.OldLines == 0 && t.OldStart == g {
				return true
			}
			if t.OldLines > 0 && t.OldStart-1 < g && g < t.OldStart-1+t.OldLines {
				return true
			}
		}
		return false
	}
	var placed []hunks.Hunk
	for _, h := range mineHunks {
		if slices.ContainsFunc(theirHunks, func(t hunks.Hunk) bool { return sameChange(h, t) }) {
			continue // theirs already made this exact change
		}
		if h.OldLines > 0 {
			s := h.OldStart - 1
			for i := s; i < s+h.OldLines; i++ {
				if changed[i] {
					return nil, false
				}
			}
			for g := s + 1; g < s+h.OldLines; g++ {
				if conflictAtGap(g) {
					return nil, false
				}
			}
			h.OldStart = tipLine(s) + 1
			placed = append(placed, h)
			continue
		}
		// A pure insertion after base line h.OldStart (gap g).
		g := h.OldStart
		if conflictAtGap(g) {
			return nil, false
		}
		switch {
		case g == 0:
			h.OldStart = 0
		case !changed[g-1]:
			h.OldStart = tipLine(g-1) + 1
		default:
			// Base line g-1 ends a block that theirs replaced: insert after
			// the replacement.
			for _, t := range theirHunks {
				if t.OldLines > 0 && t.OldStart-1+t.OldLines == g {
					h.OldStart = t.NewStart - 1 + t.NewLines
					if t.NewLines == 0 {
						h.OldStart = t.NewStart
					}
				}
			}
		}
		placed = append(placed, h)
	}
	out, err := hunks.Apply(theirs, placed)
	if err != nil {
		return nil, false
	}
	return out, true
}

func sameChange(a, b hunks.Hunk) bool {
	return a.OldStart == b.OldStart && a.OldLines == b.OldLines &&
		slices.Equal(a.Removed, b.Removed) && slices.Equal(a.Added, b.Added)
}

// replayConflicts resolves the content conflicts of a merge by line replay.
// It returns the resolved tree, or "" when some conflict is not a content
// conflict of a text file, or some block cannot be placed.
func (r *Repo) replayConflicts(baseTree, tipTree, staged string, res gitx.MergeResult) (string, error) {
	type resolved struct{ mode, oid, path string }
	var done []resolved
	for _, p := range res.Conflicts {
		if !res.ContentOnly[p] {
			return "", nil
		}
		b, okB, err := r.lsTree(baseTree, p)
		if err != nil {
			return "", err
		}
		t, okT, err := r.lsTree(tipTree, p)
		if err != nil {
			return "", err
		}
		m, okM, err := r.lsTree(staged, p)
		if err != nil {
			return "", err
		}
		if !okB || !okT || !okM || b.mode == "120000" || b.mode == "160000" {
			return "", nil
		}
		mode := t.mode
		if m.mode != b.mode {
			if t.mode != b.mode && t.mode != m.mode {
				return "", nil
			}
			mode = m.mode
		}
		mineHunks, err := r.diffBlobs(p, b.oid, m.oid)
		if err != nil {
			return "", nil // binary content
		}
		theirHunks, err := r.diffBlobs(p, b.oid, t.oid)
		if err != nil {
			return "", nil
		}
		baseContent, err := r.catBlob(b.oid)
		if err != nil {
			return "", err
		}
		tipContent, err := r.catBlob(t.oid)
		if err != nil {
			return "", err
		}
		out, ok := replayLines(baseContent, tipContent, mineHunks, theirHunks)
		if !ok {
			return "", nil
		}
		oid, err := r.Line(gitx.Opt{Stdin: bytes.NewReader(out)}, "hash-object", "-w", "--stdin")
		if err != nil {
			return "", err
		}
		done = append(done, resolved{mode, oid, p})
	}
	idx := filepath.Join(r.Dir, "tmp", fmt.Sprintf("replay-index-%d", os.Getpid()))
	if err := os.MkdirAll(filepath.Dir(idx), 0o777); err != nil {
		return "", err
	}
	os.Remove(idx)
	defer os.Remove(idx)
	if _, err := r.Run(gitx.Opt{Index: idx}, "read-tree", res.Tree); err != nil {
		return "", err
	}
	for _, d := range done {
		if _, err := r.Run(gitx.Opt{Index: idx}, "update-index", "--cacheinfo", d.mode, d.oid, d.path); err != nil {
			return "", err
		}
	}
	return r.WriteTree(idx)
}
