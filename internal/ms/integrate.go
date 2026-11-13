package ms

import (
	"errors"
	"fmt"

	"github.com/chrisfls/git-multistage/internal/gitx"
)

// integrate replays the change from baseTree to staged onto the tree of tip.
// It returns the merged tree, or an *Error with ExitConflict when the change
// cannot be replayed safely.
func (r *Repo) integrate(baseTree, staged, tip string) (string, error) {
	tipTree, err := r.TreeOf(tip)
	if err != nil {
		return "", err
	}
	if tipTree == baseTree {
		return staged, nil
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
	if l.M.Branch == "HEAD" && l.M.Base != "" {
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
	merged, err := l.r.integrate(baseTree, staged, tip)
	e := &Error{}
	if errors.As(err, &e) {
		f.Stale = true
		f.Conflicts = e.Paths
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
	if ref == "HEAD" {
		return "a detached HEAD"
	}
	return "on " + ref
}
