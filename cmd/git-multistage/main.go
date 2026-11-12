// Command git-multistage lets several agents stage and commit independently
// in one Git checkout. Git runs it as `git multistage`.
package main

import (
	"os"

	"github.com/chrisfls/git-multistage/internal/ms"
)

func main() {
	os.Exit(ms.Main(os.Args[1:], os.Stdout, os.Stderr))
}
