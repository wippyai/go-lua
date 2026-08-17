package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/value"
)

// stubInputs is a stand-in for the composition's Link input record. This
// package states its need as an interface, so a stub record proves the mount's
// own admission without a composition present.
type stubInputs struct {
	source *link.Link
	rows   []axis.MountedArtifact
	heaps  heap.Schema
	values *value.Schema
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

func (inputs stubInputs) ValueInput() *value.Schema { return inputs.values }

// TestValueMountRejectsAnAbsentArtifactView states the mount's admission: the
// value universe is derived from the mounted artifacts, so an input that
// carries none is rejected with this domain's own input evidence rather than
// sealed as a schema over nothing.
func TestValueMountRejectsAnAbsentArtifactView(t *testing.T) {
	schema, failure, ok := mountValueSchema[stubInputs](stubInputs{})
	if ok || schema != nil {
		t.Fatalf("value mount sealed a schema with no Link and no artifacts")
	}
	if failure != value.SealFailureInput {
		t.Fatalf("value mount rejected with %v, want the domain's own input evidence", failure)
	}
}

// TestValueMountRejectsAnUnavailableArtifactRow states that the neutral view is
// checked at this domain's own boundary: a row that carries no artifact never
// reaches the seal.
func TestValueMountRejectsAnUnavailableArtifactRow(t *testing.T) {
	inputs := stubInputs{rows: []axis.MountedArtifact{{}}}
	schema, failure, ok := mountValueSchema[stubInputs](inputs)
	if ok || schema != nil {
		t.Fatalf("value mount sealed a schema from an unavailable artifact row")
	}
	if failure != value.SealFailureInput {
		t.Fatalf("value mount rejected with %v, want the domain's own input evidence", failure)
	}
}

// TestValueAxisDeclaresItsOwnMount is the pilot's ownership receipt: the value
// axis seals its own Link authority, so no composition root constructs a value
// mount row on its behalf.
func TestValueAxisDeclaresItsOwnMount(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("value axis declaration rejected")
	}
	if !entry.MountDeclared() {
		t.Fatalf("value axis declares no mount")
	}
	authority, rejection, mounted := entry.Mount(stubInputs{})
	if mounted || authority.Available() {
		t.Fatalf("value mount admitted an empty artifact view")
	}
	failure, failureOK := axis.Payload[value.SealFailure](rejection)
	if !failureOK || failure != value.SealFailureInput {
		t.Fatalf("value mount lost its own rejection evidence: ok=%v failure=%v", failureOK, failure)
	}
}

// TestValueAxisDeclaresItsHeapEdge states the dependency half of the same
// ownership: the value universe is sealed over the heap family, so the axis
// declares that edge and the mount phase supplies the heap authority because of
// it rather than because of a hand-kept order.
func TestValueAxisDeclaresItsHeapEdge(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("value axis declaration rejected")
	}
	declared := false
	for index := 0; index < entry.DependencyCount(); index++ {
		dependency, dependencyOK := entry.DependencyAt(index)
		if dependencyOK && dependency == "heap" {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("value axis seals over the heap family with no declared edge to it")
	}
}
