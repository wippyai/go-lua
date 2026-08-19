package sourcecontrol

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestValidateBodyParentAgreementAcceptsNestedBody(t *testing.T) {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 2},
		familyCount{keyspace.FamilyValues, 1},
		familyCount{keyspace.FamilyReturn, 1},
	)
	parent, child := term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2)
	returned, values := term(keyspace.FamilyReturn, 1), term(keyspace.FamilyValues, 1)
	fixture := openSemanticFixture(t, semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{child}, {returned}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: child}}, Terms: nil},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: child, Values: values}}},
		},
	})
	if err := validateBodyParentAgreement(fixture.bodies, fixture.forest, parent, counts[keyspace.FamilyBody]); err != nil {
		t.Fatalf("nested Body parent agreement rejected: %v", err)
	}
	if got, ok := fixture.bodies.Parent(child); !ok || got != parent {
		t.Fatalf("Body parent = %v/%v, want %v", got, ok, parent)
	}
	if got, ok := fixture.forest.Parent(child); !ok || got != parent {
		t.Fatalf("containment parent = %v/%v, want %v", got, ok, parent)
	}
}

func TestSealRejectsEqualCardinalityForeignBodyResultSplice(t *testing.T) {
	first := openSemanticFixture(t, bodyParentSpliceSpec(false))
	foreign := openSemanticFixture(t, bodyParentSpliceSpec(true))
	entry := term(keyspace.FamilyBody, 1)
	if first.bodies.BodyCount() != foreign.bodies.BodyCount() {
		t.Fatalf("splice fixtures changed Body cardinality: %d/%d", first.bodies.BodyCount(), foreign.bodies.BodyCount())
	}
	firstParent, firstParentOK := first.bodies.Parent(term(keyspace.FamilyBody, 2))
	foreignParent, foreignParentOK := foreign.bodies.Parent(term(keyspace.FamilyBody, 2))
	if firstParentOK == foreignParentOK && firstParent == foreignParent {
		t.Fatal("splice fixtures unexpectedly share Body 2 parent")
	}
	if _, err := Seal(first.sourceView, first.flow, foreign.bodies, first.forest, first.shape, entry,
		first.staticView.ContentID(), first.moduleFinalize.View().ContentID()); err == nil {
		t.Fatal("equal-cardinality foreign Body Result splice was accepted")
	} else if !strings.Contains(err.Error(), "Body provenance disagrees") {
		t.Fatalf("foreign Body splice failed outside central parent proof: %v", err)
	}
}

func bodyParentSpliceSpec(foreign bool) semanticSpec {
	counts := countsWith(
		familyCount{keyspace.FamilyBody, 4},
		familyCount{keyspace.FamilyValues, 3},
		familyCount{keyspace.FamilyFunction, 3},
		familyCount{keyspace.FamilyBind, 3},
		familyCount{keyspace.FamilyCell, 3},
	)
	bodies := []keyspace.Term{
		term(keyspace.FamilyBody, 1), term(keyspace.FamilyBody, 2),
		term(keyspace.FamilyBody, 3), term(keyspace.FamilyBody, 4),
	}
	binds := []keyspace.Term{
		term(keyspace.FamilyBind, 1), term(keyspace.FamilyBind, 2), term(keyspace.FamilyBind, 3),
	}
	values := []keyspace.Term{
		term(keyspace.FamilyValues, 1), term(keyspace.FamilyValues, 2), term(keyspace.FamilyValues, 3),
	}
	functions := []keyspace.Term{
		term(keyspace.FamilyFunction, 1), term(keyspace.FamilyFunction, 2), term(keyspace.FamilyFunction, 3),
	}
	cells := []keyspace.Term{
		term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2), term(keyspace.FamilyCell, 3),
	}
	functionBodies := []keyspace.Term{bodies[1], bodies[2], bodies[3]}
	if foreign {
		functionBodies = []keyspace.Term{bodies[2], bodies[3], bodies[1]}
	}
	spec := semanticSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{binds[0]}, {binds[1]}, {binds[2]}, nil},
		binds: []source.BindCells{
			{Bind: binds[0], Cells: []keyspace.Term{cells[0]}},
			{Bind: binds[1], Cells: []keyspace.Term{cells[1]}},
			{Bind: binds[2], Cells: []keyspace.Term{cells[2]}},
		},
		forms: []source.FunctionFormals{
			{Function: functions[0]}, {Function: functions[1]}, {Function: functions[2]},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: bodies[0], Fixed: authored.Range{End: 1}},
					{Owner: bodies[1], Fixed: authored.Range{Start: 1, End: 2}},
					{Owner: bodies[2], Fixed: authored.Range{Start: 2, End: 3}},
				},
				Terms: functions,
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: bodies[0]},
					{Kind: authored.CellLocal, Body: bodies[1]},
					{Kind: authored.CellLocal, Body: bodies[2]},
				},
				Binds: []authored.Bind{
					{Owner: bodies[0], Values: values[0]},
					{Owner: bodies[1], Values: values[1]},
					{Owner: bodies[2], Values: values[2]},
				},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{
				{Owner: bodies[0], Body: functionBodies[0]},
				{Owner: bodies[1], Body: functionBodies[1]},
				{Owner: bodies[2], Body: functionBodies[2]},
			}},
		},
	}
	return spec
}
