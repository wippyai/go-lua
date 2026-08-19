package owner

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/pack"
)

// stubInputs is a stand-in for the composition's Link input record. This
// package states its need as an interface, so a stub record proves the mount's
// own admission without a composition present.
type stubInputs struct {
	source  *link.Link
	rows    []programmount.MountedArtifact
	packs   *pack.Schema
	effects *factor.Algebra
}

func (inputs stubInputs) LinkSource() *link.Link { return inputs.source }

func (inputs stubInputs) MountedArtifactCount() int { return len(inputs.rows) }

func (inputs stubInputs) MountedArtifactAt(index int) (programmount.MountedArtifact, bool) {
	if index < 0 || index >= len(inputs.rows) {
		return programmount.MountedArtifact{}, false
	}
	row := inputs.rows[index]
	return row, row.Available()
}

func (inputs stubInputs) PackInput() *pack.Schema { return inputs.packs }

func (inputs stubInputs) EffectInput() *factor.Algebra { return inputs.effects }

// TestEffectMountRejectsAnAbsentArtifactView states the mount's admission: the
// effect algebra's body rows are derived from the mounted artifacts, so an
// input that carries none is rejected with this domain's own input evidence
// rather than sealed as an algebra over nothing.
func TestEffectMountRejectsAnAbsentArtifactView(t *testing.T) {
	algebra, rejection, ok := mountEffectAlgebra[stubInputs](stubInputs{})
	if ok || algebra != nil {
		t.Fatalf("effect mount sealed an algebra with no Link and no artifacts")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("effect mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestEffectMountRejectsAnAbsentPackAuthority states the declared dependency at
// this domain's own boundary: the algebra is sealed over the pack universe, so
// a record whose pack authority is absent never opens the seal. The mount phase
// supplies that authority only because this axis declared the edge.
func TestEffectMountRejectsAnAbsentPackAuthority(t *testing.T) {
	inputs := stubInputs{rows: []programmount.MountedArtifact{{}}}
	algebra, rejection, ok := mountEffectAlgebra[stubInputs](inputs)
	if ok || algebra != nil {
		t.Fatalf("effect mount sealed an algebra with no pack authority")
	}
	if rejection != MountRejectionInput {
		t.Fatalf("effect mount rejected with %v, want the domain's own input evidence", rejection)
	}
}

// TestEffectAxisDeclaresItsOwnMountAndItsPackEdge is this domain's ownership
// receipt: the effect axis seals its own Link authority and declares the one
// peer that authority is sealed over, so no composition root constructs an
// effect mount row or hand-orders the two seals.
func TestEffectAxisDeclaresItsOwnMountAndItsPackEdge(t *testing.T) {
	entry, ok := axis.New(AxisEntry[stubInputs]())
	if !ok || entry == nil {
		t.Fatalf("effect axis declaration rejected")
	}
	if !entry.MountDeclared() {
		t.Fatalf("effect axis declares no mount")
	}
	declared := false
	for index := 0; index < entry.DependencyCount(); index++ {
		dependency, dependencyOK := entry.DependencyAt(index)
		if dependencyOK && dependency == "pack" {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("effect axis seals over the pack universe with no declared edge to it")
	}
	authority, rejection, mounted := entry.Mount(stubInputs{})
	if mounted || authority.Available() {
		t.Fatalf("effect mount admitted an empty artifact view")
	}
	failure, failureOK := axis.Payload[MountRejection](rejection)
	if !failureOK || failure != MountRejectionInput {
		t.Fatalf("effect mount lost its own rejection evidence: ok=%v failure=%v", failureOK, failure)
	}
}
