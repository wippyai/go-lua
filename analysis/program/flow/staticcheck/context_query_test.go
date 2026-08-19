package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
)

// This is the lowered shape of:
//
//	type Snapshot = typeof(function()
//	    local function f() return f() end
//	    return f
//	end)
//
// The outer Function is the TypeOf endpoint. Its static Body traversal reaches
// the inner Function through Bind -> Values, where the ordinary Values parent
// is retained and Binding can prove the exact FunctionCell self exception.
func TestStaticCheckNestedStaticFunctionSelfCaptureValidate(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	body3 := keyspace.MakeTerm(keyspace.FamilyBody, 3)
	outer := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyFunction, 2)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	innerSelf := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	innerCapture := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	return1 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return2 := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	values4 := keyspace.MakeTerm(keyspace.FamilyValues, 4)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 3), checkCount(keyspace.FamilyFunction, 2),
		checkCount(keyspace.FamilyCell, 3), checkCount(keyspace.FamilyBind, 2),
		checkCount(keyspace.FamilyReturn, 2), checkCount(keyspace.FamilyValues, 4),
		checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-nested-function-self-capture.lua", counts: counts,
		rows: [][]keyspace.Term{{bind1}, {bind2, return1}, {return2}},
		binds: []source.BindCells{
			{Bind: bind1, Cells: []keyspace.Term{scopeCell}},
			{Bind: bind2, Cells: []keyspace.Term{innerSelf}},
		},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body1, Fixed: authored.Range{End: 0}},
					{Owner: body2, Fixed: authored.Range{End: 1}},
					{Owner: body2, Fixed: authored.Range{Start: 1, End: 1}},
					{Owner: body3, Fixed: authored.Range{Start: 1, End: 1}},
				},
				Terms: []keyspace.Term{inner},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body3},
				},
				Binds: []authored.Bind{
					{Owner: body1, Values: values1},
					{Owner: body2, Values: values2},
				},
			},
			Functions: authored.FunctionsInput{
				Rows: []authored.Function{
					{Owner: body1, Body: body2},
					{Owner: body2, Body: body3, Captures: authored.Range{End: 1}},
				},
				Captures: []authored.Capture{{Inner: innerCapture, Outer: innerSelf}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{
				{Owner: body2, Values: values3}, {Owner: body3, Values: values4},
			}},
		},
		static: static.Input{
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}, {}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: outer}}},
		},
	})
	if !fixture.forest.Static(outer) || !fixture.forest.Static(inner) {
		t.Fatalf("nested static Functions = outer %v, inner %v", fixture.forest.Static(outer), fixture.forest.Static(inner))
	}
	if parent, ok := fixture.forest.Parent(inner); !ok || parent != values2 {
		t.Fatalf("inner ordinary parent = %v/%v, want Values %v/true", parent, ok, values2)
	}
	if self, ok := fixture.bindings.FunctionCell(inner); !ok || self != innerSelf {
		t.Fatalf("inner FunctionCell = %v/%v, want %v/true", self, ok, innerSelf)
	}
	err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("nested self-capture Validate: %v", err)
	}
}
