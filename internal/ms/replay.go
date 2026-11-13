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
	tv := newTheirView(len(hunks.SplitLines(base)), theirHunks)
	var placed []hunks.Hunk
	for _, h := range mineHunks {
		if slices.ContainsFunc(theirHunks, func(t hunks.Hunk) bool { return sameChange(h, t) }) {
			continue // theirs already made this exact change
		}
		var ok bool
		if h.OldLines > 0 {
			h, ok = tv.placeReplacement(h)
		} else {
			h, ok = tv.placeInsertion(h)
		}
		if !ok {
			return nil, false
		}
		placed = append(placed, h)
	}
	out, err := hunks.Apply(theirs, placed)
	if err != nil {
		return nil, false
	}
	// A block placed after a tip line without a final newline would join
	// two lines into one; such a result has fewer lines than it should.
	want := len(hunks.SplitLines(theirs))
	for _, h := range placed {
		want += h.NewLines - h.OldLines
	}
	if len(hunks.SplitLines(out)) != want {
		return nil, false
	}
	return out, true
}

// theirView answers where theirs changed the base.
type theirView struct {
	hunks   []hunks.Hunk
	changed []bool // set for base lines that theirs removed or replaced
}

func newTheirView(baseLen int, theirHunks []hunks.Hunk) *theirView {
	changed := make([]bool, baseLen)
	for _, t := range theirHunks {
		if t.OldLines > 0 {
			for i := t.OldStart - 1; i < t.OldStart-1+t.OldLines; i++ {
				changed[i] = true
			}
		}
	}
	return &theirView{theirHunks, changed}
}

