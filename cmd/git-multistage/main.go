// Command git-multistage keeps several independent staging areas in one Git
// checkout, each of which commits to the current branch on its own. Git runs
// it as `git multistage`.
package main

import (
	"os"

	"github.com/chrisfls/git-multistage/internal/ms"
)

func main() {
	os.Exit(ms.Main(os.Args[1:], os.Stdout, os.Stderr))
}
