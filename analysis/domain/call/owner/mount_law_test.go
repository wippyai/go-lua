package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// stubInputs is a stand-in for the composition's Link input record. This
// package states its need as an interface, so a stub record proves the mount's
// own admission without a composition present.
type stubInputs struct {
	source *link.Link
	rows   []axis.MountedArtifact
	calls  *call.Algebra
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

func (inputs stubInputs) CallInput() *call.Algebra { return inputs.calls }

// TestCallMountRejectsAnAbsentArtifactView states the mount's admission: the
// call algebra's target rows are derived from the mounted artifacts, so an
// input that carries none is rejected with this domain's own input evidence
// rather than sealed as an algebra over nothing.
func TestCallMountRejectsAnAbsentArtifactView(t *testing.T) {
	algebra, rejection, ok := mountCallAlgebra[stubInputs](stubInputs{})
	if ok || algebra != nil {
		t.Fatalf("call mount sealed an algebra with no Link and no artifacts")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("call mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestCallMountRejectsAnUnavailableArtifactRow states that the neutral view is
// checked at this domain's own boundary: a row that carries no artifact never
// reaches the seal.
func TestCallMountRejectsAnUnavailableArtifactRow(t *testing.T) {
	inputs := stubInputs{rows: []axis.MountedArtifact{{}}}
	algebra, rejection, ok := mountCallAlgebra[stubInputs](inputs)
	if ok || algebra != nil {
		t.Fatalf("call mount sealed an algebra from an unavailable artifact row")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("call mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestCallAxisDeclaresItsOwnMount is this domain's ownership receipt: the call
// axis seals its own Link authority, so no composition root constructs a call
// mount row on its behalf.
func TestCallAxisDeclaresItsOwnMount(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("call axis declaration rejected")
	}
	if !entry.MountDeclared() {
		t.Fatalf("call axis declares no mount")
	}
	authority, rejection, mounted := entry.Mount(stubInputs{})
	if mounted || authority.Available() {
		t.Fatalf("call mount admitted an empty artifact view")
	}
	failure, failureOK := axis.Payload[MountRejection](rejection)
	if !failureOK || failure != MountRejectionInput {
		t.Fatalf("call mount lost its own rejection evidence: ok=%v failure=%v", failureOK, failure)
	}
}
