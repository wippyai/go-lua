// Command cutover-manifest mechanizes the cutover landing manifest FT-25's
// hand-written Store manifest (journal seq 6299) established as the shape a
// family cutover lands against: the declared candidate/join/fold surface,
// the canonical relation/projection/reducer/carrier keys, the legacy files
// that are pure protocol residue, the visible mismatches a read-only pass
// can settle, and the required-laws checklist.
//
// It is read-only: it never writes to the repository, only to stdout.
//
//	go run ./cmd/cutover-manifest -pkg domain/placement/store
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/internal/cutovermanifest/render"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("cutover-manifest", flag.ContinueOnError)
	pkg := fs.String("pkg", "", "domain package to inventory, repository-relative, e.g. domain/placement/store (required)")
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pkg == "" {
		fmt.Fprintln(stderr, "cutover-manifest: -pkg is required")
		fs.Usage()
		return 2
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "cutover-manifest: %v\n", err)
		return 2
	}

	manifest, err := render.RenderPackage(repoRoot, *pkg)
	if err != nil {
		fmt.Fprintf(stderr, "cutover-manifest: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, manifest)
	return 0
}
