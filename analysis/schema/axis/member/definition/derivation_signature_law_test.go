package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// specimenCellView is the execution cell view a many-valued derivation input
// is delivered through, named by the caller the way the sealed disposition is.
func specimenCellView() GoType {
	return GoType{PackagePath: "example/execution", Name: "SelectedCell"}
}

// specimenDerivationRoster admits the specimen source so a derivation can be
// resolved against the roster the way a real one is.
func specimenDerivationRoster(t testing.TB, source Definition) Roster {
	t.Helper()
	withScheduledDeath(t, ScheduledDeath{
		Axis: "specimen", Relation: "specimen/derived",
		Build: GoSymbol{PackagePath: specimenPackage, Name: "DeriveRows"},
	})
	roster, rosterOK := NewRoster(Source{Package: "specimen", Name: "specimen", Base: source})
	if !rosterOK {
		t.Fatal("specimen roster is not admissible")
	}
	return roster
}

// TestADerivationsCallShapeIsAFunctionOfItsDeclaration states fence one at the
// place the shape is derived.
//
// Build answers the derivation's State from the schemas of its ordered static
// axes followed by the relation's declared inputs; Count and At consume that
// State to expose the relation's Subject rows. Every position comes from a row
// somebody declared, so nothing an implementation happens to need can widen
// the call - which is the same property the reducer call shape has, and the
// reason a derivation is admitted as authored code at all.
func TestADerivationsCallShapeIsAFunctionOfItsDeclaration(t *testing.T) {
	source := specimenDerivation(t)
	roster := specimenDerivationRoster(t, source)
	relation := source.Relations[len(source.Relations)-1]
	shape, derived := roster.DerivationSignature("specimen", relation, specimenCellView())
	if !derived {
		t.Fatal("a whole derivation derived no call shape")
	}
	if len(shape.BuildParams) != len(relation.Derivation.StaticAxes)+len(relation.Inputs) {
		t.Fatalf("Build takes %d parameters, want one per static axis and one per input", len(shape.BuildParams))
	}
	if shape.BuildParams[0].Type != specimenType("Schema") {
		t.Fatalf("Build's first parameter is %v, want the static axis's own schema", shape.BuildParams[0])
	}
	if shape.BuildParams[1].Type != specimenType("Seed") {
		t.Fatalf("Build's input parameter is %v, want the declared input carrier", shape.BuildParams[1])
	}
	if len(shape.BuildResults) != 2 || shape.BuildResults[0].Type != specimenType("Plan") || shape.BuildResults[1].Type != (GoType{Name: "bool"}) {
		t.Fatalf("Build answers %v, want the derivation State and its validity", shape.BuildResults)
	}
	if len(shape.CountParams) != 1 || shape.CountParams[0].Type != specimenType("Plan") ||
		len(shape.CountResults) != 1 || shape.CountResults[0].Type != (GoType{Name: "int"}) {
		t.Fatalf("Count is %v -> %v, want the State's census", shape.CountParams, shape.CountResults)
	}
	if len(shape.AtParams) != 2 || shape.AtParams[1].Type != (GoType{Name: "int"}) ||
		len(shape.AtResults) != 2 || shape.AtResults[0].Type != specimenType("Fact") {
		t.Fatalf("At is %v -> %v, want one Subject row of the relation", shape.AtParams, shape.AtResults)
	}
}

// TestADerivedShapeFollowsEveryDeclaredRow proves the derivation is not a
// constant. A row added, removed, or renamed moves the call, which is what
// makes holding an implementation to it a measurement rather than a ritual.
func TestADerivedShapeFollowsEveryDeclaredRow(t *testing.T) {
	base := specimenDerivation(t)
	roster := specimenDerivationRoster(t, base)
	relation := base.Relations[len(base.Relations)-1]
	original, derived := roster.DerivationSignature("specimen", relation, specimenCellView())
	if !derived {
		t.Fatal("baseline shape")
	}

	widened := relation
	widened.Inputs = append(append([]RelationInput(nil), relation.Inputs...), RelationInput{Carrier: "KeyCarrier"})
	shape, derivedOK := roster.DerivationSignature("specimen", widened, specimenCellView())
	if !derivedOK || len(shape.BuildParams) != len(original.BuildParams)+1 {
		t.Fatal("a declared input did not widen the call")
	}

	restated := relation
	restated.Derivation.State = specimenType("OtherPlan")
	shape, derivedOK = roster.DerivationSignature("specimen", restated, specimenCellView())
	if !derivedOK || shape.BuildResults[0].Type != specimenType("OtherPlan") || shape.CountParams[0].Type != specimenType("OtherPlan") {
		t.Fatal("the derivation State is not the type the call threads")
	}

	foreign := relation
	foreign.Derivation.StaticAxes = []schema.EntryReference{{Surface: schema.SurfaceKindAxis, Key: "absent"}}
	if _, derivedOK = roster.DerivationSignature("specimen", foreign, specimenCellView()); derivedOK {
		t.Fatal("a static axis no source owns resolved a schema type")
	}
}

// TestAManyValuedDerivationInputIsTheCellsOfItsJoin states the delivery the
// freeze route relation needs and the one a scalar carrier cannot express.
//
// A derivation over a whole selection - a route set computed from every
// mounted actual - cannot be handed one member at a time without asking it to
// rebuild the correlation the read already established. So the input declares
// that it is many-valued, and the derived parameter is a slice of the
// execution cell view instantiated at that input's own carrier: the cells the
// join delivered, in the order it delivered them, and nothing else.
func TestAManyValuedDerivationInputIsTheCellsOfItsJoin(t *testing.T) {
	source := specimenDerivation(t)
	relation := &source.Relations[len(source.Relations)-1]
	relation.Inputs = []RelationInput{{Carrier: "SeedCarrier"}, {Carrier: "FactCarrier", Many: true}}
	roster := specimenDerivationRoster(t, source)
	shape, derived := roster.DerivationSignature("specimen", *relation, specimenCellView())
	if !derived {
		t.Fatal("a many-valued derivation input derived no call shape")
	}
	scalar := shape.BuildParams[len(shape.BuildParams)-2]
	if scalar.Type != specimenType("Seed") || scalar.Slice || scalar.Element.Available() {
		t.Fatalf("scalar input derived %+v, want the carrier value itself", scalar)
	}
	cells := shape.BuildParams[len(shape.BuildParams)-1]
	if !cells.Slice || cells.Type != specimenCellView() || cells.Element != specimenType("Fact") {
		t.Fatalf("many-valued input derived %+v, want a slice of the cell view at its own carrier", cells)
	}

	// The view is the caller's to name. A derivation that declares a
	// many-valued input and is derived without one has no spelling for the
	// delivery, and inventing one here would be this package choosing an
	// execution type.
	if _, derivedOK := roster.DerivationSignature("specimen", *relation, GoType{}); derivedOK {
		t.Fatal("a many-valued input derived a shape with no cell view")
	}
}
