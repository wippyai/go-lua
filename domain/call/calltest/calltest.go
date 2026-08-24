// Package calltest is the shared fixture helper for sealing one Link's Call
// algebra. Domain tests that seal a factor over Call state that peer authority
// the same way the composition mounts it instead of restating the mounted
// artifact conversion in every fixture.
package calltest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call"
)

// MustSeal seals the Call algebra for one Link over the mounted artifacts a
// fixture places.
func MustSeal(t testing.TB, source *link.Link, mounts []programmount.MountedArtifact) *call.Algebra {
	t.Helper()
	rows := make([]call.MountedArtifact, 0, len(mounts))
	for _, mount := range mounts {
		rows = append(rows, call.MountedArtifact{Program: mount.Program, Snapshot: mount.Snapshot})
	}
	algebra, sealed := call.NewWithMountedArtifacts(source, rows)
	if !sealed || algebra == nil || !algebra.Valid() {
		t.Fatal("seal Call algebra")
	}
	return algebra
}
