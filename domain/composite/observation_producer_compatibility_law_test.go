package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/query"
)

// TestObservationProducerConsumesTheOwnerEnvelope states the composite
// boundary in both directions: a producer's own population lane remains an
// owner-side fact rather than the diagnostic observation population, while a
// drifted owner codec or producer lookup cannot derive observation subjects.
func TestObservationProducerConsumesTheOwnerEnvelope(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	if axes, axesOK := ProducedValueAxes(compilation); !axesOK || len(axes) == 0 {
		t.Fatal("complete observation producer inventory did not expose value axes")
	}
	state := compilation.catalog
	registration, registrationOK := queryRegistrationFor(state, QueryFamilyValueSummary)
	if !registrationOK || registration == nil {
		t.Fatal("value-summary producer registration unavailable")
	}

	envelope, envelopeOK := registration.ProducerEnvelope()
	if !envelopeOK || envelope.Population != query.PopulationKindSelectedPoint {
		t.Fatalf("sealed producer did not issue its execution-lane envelope: %#v", envelope)
	}

	issued := observationIssuance(state)
	if len(issued) == 0 {
		t.Fatal("sealed observation inventory unavailable")
	}
	for name, damage := range map[string]func(*IssuedObservation){
		"codec role drift": func(observation *IssuedObservation) {
			observation.Codec = "semantic/query-result/effect-exact"
		},
		"producer lookup drift": func(observation *IssuedObservation) {
			observation.Producer = "query/missing"
		},
	} {
		broken := issued[0]
		damage(&broken)
		if _, _, producerOK := observationProducerRegistration(state, broken); producerOK {
			t.Fatalf("observation accepted nearest %s drift", name)
		}
	}
}
