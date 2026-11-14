package ms

import (
	"context"
	"math/rand/v2"
	"os/exec"
	"strings"
	"testing"
)

// TestCleanMessage compares cleanMessage with `git stripspace` on chosen
// texts and on random texts of white space, NUL, and letters.
func TestCleanMessage(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"", "\n", "\n\n  \n", "x", "x\n", "  a  \n\n\n b\t\r\n", "a\r\n\r\nb\r\n", "\ta\n",
		"a\v\nb\f\n", "a\x00 \nb", "\n\na\n\n", "a\n \n\t\nb", "é  \n", "a\rb\r",
	}
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // reproducible test inputs need no cryptographic randomness
	const alphabet = " \t\r\n\v\fa\x00"
	for range 200 {
		b := make([]byte, rng.IntN(12))
		for i := range b {
			b[i] = alphabet[rng.IntN(len(alphabet))]
		}
		inputs = append(inputs, string(b))
	}
	for _, in := range inputs {
		cmd := exec.CommandContext(context.Background(), "git", "stripspace")
		cmd.Stdin = strings.NewReader(in)
		want, err := cmd.Output()
		if err != nil {
			t.Fatalf("git stripspace: %v", err)
		}
		if got := cleanMessage(in); got != string(want) {
			t.Fatalf("cleanMessage(%q) = %q; git stripspace gives %q", in, got, want)
		}
	}
}
