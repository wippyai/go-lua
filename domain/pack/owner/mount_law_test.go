package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/static"
)

// stubInputs is a stand-in for the composition's Link input record. This
// package states its need as an interface, so a stub record proves the mount's
// own admission without a composition present.
type stubInputs struct {
	source  *link.Link
	rows    []axis.MountedArtifact
	statics *static.Authority
	packs   *pack.Schema
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

func (inputs stubInputs) StaticInput() *static.Authority { return inputs.statics }

func (inputs stubInputs) PackInput() *pack.Schema { return inputs.packs }

// TestPackMountRejectsAnAbsentArtifactView states the mount's admission: the
// pack universe is derived from the mounted artifacts, so an input that carries
// none is rejected with this domain's own input evidence rather than sealed as
// a schema over nothing.
func TestPackMountRejectsAnAbsentArtifactView(t *testing.T) {
	schema, rejection, ok := mountPackSchema[stubInputs](stubInputs{})
	if ok || schema != nil {
		t.Fatalf("pack mount sealed a schema with no Link and no artifacts")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("pack mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestPackMountRejectsAnAbsentStaticAuthority states that the static inventory
// is one of this mount's own inputs: the pack seal reads its mounted value
// substitutions from it, so a record without it never opens the seal.
func TestPackMountRejectsAnAbsentStaticAuthority(t *testing.T) {
	inputs := stubInputs{rows: []axis.MountedArtifact{{}}}
	schema, rejection, ok := mountPackSchema[stubInputs](inputs)
	if ok || schema != nil {
		t.Fatalf("pack mount sealed a schema with no static authority")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("pack mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestPackAxisDeclaresItsOwnMount is this domain's ownership receipt: the pack
// axis seals its own Link authority, so no composition root constructs a pack
// mount row on its behalf.
func TestPackAxisDeclaresItsOwnMount(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("pack axis declaration rejected")
	}
	if !entry.MountDeclared() {
		t.Fatalf("pack axis declares no mount")
	}
	authority, rejection, mounted := entry.Mount(stubInputs{})
	if mounted || authority.Available() {
		t.Fatalf("pack mount admitted an empty artifact view")
	}
	failure, failureOK := axis.Payload[MountRejection](rejection)
	if !failureOK || failure != MountRejectionInput {
		t.Fatalf("pack mount lost its own rejection evidence: ok=%v failure=%v", failureOK, failure)
	}
}
