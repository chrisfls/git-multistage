package ms

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// integrate replays the change from baseTree, the tree of the commit base,
// to staged onto the tree of tip. It first moves the paths that the
// Multistage-Moved trailers of base..tip name in both trees, so the change
// lands where the branch moved the files. It returns the merged tree, or an
// *Error with ExitConflict when the change cannot be replayed safely. Each
// path of recreate that the branch deleted is replayed as a new file.
// stagedMoves are the moves that the change stages.
func (r *Repo) integrate(base, baseTree, staged, tip string, stagedMoves []Move, recreate []string) (string, error) {
	tipTree, err := r.TreeOf(tip)
	if err != nil {
		return "", err
	}
	if tipTree == baseTree {
		return staged, nil
	}
	if baseTree, staged, err = r.followBranch(base, tip, baseTree, staged, tipTree, stagedMoves, recreate); err != nil {
		return "", err
	}
	res, err := r.MergeTrees(baseTree, tipTree, staged)
	if err != nil {
		return "", err
	}
	if len(res.Conflicts) > 0 {
		replayed, err := r.replayConflicts(baseTree, tipTree, staged, res)
		if err != nil {
			return "", err
		}
		if replayed != "" {
			if err := r.verifyIntegration(baseTree, staged, tipTree, replayed); err != nil {
				return "", err
			}
			return replayed, nil
		}
		return "", &Error{
			Code: ExitConflict, Kind: "conflict",
			Message: "staged changes conflict with " + short(tip),
			Paths:   res.Conflicts, Details: res.Messages,
			Hint: "the staged changes are kept; edit the files so they include the branch's changes, " +
				"then `git multistage restore --staged <path>` and stage them again",
		}
	}
	if err := r.verifyIntegration(baseTree, staged, tipTree, res.Tree); err != nil {
		return "", err
	}
	return res.Tree, nil
}

// followBranch moves the entries of baseTree and staged that the
// Multistage-Moved trailers of base..tip moved, then checks for paths that
// base..tip deleted, and returns the two trees. Only entries that baseTree
// holds move, so a file that the stage added stays where the stage put it.
// A staged move of a path that a trailer moved elsewhere is a conflict.
func (r *Repo) followBranch(base, tip, baseTree, staged, tipTree string, stagedMoves []Move, recreate []string) (string, string, error) {
	if base == "" || tip == "" {
		return baseTree, staged, nil
	}
	moves, err := r.trailerMoves(base + ".." + tip)
	if err != nil {
		return "", "", err
	}
	for oldp := range moves {
		if _, ok, err := r.lsTree(baseTree, oldp); err != nil || !ok {
			if err != nil {
				return "", "", err
			}
			delete(moves, oldp)
		}
	}
	if err := movedTwice(stagedMoves, moves, tip); err != nil {
		return "", "", err
	}
	if baseTree, err = r.moveEntries(baseTree, moves); err != nil {
		return "", "", err
	}
	if staged, err = r.moveEntries(staged, moves); err != nil {
		return "", "", err
	}
	baseTree, err = r.checkDeleted(base, tip, baseTree, staged, tipTree, stagedMoves, followMoves(recreate, moves))
	return baseTree, staged, err
}

// movedTwice fails with conflict when a staged move moves a path that the
// trailers moves moved to another path, so the stage's trailer would name a
// path that the parent commit lacks.
func movedTwice(stagedMoves []Move, moves map[string]string, tip string) error {
	var paths, details []string
	for _, m := range stagedMoves {
		if n, ok := moves[m.Old]; ok && n != m.New {
			paths = append(paths, m.Old)
			details = append(details, fmt.Sprintf("the stage moves %s to %s, but a commit on the branch moved it to %s", m.Old, m.New, n))
		}
	}
	if len(paths) == 0 {
		return nil
	}
	return &Error{
		Code: ExitConflict, Kind: "conflict",
		Message: "staged moves conflict with moves on " + short(tip),
		Paths:   paths, Details: details,
		Hint: "the staged changes are kept; unstage the move with `git multistage restore --staged <path>` " +
			"for both of its paths, then move the file again",
	}
}

