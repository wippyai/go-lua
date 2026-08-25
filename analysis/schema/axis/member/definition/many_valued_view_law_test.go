package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// viewSpecimen is the package's own derivation specimen with its one relation
// input made MANY-VALUED under the given read form, and a reducer folding that
// same input beside it. One declared read, two consumers, so the law below can
// ask each of them what view it was handed.
func viewSpecimen(t testing.TB, form member.ReadForm) (Definition, Relation, Reducer) {
	t.Helper()
	source := specimenDerivation(t)
	last := len(source.Relations) - 1
	source.Relations[last].Inputs = []RelationInput{{Carrier: "SeedCarrier", Many: true, Form: form}}
	// The reducer is held beside the base rather than in it: a base that
	// declares its own reducer is refused at composition, because a reducer no
	// rule is on record as folding with is the per-axis list contributions
	// replaced. The call shape needs only this axis's carriers.
	fold := Reducer{
		Name: "ViewReducer", Key: "specimen/reducer/view", Candidate: "SeedCarrier",
		Inputs: []ReducerInput{{
			Axis: specimenAxis(), Carrier: "SeedCarrier",
			Form: form, Multiplicity: member.MultiplicityMany,
		}},
		Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
		Implementation: GoSymbol{PackagePath: specimenPackage, Name: "ViewFold", ResultIndex: 0},
	}
	return source, source.Relations[last], fold
}

// TestAManyValuedDeliveryIsTheViewItsReadFormEstablishes is the selection law.
//
// A whole-vector read over a closed denominator establishes each cell's
// position and no tag; a selection establishes the tag it correlated each cell
// by. So the view is a function of the read's Form, and handing a selection
// the weaker one would ask its consumer to recover a correlation the read
// already proved.
func TestAManyValuedDeliveryIsTheViewItsReadFormEstablishes(t *testing.T) {
	cell, vector := specimenCellView(), specimenVectorView()
	view, slice, ok := ManyValuedView(member.ReadFormSelected, cell, vector)
	if !ok || view != cell || !slice {
		t.Fatalf("a selection is delivered as view=%+v slice=%t ok=%t; it establishes a tag per cell, so it is a slice of the cell view", view, slice, ok)
	}
	view, slice, ok = ManyValuedView(member.ReadFormSummary, cell, vector)
	if !ok || view != vector || slice {
		t.Fatalf("a whole-vector read is delivered as view=%+v slice=%t ok=%t; it establishes no tag, so it is one vector", view, slice, ok)
	}
}

// TestAManyValuedDeliveryRefusesAFormThatEstablishesNoDelivery states the
// other half. An exact read delivers one cell and is not many-valued at all,
// so there is no view for it here rather than a default that would silently
// hand a consumer the wrong shape.
func TestAManyValuedDeliveryRefusesAFormThatEstablishesNoDelivery(t *testing.T) {
	cell, vector := specimenCellView(), specimenVectorView()
	if _, _, ok := ManyValuedView(member.ReadFormExact, cell, vector); ok {
		t.Fatal("an exact read was handed a many-valued view")
	}
	if _, _, ok := ManyValuedView(member.ReadFormSelected, GoType{}, vector); ok {
		t.Fatal("a selection was delivered without the caller naming the cell view")
	}
	if _, _, ok := ManyValuedView(member.ReadFormSummary, cell, GoType{}); ok {
		t.Fatal("a whole-vector read was delivered without the caller naming the vector view")
	}
}

// TestAFoldAndItsDerivationAreHandedTheSameView is the law the asymmetry
// broke, and the reason the selection has one statement.
//
// A relation derived over a many-valued input and a fold that reduces that
// same input consume one delivery. They were derived by two functions that
// each chose a view: DerivationSignature chose by the input's Form, and
// ReducerSignature always chose the vector - so a fold over a SELECTION was
// handed a view with the tags stripped out while the derivation beside it saw
// them. Both now ask ManyValuedView, so the two cannot disagree.
func TestAFoldAndItsDerivationAreHandedTheSameView(t *testing.T) {
	cell, vector := specimenCellView(), specimenVectorView()
	outcome := GoType{PackagePath: "example/structure", Name: "ReductionOutcome"}
	for _, form := range []member.ReadForm{member.ReadFormSelected, member.ReadFormSummary} {
		source, relation, fold := viewSpecimen(t, form)
		roster := specimenDerivationRoster(t, source)
		shape, shapeOK := roster.DerivationSignature("specimen", relation, cell, vector)
		if !shapeOK || len(shape.BuildParams) != 2 {
			t.Fatalf("form %v: derivation shape not derived: %+v", form, shape)
		}
		derived := shape.BuildParams[1]

		arguments, _, argumentsOK := source.ReducerSignature(fold, outcome, cell, vector)
		if !argumentsOK || len(arguments) != 2 {
			t.Fatalf("form %v: reducer shape not derived: %+v", form, arguments)
		}
		folded := arguments[1]
		if folded.Role != ArgumentVector {
			t.Fatalf("form %v: a many-valued input is not one argument: role=%v", form, folded.Role)
		}
		if folded.Type != derived.Type || folded.Element != derived.Element || folded.Slice != derived.Slice {
			t.Fatalf("form %v: the fold is handed %+v and its derivation %+v; one delivery is one view", form, folded, derived)
		}
	}
}
