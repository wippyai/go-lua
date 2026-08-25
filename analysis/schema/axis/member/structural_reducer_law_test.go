package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// structural_reducer_law_test.go states the one declared exception to "a
// reducer publishes at least one carrier".
//
// A fold's declared outputs are the FACTS it publishes. A structural fold
// publishes no fact at all - its whole result is the disposition of the branch
// it was invoked for, which is what a ReductionOutcome already is - so it
// declares no output carrier. Every other fold keeps its carriers, and the
// exception is stated on the reducer row rather than inferred from an empty
// list, so an ordinary reducer that simply forgot its output is still refused.

func structuralReducerLawAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "structural-reducer-law"}
}

func structuralReducerLawRow(structural bool, outputs int) Reducer {
	reducer := Reducer{
		Key:        "structural-reducer-law/reducer",
		Structural: structural,
		Inputs: []ReducerInput{{
			Axis: structuralReducerLawAxis(), Carrier: "carrier/structural-reducer-law/fact",
			Form: ReadFormExact, Multiplicity: MultiplicityOne,
		}},
	}
	for index := 0; index < outputs; index++ {
		reducer.Outputs = append(reducer.Outputs, ReducerOutput{
			Axis: structuralReducerLawAxis(), Carrier: "carrier/structural-reducer-law/fact",
		})
	}
	return reducer
}

// TestAStructuralFoldDeclaresNoOutputCarrier is the exception itself.
func TestAStructuralFoldDeclaresNoOutputCarrier(t *testing.T) {
	if !structuralReducerLawRow(true, 0).Available() {
		t.Fatal("a structural reducer with no output carrier was refused")
	}
	if structuralReducerLawRow(true, 1).Available() {
		t.Fatal("a structural reducer published an output carrier")
	}
}

// TestAnOrdinaryFoldStillPublishesACarrier keeps the law that was there. The
// exception is declared, so an ordinary reducer that lost its output is
// refused exactly as before rather than silently reading as structural.
func TestAnOrdinaryFoldStillPublishesACarrier(t *testing.T) {
	if structuralReducerLawRow(false, 0).Available() {
		t.Fatal("an ordinary reducer with no output carrier was admitted")
	}
	if !structuralReducerLawRow(false, 1).Available() {
		t.Fatal("an ordinary reducer with one output carrier was refused")
	}
}

// TestAReducerSurvivesTheCatalogWhole is the law that catches a copy which
// drops a row's field. A catalog clones every reducer it seals and hands out a
// fresh copy on every lookup, so a field the copy forgets is a declaration
// that reads back as something its author did not write - and a structural
// reducer that forgets its marker reads as an ordinary one missing its output.
func TestAReducerSurvivesTheCatalogWhole(t *testing.T) {
	catalog, sealed := NewCatalog(
		[]Relation{{
			Key: "structural-reducer-law/candidates", Subject: "carrier/structural-reducer-law/candidate",
			CandidateProvider: AxisRelationCandidate(RelationRef{Axis: structuralReducerLawAxis(), Member: "structural-reducer-law/candidates"}),
		}},
		nil,
		[]Reducer{structuralReducerLawRow(true, 0)},
		nil,
	)
	if !sealed {
		t.Fatal("a catalog refused a structural reducer")
	}
	stored, found := catalog.Reducer("structural-reducer-law/reducer")
	if !found {
		t.Fatal("the sealed reducer is not in the catalog")
	}
	if !stored.Structural {
		t.Fatal("the catalog dropped the reducer's structural marker")
	}
	if !stored.Available() {
		t.Fatal("a reducer read back from the catalog is no longer admissible")
	}
}
