package typestate

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
)

// TestProtocolDeclarationsAreDerivedAtTypestateBoundary proves the protocol
// owner cut: Link supplies only origins/application coordinates while
// Typestate derives Contract declarations and stable operand identities.
func TestProtocolDeclarationsAreDerivedAtTypestateBoundary(t *testing.T) {
	source := typestateProtocolSource(t)
	schema, ok := NewSchema(source)
	if !ok {
		t.Fatal("schema")
	}
	var acquisitionOrigin, formalOrigin ResourceSource
	formalOrigins := make([]ResourceSource, 0)
	for index := 0; index < schema.SourceCount(); index++ {
		origin, ok := schema.SourceAt(index)
		if !ok {
			t.Fatal("source")
		}
		if origin.kind == resourceSourceAcquisition {
			acquisitionOrigin = origin
		}
		if origin.kind == resourceSourceInput {
			formalOrigin = origin
			formalOrigins = append(formalOrigins, origin)
		}
	}
	if acquisitionOrigin == (ResourceSource{}) || formalOrigin == (ResourceSource{}) {
		t.Fatal("fixture lacks acquisition/input sources")
	}
	first, ok := schema.AcquisitionForSource(acquisitionOrigin, materialization.Exact)
	if !ok || !first.ContentID().Available() || !schema.ValidAcquisition(first) {
		t.Fatal("acquisition declaration")
	}
	second, ok := schema.AcquisitionForSource(acquisitionOrigin, materialization.Exact)
	if !ok || second.ContentID() != first.ContentID() || second.Key() != first.Key() {
		t.Fatal("acquisition derivation is not canonical")
	}
	if _, ok := schema.AcquisitionForSource(formalOrigin, materialization.Exact); ok {
		t.Fatal("input source admitted as acquisition")
	}
	var transition Transition
	foundTransition := false
	for _, origin := range formalOrigins {
		candidate, derived := schema.TransitionForSource(origin, 0, 0, materialization.Exact)
		if derived {
			formalOrigin, transition, foundTransition = origin, candidate, true
			break
		}
	}
	if !foundTransition || !transition.ContentID().Available() || !schema.ValidTransition(transition) {
		t.Fatal("transition declaration")
	}
	again, ok := schema.TransitionForSource(formalOrigin, 0, 0, materialization.Exact)
	if !ok || again.ContentID() != transition.ContentID() || again.Key() != transition.Key() {
		t.Fatal("transition derivation is not canonical")
	}
	if _, ok := schema.TransitionForSource(acquisitionOrigin, 0, 0, materialization.Exact); ok {
		t.Fatal("acquisition source admitted as transition input")
	}
	if _, ok := schema.TransitionForSource(formalOrigin, 1, 0, materialization.Exact); ok {
		t.Fatal("undeclared transition row admitted")
	}
}

func TestSchemaRebindReconstructsCanonicalUniverseWithoutApplicationInverse(t *testing.T) {
	left := typestateProtocolSource(t)
	right := typestateProtocolSource(t)
	if left == right || left.ContentID() != right.ContentID() {
		t.Fatal("fixture replay did not produce distinct equivalent Links")
	}
	schema, ok := NewSchema(left)
	if !ok {
		t.Fatal("left Schema unavailable")
	}
	rebound, ok := schema.Rebind(right)
	if !ok || !rebound.Valid() {
		t.Fatal("Schema replay rebind failed")
	}
	if rebound.ContentID() != schema.ContentID() || rebound.universe == schema.universe || rebound.universe.source != right {
		t.Fatal("Schema rebind did not reconstruct one equivalent canonical universe")
	}
}