// tipLine maps an unchanged base line to its line in theirs.
func (tv *theirView) tipLine(i int) int {
	off := 0
	for _, t := range tv.hunks {
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

// addsAtGap reports whether theirs adds lines at base gap g (between base
// lines g-1 and g): an insertion there, or a replacement that starts or ends
// there. Two sides adding lines at one point have no defined order, and
// identical additions would be duplicated.
func (tv *theirView) addsAtGap(g int) bool {
	for _, t := range tv.hunks {
		start := t.OldStart // gap before the hunk's first base line
		if t.OldLines > 0 {
			start = t.OldStart - 1
		}
		end := start + t.OldLines
		if t.OldLines == 0 && start == g {
			return true
		}
		if t.OldLines > 0 && t.NewLines > 0 && (start == g || end == g) {
			return true
		}
	}
	return false
}

// insideTheirs reports whether gap g lies strictly inside a block that
// theirs replaced or deleted.
func (tv *theirView) insideTheirs(g int) bool {
	for _, t := range tv.hunks {
		if t.OldLines > 0 && t.OldStart-1 < g && g < t.OldStart-1+t.OldLines {
			return true
		}
	}
	return false
}

// pureInsertAt reports whether theirs inserted lines (without replacing
// any) at gap g.
func (tv *theirView) pureInsertAt(g int) bool {
	for _, t := range tv.hunks {
		if t.OldLines == 0 && t.OldStart == g {
			return true
		}
	}
	return false
}

// placeReplacement moves h, which replaces or deletes base lines, to the
// same lines of theirs. It fails when theirs changed those lines or added
// lines among or next to them.
func (tv *theirView) placeReplacement(h hunks.Hunk) (hunks.Hunk, bool) {
	s := h.OldStart - 1
	for i := s; i < s+h.OldLines; i++ {
		if tv.changed[i] {
			return h, false
		}
	}
	for g := s + 1; g < s+h.OldLines; g++ {
		if tv.pureInsertAt(g) || tv.insideTheirs(g) {
			return h, false
		}
	}
	// Their insertion right before or after my replaced block has no
	// defined order relative to my new lines.
	if h.NewLines > 0 && (tv.pureInsertAt(s) || tv.pureInsertAt(s+h.OldLines)) {
		return h, false
	}
	h.OldStart = tv.tipLine(s) + 1
	return h, true
}

// placeInsertion moves h, a pure insertion after base line h.OldStart
// (gap g), to the same point of theirs.
func (tv *theirView) placeInsertion(h hunks.Hunk) (hunks.Hunk, bool) {
	g := h.OldStart
	if tv.addsAtGap(g) || tv.insideTheirs(g) {
		return h, false
	}
	switch {
	case g == 0:
		h.OldStart = 0
	case !tv.changed[g-1]:
		h.OldStart = tv.tipLine(g-1) + 1
	default:
		// Base line g-1 ends a block that theirs replaced: insert after
		// the replacement.
		for _, t := range tv.hunks {
			if t.OldLines > 0 && t.OldStart-1+t.OldLines == g {
				h.OldStart = t.NewStart - 1 + t.NewLines
				if t.NewLines == 0 {
					h.OldStart = t.NewStart
				}
			}
		}
	}
	return h, true
}

func sameChange(a, b hunks.Hunk) bool {
	return a.OldStart == b.OldStart && a.OldLines == b.OldLines &&
		slices.Equal(a.Removed, b.Removed) && slices.Equal(a.Added, b.Added)
}

// replayConflicts resolves the content conflicts of a merge by line replay.
// It returns the resolved tree, or "" when some conflict is not a content
// conflict of a text file, or some block cannot be placed.
func (r *Repo) replayConflicts(baseTree, tipTree, staged string, res gitx.MergeResult) (string, error) {
	var done []resolvedPath
	for _, p := range res.Conflicts {
		if !res.ContentOnly[p] {
			return "", nil
		}
		d, ok, err := r.replayPath(baseTree, tipTree, staged, p)
		if err != nil || !ok {
			return "", err
		}
		done = append(done, d)
	}
	idx := filepath.Join(r.Dir, "tmp", fmt.Sprintf("replay-index-%d", os.Getpid()))
	if err := os.MkdirAll(filepath.Dir(idx), 0o777); err != nil {
		return "", err
	}
	_ = os.Remove(idx)
	defer func() { _ = os.Remove(idx) }()
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

type resolvedPath struct{ mode, oid, path string }

// replayPath resolves the content conflict of path p by line replay. It
// reports false when p is not a text file in all three trees, the modes
// conflict, or some block cannot be placed.
func (r *Repo) replayPath(baseTree, tipTree, staged, p string) (resolvedPath, bool, error) {
	b, t, m, ok, err := r.threeEntries(baseTree, tipTree, staged, p)
	if err != nil || !ok {
		return resolvedPath{}, false, err
	}
	mode := t.mode
	if m.mode != b.mode {
		if t.mode != b.mode && t.mode != m.mode {
			return resolvedPath{}, false, nil
		}
		mode = m.mode
	}
	mineHunks, err := r.diffBlobs(p, b.oid, m.oid)
	if err != nil {
		return resolvedPath{}, false, nil //nolint:nilerr // binary content cannot be replayed
	}
	theirHunks, err := r.diffBlobs(p, b.oid, t.oid)
	if err != nil {
		return resolvedPath{}, false, nil //nolint:nilerr // binary content cannot be replayed
	}
	baseContent, err := r.catBlob(b.oid)
	if err != nil {
		return resolvedPath{}, false, err
	}
	tipContent, err := r.catBlob(t.oid)
	if err != nil {
		return resolvedPath{}, false, err
	}
	out, ok := replayLines(baseContent, tipContent, mineHunks, theirHunks)
	if !ok {
		return resolvedPath{}, false, nil
	}
	oid, err := r.Line(gitx.Opt{Stdin: bytes.NewReader(out)}, "hash-object", "-w", "--stdin")
	if err != nil {
		return resolvedPath{}, false, err
	}
	return resolvedPath{mode, oid, p}, true, nil
}

// threeEntries reads p from the base, tip, and staged trees. It reports
// false when p is missing from one of them, or is a symbolic link or
// submodule in the base.
func (r *Repo) threeEntries(baseTree, tipTree, staged, p string) (b, t, m indexEntry, ok bool, err error) {
	b, okB, err := r.lsTree(baseTree, p)
	if err != nil {
		return b, t, m, false, err
	}
	t, okT, err := r.lsTree(tipTree, p)
	if err != nil {
		return b, t, m, false, err
	}
	m, okM, err := r.lsTree(staged, p)
	if err != nil {
		return b, t, m, false, err
	}
	ok = okB && okT && okM && b.mode != "120000" && b.mode != "160000"
	return b, t, m, ok, nil
}
