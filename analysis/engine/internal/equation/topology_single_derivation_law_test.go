package equation

import "testing"

// A sealed TopologySpec has one row population. Points, the capability
// catalog, and the canonical rule instances are what every later phase reads,
// so deriving them twice mints the same identities twice and makes the cost of
// sealing a program a multiple of its size for no distinction. This law states
// the count directly: one seal, one derivation.
//
// The count is the proof rather than the absence of a call, because a second
// derivation reintroduced anywhere behind the seal - in the compiler, in
// activation construction, in a future phase inserted between them - is the
// same defect and must fail the same law.
func TestTopologySealDerivesItsRowPopulationExactlyOnce(t *testing.T) {
	fixture := newActivationRowFixtureWithGrammar(t, true, false)
	surface := Surface{Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1,
		Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}
	spec := TopologySpec{
		Batch:     fixture.actuals,
		Points:    []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
		Queries:   []QueryInstance{{Context: boundaryContext(12), Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{surface}}},
		Summaries: []SummaryMapping{{Surface: surface, Keys: []uint64{1, 3}}},
	}

	before := topologyRowDerivations.Load()
	topology, sealed := SealTopology(fixture.source, spec)
	derivations := topologyRowDerivations.Load() - before
	if !sealed || topology == nil {
		t.Fatal("topology seal")
	}
	if derivations != 1 {
		t.Fatalf("one seal derived its row population %d times, want 1", derivations)
	}
}

// The one derivation still owes the whole-spec catalog proof. A mapping no
// Rule or Query surface demands is foreign topology state, and the compiler
// refuses the catalog it is handed rather than trusting that the seal built
// it. Sharing one derivation must not turn that proof into an assumption.
func TestTopologySealRefusesACatalogMappingNoSurfaceDemands(t *testing.T) {
	fixture := newActivationRowFixtureWithGrammar(t, true, false)
	demanded := Surface{Factor: boundaryKey(201), Form: SurfaceReadSummary, Local: 1,
		Semantic: boundaryKey(218), Normalizer: boundaryKey(218)}
	latent := demanded
	latent.Local = 2
	topology, sealed := SealTopology(fixture.source, TopologySpec{
		Batch:   fixture.actuals,
		Points:  []PointSpec{{Site: fixture.actualInput}, {Site: fixture.actualOutput}},
		Queries: []QueryInstance{{Context: boundaryContext(12), Family: fixture.query, Point: PointAt(0), Surfaces: []Surface{demanded}}},
		Summaries: []SummaryMapping{
			{Surface: demanded, Keys: []uint64{1, 3}},
			{Surface: latent, Keys: []uint64{2}},
		},
	})
	if sealed || topology != nil {
		t.Fatal("a catalog mapping no surface demands was sealed into the topology")
	}
}
