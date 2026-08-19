package position

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
)

func TestSealDirectDoBodyPositionAndCommit(t *testing.T) {
	counts := positionCounts(2, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{body2}, nil},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	row, ok := positionFor(index.Positions, body2)
	if !ok || row.Term != body2 || row.Root != body2 || row.Body != body1 || row.Offset != 0 || row.Cursor != 0 ||
		row.FrontierBody != body1 || row.FrontierCursor != 0 || row.Repeat {
		t.Fatalf("direct do-body position = %#v/%v", row, ok)
	}
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit: %v", err)
	}
	committed := component.View().Index()
	if root, ok := committed.Root(body2); !ok || root != body2 {
		t.Fatalf("committed do-body Root = %v/%v, want %v/true", root, ok, body2)
	}
	body, offset, cursor, ok := committed.Position(body2)
	if !ok || body != body1 || offset != 0 || cursor != 0 {
		t.Fatalf("committed do-body Position = %v/%d/%d/%v", body, offset, cursor, ok)
	}
	frontierBody, frontierCursor, ok := committed.Frontier(body2)
	if !ok || frontierBody != body1 || frontierCursor != 0 {
		t.Fatalf("committed do-body Frontier = %v/%d/%v", frontierBody, frontierCursor, ok)
	}
}

func TestSealFunctionBodyIsTypedButItsDirectContentsArePositioned(t *testing.T) {
	counts := positionCounts(2, 2, 2, 0, 0, 0, 0, 0, 0, 1)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	returns := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyReturn, 1), keyspace.MakeTerm(keyspace.FamilyReturn, 2)}
	values := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2)}
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{returns[0]}, {returns[1]}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 1}}, {Owner: body2, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body1, Body: body2}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body1, Values: values[0]}, {Owner: body2, Values: values[1]}}},
		},
		static: static.Input{Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}}},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	functionRow, ok := positionFor(index.Positions, function)
	if !ok || functionRow.Root != returns[0] || functionRow.Body != body1 {
		t.Fatalf("Function closure position = %#v/%v, want Return 1 root", functionRow, ok)
	}
	contentRow, ok := positionFor(index.Positions, returns[1])
	if !ok || contentRow.Root != returns[1] || contentRow.Body != body2 {
		t.Fatalf("Function Body direct content position = %#v/%v", contentRow, ok)
	}
	assertTypedBodyUnpositioned(t, index, body2)
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit Function Body: %v", err)
	}
	assertCommittedBodyUnpositioned(t, component.View().Index(), body2)
}

func TestSealBranchBodiesAreTypedButTheirDirectContentsArePositioned(t *testing.T) {
	counts := positionCounts(3, 2, 2, 0, 1, 0, 0, 1, 0, 0)
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	whenTrue := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	whenFalse := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	condition := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	returns := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyReturn, 1), keyspace.MakeTerm(keyspace.FamilyReturn, 2)}
	values := []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2)}
	fixture := openPositionFixture(t, positionSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{branch}, {returns[0]}, {returns[1]}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: whenTrue}, {Owner: whenFalse}}},
			Control: authored.ControlInput{
				Returns:  []authored.Return{{Owner: whenTrue, Values: values[0]}, {Owner: whenFalse, Values: values[1]}},
				Branches: []authored.Branch{{Owner: body1, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse}},
			},
		},
	})
	index, err := sealPositionFixture(fixture)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	branchRow, ok := positionFor(index.Positions, branch)
	if !ok || branchRow.Root != branch || branchRow.Body != body1 {
		t.Fatalf("Branch position = %#v/%v", branchRow, ok)
	}
	for _, body := range []keyspace.Term{whenTrue, whenFalse} {
		assertTypedBodyUnpositioned(t, index, body)
	}
	for _, content := range returns {
		row, ok := positionFor(index.Positions, content)
		if !ok || row.Root != content {
			t.Fatalf("Branch Body direct content %v position = %#v/%v", content, row, ok)
		}
	}
	conditionRow, ok := positionFor(index.Positions, condition)
	if !ok || conditionRow.Root != branch {
		t.Fatalf("Branch condition position = %#v/%v", conditionRow, ok)
	}
	component, err := fixture.sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("Source Commit Branch Bodies: %v", err)
	}
	for _, body := range []keyspace.Term{whenTrue, whenFalse} {
		assertCommittedBodyUnpositioned(t, component.View().Index(), body)
	}
}

func assertTypedBodyUnpositioned(t *testing.T, index source.IndexInput, body keyspace.Term) {
	t.Helper()
	if row, ok := positionFor(index.Positions, body); ok {
		t.Fatalf("typed Body %v acquired Position: %#v", body, row)
	}
}

func assertCommittedBodyUnpositioned(t *testing.T, index source.Index, body keyspace.Term) {
	t.Helper()
	if _, _, _, ok := index.Position(body); ok {
		t.Fatalf("typed Body %v acquired committed Position", body)
	}
	if _, ok := index.Root(body); ok {
		t.Fatalf("typed Body %v acquired committed Root", body)
	}
	if _, _, ok := index.Frontier(body); ok {
		t.Fatalf("typed Body %v acquired committed Frontier", body)
	}
}
