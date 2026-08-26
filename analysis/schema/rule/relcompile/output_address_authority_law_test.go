package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
)

// OutputBinding.Result is the writer axis's key carrier. The evidence Factor
// stores Evidence values, but its rows remain keyed by the shared Placement
// coordinate; using the fact carrier here makes a canonical route destination
// look foreign to outputAddress.
func TestSuspensionEvidenceOutputUsesItsFactorKeyCarrier(t *testing.T) {
	surfaces := newOwners(t)
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "placement-suspension-evidence"}
	got := surfaces.axisResult(axis)
	if got != placementsuspension.PlacementKeyCarrier {
		t.Fatalf("evidence output carrier=%q, want Factor key carrier %q", got, placementsuspension.PlacementKeyCarrier)
	}
	if got == placementsuspension.EvidenceFactCarrier {
		t.Fatal("evidence output binding used its writer fact carrier as the row key")
	}
}
