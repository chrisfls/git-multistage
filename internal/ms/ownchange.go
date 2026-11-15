package ms

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// The editor hooks record the change that the tool call itself made: the
// file as the tool read it and as the tool wrote it. Another writer's change
// to the file during the call stays out of the record, without a lock.

// ownChange returns the before and after blobs of the tool call's own
// change, from the tool input and response. ok is false when they do not
// tell the change, and the hook then records the snapshots of the file.
func (h *editHook) ownChange(p pendingEdit) (before, after string, ok bool) {
	var orig, written *string
	switch {
	case h.in.patch != "":
		orig, written = h.codexChange(p)
	case h.in.editor == agentClaude: // another harness's tool of the same name may write otherwise
		orig, written = h.claudeChange()
	}
	if written == nil {
		return "", "", false
	}
	var err error
	before = p.Before
	if orig != nil && *orig != "" {
		if before, err = h.r.cleanContent(h.indexFile(), h.path, []byte(*orig)); err != nil {
			return "", "", false
		}
	}
	if after, err = h.r.cleanContent(h.indexFile(), h.path, []byte(*written)); err != nil {
		return "", "", false
	}
	return before, after, true
}

// claudeChange returns the file as a Claude Code Edit, MultiEdit, or Write
// read it, when its response holds it, and as the tool wrote it.
func (h *editHook) claudeChange() (orig, written *string) {
	var resp claudeResponse
	if json.Unmarshal(h.in.Response, &resp) == nil {
		orig = resp.OriginalFile
		if orig == nil {
			orig = resp.OriginalFileContents
		}
	}
	switch h.in.Tool {
	case "Write":
		return orig, h.in.Input.Content
	case "Edit", "MultiEdit":
		if orig == nil {
			return nil, nil
		}
		edits := h.in.Input.Edits
		if h.in.Tool == "Edit" {
			e := h.in.Input.claudeEdit
			if resp.OldString != nil && resp.NewString != nil {
				e.OldString, e.NewString = *resp.OldString, *resp.NewString
			}
			edits = []claudeEdit{e}
		}
		s, applied := replaceAll(*orig, edits)
		if !applied {
			return nil, nil
		}
		return orig, &s
	}
	return nil, nil
}

// replaceAll applies the string replacements of an Edit or MultiEdit to s,
// as the tool does. It reports false when a replaced string is missing.
func replaceAll(s string, edits []claudeEdit) (string, bool) {
	if len(edits) == 0 {
		return "", false
	}
	for _, e := range edits {
		if e.OldString == "" || !strings.Contains(s, e.OldString) {
			return "", false
		}
		n := 1
		if e.ReplaceAll {
			n = -1
		}
		s = strings.Replace(s, e.OldString, e.NewString, n)
	}
	return s, true
}

// codexChange applies the apply_patch section of the hook's file to the
// file's bytes before the call. It returns nil when the section is not an
// update or an addition that applies exactly.
func (h *editHook) codexChange(p pendingEdit) (orig, written *string) {
	sec, ok := codexSection(h.in.patch, h.in.Cwd, h.file)
	if !ok {
		return nil, nil
	}
	switch {
	case sec.kind == "add" && p.Before == "":
		s := joinLines(sec.added)
		return nil, &s
	case sec.kind == codexUpdate && sec.move == "" && p.Raw != "":
		raw, err := h.r.catBlob(p.Raw)
		if err != nil || strings.Contains(string(raw), "\r") {
			return nil, nil
		}
		s := string(raw)
		out, ok := applyCodexHunks(s, sec.hunks)
		if !ok {
			return nil, nil
		}
		return &s, &out
	}
	return nil, nil
}

// codexUpdate is the kind of an Update File section.
const codexUpdate = "update"

type codexHunk struct {
	context  string
	old, new []string
}

type codexPatchSection struct {
	kind, move string
	added      []string
	hunks      []codexHunk
}

// codexSection returns the section of patch that names file.
func codexSection(patch, cwd, file string) (codexPatchSection, bool) {
	var sec *codexPatchSection
	var found codexPatchSection
	matched := false
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		if kind, p, ok := codexHeader(line); ok {
			if sec != nil && matched {
				found = *sec
			}
			sec = &codexPatchSection{kind: kind}
			if !filepath.IsAbs(p) {
				p = filepath.Join(cwd, p)
			}
			matched = filepath.Clean(p) == filepath.Clean(file)
			continue
		}
		if sec != nil {
			sec.addLine(line)
		}
	}
	if sec != nil && matched {
		found = *sec
	}
	return found, found.kind != ""
}

// codexHeaders are the headers that start a file section of a patch, with
// the kind of each section. No line starts with two of them.
var codexHeaders = [...]struct{ prefix, kind string }{
	{"*** Add File: ", "add"},
	{"*** Update File: ", codexUpdate},
	{"*** Delete File: ", "delete"},
}

// codexHeader parses a line that starts a file section of a patch.
func codexHeader(line string) (kind, path string, ok bool) {
	for _, h := range codexHeaders {
		if p, ok := strings.CutPrefix(line, h.prefix); ok {
			return h.kind, strings.TrimSpace(p), true
		}
	}
	return "", "", false
}

// addLine adds one patch line that follows the section's header.
func (sec *codexPatchSection) addLine(line string) {
	switch {
	case line == "*** End Patch", line == "*** End of File":
	case strings.HasPrefix(line, "*** Move to: "):
		sec.move = strings.TrimSpace(strings.TrimPrefix(line, "*** Move to: "))
	case sec.kind == "add":
		if l, ok := strings.CutPrefix(line, "+"); ok {
			sec.added = append(sec.added, l)
		}
	case sec.kind != codexUpdate:
	case strings.HasPrefix(line, "@@"):
		sec.hunks = append(sec.hunks, codexHunk{context: strings.TrimSpace(strings.TrimPrefix(line, "@@"))})
	default:
		if len(sec.hunks) == 0 {
			sec.hunks = append(sec.hunks, codexHunk{})
		}
		hk := &sec.hunks[len(sec.hunks)-1]
		switch {
		case strings.HasPrefix(line, "+"):
			hk.new = append(hk.new, line[1:])
		case strings.HasPrefix(line, "-"):
			hk.old = append(hk.old, line[1:])
		default:
			l := strings.TrimPrefix(line, " ")
			hk.old, hk.new = append(hk.old, l), append(hk.new, l)
		}
	}
}

// applyCodexHunks applies hunks to content, each after the previous one,
// matching their lines exactly. It reports false when a hunk does not match.
func applyCodexHunks(content string, hunks []codexHunk) (string, bool) {
	var lines []string
	if content != "" {
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	cursor := 0
	for _, hk := range hunks {
		if hk.context != "" {
			i := indexOf(lines, []string{hk.context}, cursor)
			if i < 0 {
				return "", false
			}
			cursor = i + 1
		}
		at := len(lines)
		if len(hk.old) > 0 {
			if at = indexOf(lines, hk.old, cursor); at < 0 {
				return "", false
			}
		}
		lines = append(lines[:at:at], append(append([]string{}, hk.new...), lines[at+len(hk.old):]...)...)
		cursor = at + len(hk.new)
	}
	return joinLines(lines), true
}

// indexOf returns the first index at or after from where seq starts in lines.
func indexOf(lines, seq []string, from int) int {
	for i := from; i+len(seq) <= len(lines); i++ {
		match := true
		for j := range seq {
			if lines[i+j] != seq[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
