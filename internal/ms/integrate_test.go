package ms

import "testing"

func TestMoveTrailerRoundTrip(t *testing.T) {
	tests := []struct {
		name, old, new, want string
	}{
		{"plain paths", "x.go", "d/y.go", "x.go -> d/y.go"},
		{"arrow in the old path", "p -> q.go", "r.go", `"p -> q.go" -> r.go`},
		{"arrow in the new path", "r.go", "p -> q.go", `r.go -> "p -> q.go"`},
		{"quote and backslash", `a"b\c.go`, "d.go", `"a\"b\\c.go" -> d.go`},
		{"control characters", "a\tb\x01.go", "c.go", `"a\tb\001.go" -> c.go`},
		{"non-ASCII stays", "café.go", "d.go", "café.go -> d.go"},
		{"old path ending in an arrow", "p.go ->", "r.go", `"p.go ->" -> r.go`},
		{"new path starting with an arrow", "r.go", "-> p.go", `r.go -> "-> p.go"`},
		{"surrounding spaces", " p.go", "r.go ", `" p.go" -> "r.go "`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := moveTrailer(Move{Old: tt.old, New: tt.new})
			if got != tt.want {
				t.Fatalf("moveTrailer = %q, want %q", got, tt.want)
			}
			oldp, newp, ok := parseMoveTrailer(got)
			if !ok || oldp != tt.old || newp != tt.new {
				t.Fatalf("parseMoveTrailer(%q) = %q, %q, %v", got, oldp, newp, ok)
			}
		})
	}
	for _, bad := range []string{"x.go", `"x.go -> y.go`, `"x.go" y.go`, " -> y.go", `x.go -> "y.go" z`} {
		if _, _, ok := parseMoveTrailer(bad); ok {
			t.Errorf("parseMoveTrailer(%q) accepted it", bad)
		}
	}
}