// trailerMoves returns the moves that the Multistage-Moved trailers of the
// commits of revs name, as a map from each moved path to the path where the
// last of them put it. The trailers of one commit move their paths at the
// same time, and each commit's moves apply to the result of the earlier ones.
func (r *Repo) trailerMoves(revs ...string) (map[string]string, error) {
	out, err := r.Run(gitx.Opt{}, append([]string{cmdLog, "--reverse", "--format=%x1e%(trailers:key=" + trailerMoved + ",valueonly)"}, revs...)...)
	if err != nil {
		return nil, err
	}
	moves := map[string]string{}
	for _, commit := range strings.Split(string(out), "\x1e") {
		step := map[string]string{}
		for _, line := range strings.Split(commit, "\n") {
			oldp, newp, ok := parseMoveTrailer(strings.TrimSpace(line))
			if _, seen := step[oldp]; ok && !seen {
				step[oldp] = newp
			}
		}
		reached := map[string]bool{}
		for k, v := range moves {
			reached[v] = true
			if n, ok := step[v]; ok {
				moves[k] = n
			}
		}
		for oldp, newp := range step {
			if _, moved := moves[oldp]; !moved && !reached[oldp] {
				moves[oldp] = newp
			}
		}
	}
	return moves, nil
}

// moveTrailer returns the value of the Multistage-Moved trailer of m.
func moveTrailer(m Move) string { return quotePath(m.Old) + " -> " + quotePath(m.New) }

// parseMoveTrailer splits the value of a Multistage-Moved trailer into its
// old and new paths.
func parseMoveTrailer(v string) (string, string, bool) {
	var oldp, rest string
	var ok bool
	if strings.HasPrefix(v, `"`) {
		if oldp, rest, ok = cutQuoted(v); ok {
			rest, ok = strings.CutPrefix(rest, " -> ")
		}
	} else {
		oldp, rest, ok = strings.Cut(v, " -> ")
	}
	if !ok {
		return "", "", false
	}
	newp := rest
	if strings.HasPrefix(rest, `"`) {
		var tail string
		if newp, tail, ok = cutQuoted(rest); !ok || tail != "" {
			return "", "", false
		}
	}
	return oldp, newp, oldp != "" && newp != ""
}

