package ms

import (
	"fmt"
	"strings"
	"testing"
)

// benchPatch is a Codex patch that updates 20 files with one hunk each.
func benchPatch() string {
	var sb strings.Builder
	sb.WriteString("*** Begin Patch\n")
	for i := range 20 {
		fmt.Fprintf(&sb, "*** Update File: dir/file%d.txt\n@@\n context\n-old line\n+new line\n context\n", i)
	}
	sb.WriteString("*** End Patch\n")
	return sb.String()
}

func BenchmarkPatchFiles(b *testing.B) {
	patch := benchPatch()
	b.ReportAllocs()
	for range b.N {
		if len(patchFiles(patch, "/w")) != 20 {
			b.Fatal("wrong file count")
		}
	}
}

func BenchmarkCodexSection(b *testing.B) {
	patch := benchPatch()
	b.ReportAllocs()
	for range b.N {
		if _, ok := codexSection(patch, "/w", "/w/dir/file10.txt"); !ok {
			b.Fatal("section not found")
		}
	}
}
