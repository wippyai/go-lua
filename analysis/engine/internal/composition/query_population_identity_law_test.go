package composition

import (
	"testing"

	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

func TestQueryPopulationIsAnAbsoluteCompositionIdentity(t *testing.T) {
	factor, family, freezer := coldKey(241), coldKey(242), coldKey(243)
	base := Candidate{
		Factors: []Factor{{Key: factor}},
		Queries: []QueryFamily{{
			Key: family, Freezer: freezer, Population: queryschema.PopulationKindSelectedPoint,
			Projections: []QueryProjection{{Kind: QueryFactorExact, Factor: factor}},
		}},
	}
	selected, selectedOK := Seal(base)
	if !selectedOK || selected == nil {
		t.Fatal("selected-point query population refused sealing")
	}
	base.Queries[0].Population = queryschema.PopulationKindObservation
	observation, observationOK := Seal(base)
	if !observationOK || observation == nil {
		t.Fatal("observation query population refused sealing")
	}
	if selected.ID() == observation.ID() {
		t.Fatal("selected-point and observation populations alias one CompositionID")
	}
	base.Queries[0].Population = queryschema.PopulationKindInvalid
	if invalid, invalidOK := Seal(base); invalidOK || invalid != nil {
		t.Fatal("invalid query population entered a sealed composition")
	}
}
