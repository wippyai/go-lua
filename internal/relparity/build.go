package relparity

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wippyai/go-lua/internal/cutoververify"
)

// Checkout is the shared clone both sides are built from, one after the other.
// The clone, its reset and its revision resolution are the cutover ritual's,
// reused rather than restated.
type Checkout struct {
	Path string
}

// OpenCheckout returns the cached shared clone of repository under scratchRoot,
// cloning it once if it is not there yet.
func OpenCheckout(scratchRoot, repository string) (Checkout, error) {
	path, err := cutoververify.EnsureClone(scratchRoot, repository)
	if err != nil {
		return Checkout{}, fmt.Errorf("relparity: %w", err)
	}
	return Checkout{Path: path}, nil
}

// BuildSide resets the checkout to ref and builds the probe command out of it
// into binaryDirectory.
//
// The binary is written outside the checkout, so the next side's reset cannot
// disturb it and the two processes under comparison are two files on disk, not
// one file rebuilt.
func BuildSide(checkout Checkout, probe Probe, role, ref, binaryDirectory string) (Side, error) {
	if err := cutoververify.ResetClone(checkout.Path, ref); err != nil {
		return Side{}, fmt.Errorf("relparity: %w", err)
	}
	commit, err := cutoververify.ResolveCommit(checkout.Path, ref)
	if err != nil {
		return Side{}, fmt.Errorf("relparity: %w", err)
	}
	if err := os.MkdirAll(binaryDirectory, 0o755); err != nil {
		return Side{}, fmt.Errorf("relparity: create binary directory %s: %w", binaryDirectory, err)
	}
	binary := filepath.Join(binaryDirectory, role)
	command := exec.Command("go", "build", "-o", binary, probe.Package)
	command.Dir = checkout.Path
	if output, err := command.CombinedOutput(); err != nil {
		return Side{}, fmt.Errorf("relparity: build %s probe %s at %s: %w\n%s",
			role, probe.Package, ref, err, output)
	}
	return Side{Name: role, Ref: ref, Commit: commit, Binary: binary}, nil
}