// quotePath returns p in double quotes with C-style escapes, as `git diff`
// writes paths, when p holds " -> ", a double quote, a backslash, or a
// control character; otherwise p.
func quotePath(p string) string {
	plain := !strings.Contains(p, " -> ") && !strings.ContainsAny(p, `"\`) &&
		!strings.ContainsFunc(p, func(c rune) bool { return c < 0x20 || c == 0x7f })
	if plain {
		return p
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := range len(p) {
		c := p[i]
		if j := strings.IndexByte(cEscapes, c); j >= 0 && c != 0 {
			b.WriteByte('\\')
			b.WriteByte(cEscapeNames[j])
		} else if c < 0x20 || c == 0x7f {
			fmt.Fprintf(&b, "\\%03o", c)
		} else {
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// The bytes that a C-style quoted path escapes by name, and their names.
const (
	cEscapes     = "\a\b\f\n\r\t\v\"\\"
	cEscapeNames = "abfnrtv\"\\"
)

// cutQuoted reads the C-style quoted string at the start of s, and returns
// it unquoted with the rest of s.
func cutQuoted(s string) (string, string, bool) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			return b.String(), s[i+1:], true
		case c != '\\':
			b.WriteByte(c)
			continue
		case i+1 >= len(s):
			return "", "", false
		}
		i++
		if j := strings.IndexByte(cEscapeNames, s[i]); j >= 0 {
			b.WriteByte(cEscapes[j])
			continue
		}
		if i+3 > len(s) {
			return "", "", false
		}
		n, err := strconv.ParseUint(s[i:i+3], 8, 8)
		if err != nil {
			return "", "", false
		}
		b.WriteByte(byte(n))
		i += 2
	}
	return "", "", false
}

// followMoves returns paths, with the path where moves put each moved one.
func followMoves(paths []string, moves map[string]string) []string {
	out := slices.Clone(paths)
	for _, p := range paths {
		if n, ok := moves[p]; ok {
			out = append(out, n)
		}
	}
	return out
}

// moveEntries returns tree with each entry that moves names at its new path.
// All the moves happen at once, so one entry may take the path that another
// leaves. An entry whose new path tree holds otherwise stays.
func (r *Repo) moveEntries(tree string, moves map[string]string) (string, error) {
	if len(moves) == 0 {
		return tree, nil
	}
	return r.editTree(tree, func(idx string) (bool, error) {
		entries, err := r.freeMoves(idx, moves)
		if err != nil {
			return false, err
		}
		if len(entries) == 0 {
			return false, nil
		}
		olds := make([]string, 0, len(entries))
		for oldp := range entries {
			olds = append(olds, oldp)
		}
		slices.Sort(olds)
		if _, err := r.Run(gitx.Opt{Index: idx}, append([]string{"update-index", "--force-remove", "--"}, olds...)...); err != nil {
			return false, err
		}
		for _, oldp := range olds {
			if err := r.setIndexBlob(idx, moves[oldp], entries[oldp].mode, entries[oldp].oid); err != nil {
				return false, err
			}
		}
		return true, nil
	})
}

// freeMoves returns the entries of the index file idx that moves can move:
// each entry whose new path is free, or is left by another entry that moves.
func (r *Repo) freeMoves(idx string, moves map[string]string) (map[string]indexEntry, error) {
	entries := map[string]indexEntry{}
	taken := map[string]bool{}
	for oldp, newp := range moves {
		e, ok, err := r.lsIndex(idx, oldp)
		if err != nil {
			return nil, err
		}
		if ok && oldp != newp {
			entries[oldp] = e
		}
		if _, ok, err = r.lsIndex(idx, newp); err != nil {
			return nil, err
		}
		taken[newp] = ok
	}
	// Drop each move whose new path an entry that stays holds, until every
	// left move has a free path.
	for changed := true; changed; {
		changed = false
		for oldp := range entries {
			newp := moves[oldp]
			if _, leaves := entries[newp]; taken[newp] && !leaves {
				delete(entries, oldp)
				changed = true
			}
		}
	}
	return entries, nil
}

// editTree returns tree after edit changed it in a temporary index. edit
// reports whether it changed anything.
func (r *Repo) editTree(tree string, edit func(idx string) (bool, error)) (string, error) {
	dir, err := r.tempDir("tree-")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	idx := filepath.Join(dir, "index")
	if _, err := r.Run(gitx.Opt{Index: idx}, "read-tree", tree); err != nil {
		return "", err
	}
	changed, err := edit(idx)
	if err != nil || !changed {
		return tree, err
	}
	return r.WriteTree(idx)
}

// checkDeleted fails with path-deleted when a commit of base..tip deleted a
// path that the change from baseTree to staged modifies or moves. It returns
// baseTree without the deleted paths of recreate, so that the merge adds
// them; a move is recreated when recreate names either of its paths.
func (r *Repo) checkDeleted(base, tip, baseTree, staged, tipTree string, stagedMoves []Move, recreate []string) (string, error) {
	mine, err := r.DiffTrees(baseTree, staged, false)
	if err != nil {
		return "", err
	}
	theirs, err := r.DiffTrees(baseTree, tipTree, true)
	if err != nil {
		return "", err
	}
	deleted, recreated := deletedPaths(mine, theirs, stagedMoves, recreate)
	if len(deleted) > 0 {
		e := &Error{
			Code: ExitConflict, Kind: "path-deleted",
			Message: "the staged changes modify files that a commit on the branch deleted",
			Paths:   deleted,
			Hint: "the staged changes are kept; commit a file as new with `git multistage commit --recreate <path>`, " +
				"or unstage it with `git multistage restore --staged <path>`",
		}
		for _, p := range deleted {
			e.Details = append(e.Details, p+" was deleted by "+r.deletingCommit(p, base+".."+tip))
		}
		return "", e
	}
	if len(recreated) == 0 {
		return baseTree, nil
	}
	return r.editTree(baseTree, func(idx string) (bool, error) {
		_, err := r.Run(gitx.Opt{Index: idx}, append([]string{"update-index", "--force-remove", "--"}, recreated...)...)
		return true, err
	})
}

// deletedPaths returns the paths that mine modifies, or moves away, and
// that theirs deletes: the paths that recreate names, directly or by the new
// path of their move, and the others.
func deletedPaths(mine, theirs []gitx.Entry, stagedMoves []Move, recreate []string) (deleted, recreated []string) {
	deletedOnTip := map[string]bool{}
	for _, e := range theirs {
		if e.Status == 'D' {
			deletedOnTip[e.Path] = true
		}
	}
	movedTo := map[string]string{}
	for _, m := range stagedMoves {
		movedTo[m.Old] = m.New
	}
	for _, e := range mine {
		newp, moved := movedTo[e.Path]
		switch {
		case !deletedOnTip[e.Path]:
		case e.Status != 'M' && e.Status != 'T' && (e.Status != 'D' || !moved):
		case slices.Contains(recreate, e.Path) || moved && slices.Contains(recreate, newp):
			recreated = append(recreated, e.Path)
		default:
			deleted = append(deleted, e.Path)
		}
	}
	return deleted, recreated
}

// deletingCommit names the newest commit of revs that deleted p.
func (r *Repo) deletingCommit(p string, revs ...string) string {
	args := append([]string{cmdLog, "-1", "--no-renames", "--diff-filter=D", "--format=%h %s"}, revs...)
	line, err := r.Line(gitx.Opt{}, append(args, "--", p)...)
	if err != nil || line == "" {
		return "a commit on the branch"
	}
	return line
}

// verifyIntegration checks the merged tree against the staged intent:
// the result may differ from the tip only at staged paths (or where the tip
// renamed a staged path), and each staged path that the tip did not touch
// must hold exactly the staged content.
func (r *Repo) verifyIntegration(baseTree, staged, tipTree, result string) error {
	mine, err := r.DiffTrees(baseTree, staged, false)
	if err != nil {
		return err
	}
	theirs, err := r.DiffTrees(baseTree, tipTree, true)
	if err != nil {
		return err
	}
	theirsPlain, err := r.DiffTrees(baseTree, tipTree, false)
	if err != nil {
		return err
	}
	out, err := r.DiffTrees(tipTree, result, false)
	if err != nil {
		return err
	}
	if outside := pathsOutside(mine, theirs, out); len(outside) > 0 {
		return &Error{
			Code: ExitConflict, Kind: "outside-paths",
			Message: "the merged result changes paths that are not staged; refusing to commit it",
			Paths:   outside,
		}
	}
	if missing := missingContent(mine, theirsPlain, out); len(missing) > 0 {
		return &Error{
			Code: ExitConflict, Kind: "missing-content",
			Message: "the merged result does not contain the staged content; refusing to commit it",
			Paths:   missing,
		}
	}
	return nil
}

// pathsOutside lists the paths that out changes although mine does not
// stage them, also through a rename in theirs.
func pathsOutside(mine, theirs, out []gitx.Entry) []string {
	allowed := map[string]bool{}
	for _, e := range mine {
		allowed[e.Path] = true
	}
	for _, e := range theirs {
		if e.Status == 'R' && allowed[e.OldPath] {
			allowed[e.Path] = true
		}
	}
	var outside []string
	for _, e := range out {
		if !allowed[e.Path] {
			outside = append(outside, e.Path)
		}
	}
	return outside
}

// missingContent lists the staged paths of mine that theirs did not touch
// and whose staged content out does not hold.
func missingContent(mine, theirs, out []gitx.Entry) []string {
	got := map[string]gitx.Entry{}
	for _, e := range out {
		got[e.Path] = e
	}
	touched := map[string]bool{}
	for _, e := range theirs {
		touched[e.Path] = true
	}
	var missing []string
	for _, e := range mine {
		if touched[e.Path] {
			continue
		}
		g, ok := got[e.Path]
		if !ok || g.NewMode != e.NewMode || g.NewOID != e.NewOID {
			missing = append(missing, e.Path)
		}
	}
	return missing
}

// Freshness describes how a stage's base relates to its branch.
type Freshness struct {
	Branch    string   `json:"branch"`
	Head      string   `json:"head_branch"` // the branch HEAD points to now
	Base      string   `json:"base"`
	Tip       string   `json:"tip"`
	Stale     bool     `json:"stale"` // the base is behind the tip and cannot be moved
	Conflicts []string `json:"conflicts,omitempty"`
	Reason    string   `json:"reason,omitempty"`
}

// refresh moves the stage's base to the branch tip, replaying the staged
// changes onto it. When the replay is not safe, the stage stays as it is and
// the result reports it as stale.
func (l *Stage) refresh() (Freshness, error) {
	f := Freshness{Branch: l.M.Branch, Base: l.M.Base}
	head, err := l.r.headTarget()
	if err != nil {
		return f, err
	}
	f.Head = head
	tip, err := l.r.ResolveRef(l.M.Branch)
	if err != nil {
		return f, err
	}
	f.Tip = tip
	if head != l.M.Branch {
		f.Stale = tip != l.M.Base
		f.Reason = fmt.Sprintf("HEAD is %s, but this stage targets %s", describeTarget(head), describeTarget(l.M.Branch))
		return f, nil
	}
	if tip == l.M.Base {
		return f, nil
	}
	if reason := l.cannotMoveTo(tip); reason != "" {
		f.Stale, f.Reason = true, reason
		return f, nil
	}
	return l.moveBase(f, tip)
}

// cannotMoveTo returns why the stage's base cannot move to tip, or "".
func (l *Stage) cannotMoveTo(tip string) string {
	if l.M.Branch == refHEAD && l.M.Base != "" {
		// A base that git cannot check, such as a pruned commit, is reported as stale.
		if ok, err := l.r.IsAncestor(l.M.Base, tip); err != nil || !ok {
			return "the detached HEAD moved to a commit that does not contain this stage's base"
		}
	}
	if tip == "" {
		return fmt.Sprintf("branch %s no longer exists", l.M.Branch)
	}
	return ""
}

// moveBase replays the staged changes onto tip and makes tip the base. When
// the replay is not safe, f reports the stage as stale.
func (l *Stage) moveBase(f Freshness, tip string) (Freshness, error) {
	staged, err := l.r.WriteTree(l.IndexPath())
	if err != nil {
		return f, err
	}
	baseTree, err := l.r.TreeOf(l.M.Base)
	if err != nil {
		return f, err
	}
	if staged == baseTree {
		if err := l.setIndex(tip); err != nil {
			return f, err
		}
		f.Base = tip
		return f, nil
	}
	merged, err := l.r.integrate(l.M.Base, baseTree, staged, tip, l.M.Moves, nil)
	e := &Error{}
	if errors.As(err, &e) {
		f.Stale = true
		f.Conflicts = e.Paths
		if e.Kind == "path-deleted" {
			f.Conflicts = e.Details
		}
		f.Reason = e.Message
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := l.setIndexTree(merged, tip); err != nil {
		return f, err
	}
	f.Base = tip
	return f, nil
}

// describeTarget names a stage target for messages.
func describeTarget(ref string) string {
	if ref == refHEAD {
		return "a detached HEAD"
	}
	return "on " + ref
}
