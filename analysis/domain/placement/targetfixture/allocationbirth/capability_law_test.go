package allocationbirth

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// TestDeclaresItsTypePolicies keeps the real target-runtime specimen honest:
// relation coordinates carry semantic-key equality, candidate/value lanes
// carry exact codecs, and Placement alone ascends.
func TestDeclaresItsTypePolicies(t *testing.T) {
	identity := targetfixture.NewIdentity(t, fixtureDomain)
	rowContent := content(t, identity, "law/row")
	ids := newIDs(t, rowContent)
	_, _, _, declaration, _, _, _ := newDeclaration(t, ids)
	policies := make(map[model.TypeID]model.TypeCapability)
	for _, capability := range declaration.TypeCapabilities {
		policies[capability.Type()] = capability
	}
	coordinate, ok := policies[ids.coordinateType]
	if !ok || !coordinate.Equatable() || coordinate.Ascending() {
		t.Fatalf("coordinate capability = %v/%t, want Equatable/true", coordinate.Kind(), ok)
	}
	for _, typeID := range []model.TypeID{ids.candidateType, ids.valueType} {
		capability, ok := policies[typeID]
		if !ok || !capability.DecodeOnly() {
			t.Fatalf("opaque type capability %v = %v/%t, want DecodeOnly/true", typeID, capability.Kind(), ok)
		}
	}
	placement, ok := policies[ids.placementType]
	if !ok || !placement.Ascending() {
		t.Fatalf("Placement fact capability = %v/%t, want Ascending/true", placement.Kind(), ok)
	}
	schema, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("allocationbirth compile: %v", err)
	}
	certificateValue, refusal := certificate.Check(schema)
	if refusal != nil || !certificateValue.Available() {
		t.Fatalf("allocationbirth certificate: %v", refusal)
	}
	requirements := certificateValue.AlgebraRequirements()
	if len(requirements) != 1 || requirements[0] != ids.placementType {
		t.Fatalf("allocationbirth algebra requirements = %+v, want Placement fact only", requirements)
	}
}
