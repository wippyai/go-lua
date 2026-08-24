package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

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
	shape, derived := roster.DerivationSignature("specimen", relation)
	if !derived {
		t.Fatal("a whole derivation derived no call shape")
	}
	if len(shape.BuildParams) != len(relation.Derivation.StaticAxes)+len(relation.Inputs) {
		t.Fatalf("Build takes %d parameters, want one per static axis and one per input", len(shape.BuildParams))
	}
	if shape.BuildParams[0] != specimenType("Schema") {
		t.Fatalf("Build's first parameter is %v, want the static axis's own schema", shape.BuildParams[0])
	}
	if shape.BuildParams[1] != specimenType("Seed") {
		t.Fatalf("Build's input parameter is %v, want the declared input carrier", shape.BuildParams[1])
	}
	if len(shape.BuildResults) != 2 || shape.BuildResults[0] != specimenType("Plan") || shape.BuildResults[1] != (GoType{Name: "bool"}) {
		t.Fatalf("Build answers %v, want the derivation State and its validity", shape.BuildResults)
	}
	if len(shape.CountParams) != 1 || shape.CountParams[0] != specimenType("Plan") ||
		len(shape.CountResults) != 1 || shape.CountResults[0] != (GoType{Name: "int"}) {
		t.Fatalf("Count is %v -> %v, want the State's census", shape.CountParams, shape.CountResults)
	}
	if len(shape.AtParams) != 2 || shape.AtParams[1] != (GoType{Name: "int"}) ||
		len(shape.AtResults) != 2 || shape.AtResults[0] != specimenType("Fact") {
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
	original, derived := roster.DerivationSignature("specimen", relation)
	if !derived {
		t.Fatal("baseline shape")
	}

	widened := relation
	widened.Inputs = append(append([]string(nil), relation.Inputs...), "KeyCarrier")
	shape, derivedOK := roster.DerivationSignature("specimen", widened)
	if !derivedOK || len(shape.BuildParams) != len(original.BuildParams)+1 {
		t.Fatal("a declared input did not widen the call")
	}

	restated := relation
	restated.Derivation.State = specimenType("OtherPlan")
	shape, derivedOK = roster.DerivationSignature("specimen", restated)
	if !derivedOK || shape.BuildResults[0] != specimenType("OtherPlan") || shape.CountParams[0] != specimenType("OtherPlan") {
		t.Fatal("the derivation State is not the type the call threads")
	}

	foreign := relation
	foreign.Derivation.StaticAxes = []schema.EntryReference{{Surface: schema.SurfaceKindAxis, Key: "absent"}}
	if _, derivedOK = roster.DerivationSignature("specimen", foreign); derivedOK {
		t.Fatal("a static axis no source owns resolved a schema type")
	}
}
