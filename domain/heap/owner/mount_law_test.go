package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/domain/heap"
)

// stubInputs is a stand-in for the composition's Link input record. This
// package states its need as an interface, so a stub record proves the mount's
// own admission without a composition present.
type stubInputs struct {
	source *link.Link
	rows   []axis.MountedArtifact
	heaps  heap.Schema
}

func (inputs stubInputs) LinkSource() *link.Link { return inputs.source }

func (inputs stubInputs) MountedArtifactCount() int { return len(inputs.rows) }

func (inputs stubInputs) MountedArtifactAt(index int) (axis.MountedArtifact, bool) {
	if index < 0 || index >= len(inputs.rows) {
		return axis.MountedArtifact{}, false
	}
	row := inputs.rows[index]
	return row, row.Available()
}

func (inputs stubInputs) HeapInput() heap.Schema { return inputs.heaps }

// TestHeapMountRejectsAnAbsentArtifactView states the mount's admission: the
// heap family is derived from the mounted artifacts, so an input that carries
// none is rejected with this domain's own source evidence rather than sealed as
// a family over nothing.
func TestHeapMountRejectsAnAbsentArtifactView(t *testing.T) {
	schema, failure, ok := mountHeapSchema[stubInputs](stubInputs{})
	if ok || schema.Valid() {
		t.Fatalf("heap mount sealed a family with no Link and no artifacts")
	}
	if failure != heap.SealFailureSource {
		t.Fatalf("heap mount rejected with %v, want the domain's own source evidence", failure)
	}
}

// TestHeapMountRejectsAnUnavailableArtifactRow states that the neutral view is
// checked at this domain's own boundary: a row that carries no artifact never
// reaches the seal.
func TestHeapMountRejectsAnUnavailableArtifactRow(t *testing.T) {
	inputs := stubInputs{rows: []axis.MountedArtifact{{}}}
	schema, failure, ok := mountHeapSchema[stubInputs](inputs)
	if ok || schema.Valid() {
		t.Fatalf("heap mount sealed a family from an unavailable artifact row")
	}
	if failure != heap.SealFailureProgramAllocations {
		t.Fatalf("heap mount rejected with %v, want the domain's own mount evidence", failure)
	}
}

// TestHeapAxisDeclaresItsOwnMount is this domain's ownership receipt: the heap
// axis seals its own Link authority, so no composition root constructs a heap
// mount row on its behalf.
func TestHeapAxisDeclaresItsOwnMount(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("heap axis declaration rejected")
	}
	if !entry.MountDeclared() {
		t.Fatalf("heap axis declares no mount")
	}
	authority, rejection, mounted := entry.Mount(stubInputs{})
	if mounted || authority.Available() {
		t.Fatalf("heap mount admitted an empty artifact view")
	}
	failure, failureOK := axis.Payload[heap.SealFailure](rejection)
	if !failureOK || failure != heap.SealFailureSource {
		t.Fatalf("heap mount lost its own rejection evidence: ok=%v failure=%v", failureOK, failure)
	}
}
