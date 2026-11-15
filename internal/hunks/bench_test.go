package hunks

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// BenchmarkApply applies three one-line hunks to a file of 2,000 lines.
func BenchmarkApply(b *testing.B) {
	var sb strings.Builder
	for i := range 2000 {
		fmt.Fprintf(&sb, "line %d of the file\n", i+1)
	}
	old := []byte(sb.String())
	src := SplitLines(old)
	var hs []Hunk
	for _, n := range []int{100, 1000, 1900} {
		hs = append(hs, Hunk{
			ID: strconv.Itoa(n), OldStart: n, OldLines: 1, NewStart: n, NewLines: 1,
			Removed: []string{src[n-1]}, Added: []string{"changed\n"},
		})
	}
	b.ReportAllocs()
	for range b.N {
		if _, err := Apply(old, hs); err != nil {
			b.Fatal(err)
		}
	}
}
