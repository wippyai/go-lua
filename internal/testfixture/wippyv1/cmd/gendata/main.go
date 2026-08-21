// Command gendata regenerates the checked-in wire-encoded JSON fixture data
// under testdata/manifests/wippyv1 from the wippyv1 package's Go manifest
// constructors. Run it through `go generate ./internal/testfixture/wippyv1`;
// TestManifestDataIsNotStale fails if a constructor edit is not followed by
// regeneration.
package main

import (
	"fmt"
	"os"

	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
)

func main() {
	repository, err := testfixture.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := wippyv1.GenerateManifestData(repository); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
