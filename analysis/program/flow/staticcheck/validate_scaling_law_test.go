package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
)

func TestStaticCheckValidateGenericScopeScaling(t *testing.T) {
	const n = 4096
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyValues, 1), checkCount(keyspace.FamilyReturn, 1),
		checkCount(keyspace.FamilyTypeParam, n), checkCount(keyspace.FamilyNil, n),
		checkCount(keyspace.FamilyTypeOf, n),
	)

	typeParams := make([]staticdecl.TypeParam, n)
	contractParams := make([]keyspace.Term, n)
	typeOfs := make([]staticoperators.TypeOf, n)
	for index := 0; index < n; index++ {
		param := keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(index+1))
		typeParams[index] = staticdecl.TypeParam{Owner: function, Name: 1}
		contractParams[index] = param
		typeOfs[index] = staticoperators.TypeOf{Scope: param, Operand: keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+1))}
	}

	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-validate-generic-scaling.lua", counts: counts,
		rows: [][]keyspace.Term{{keyspace.MakeTerm(keyspace.FamilyReturn, 1)}, {}},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{function}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)}}},
		},
		static: static.Input{
			Declarations: staticdecl.Input{TypeParam: typeParams},
			Operators:    staticoperators.Input{TypeOf: typeOfs},
			Contracts:    staticcontracts.Input{Function: []staticcontracts.FunctionContract{{TypeParams: contractParams}}},
		},
	})
	err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.flowView.ModuleID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestStaticCheckValidatePositionlessFunctionBodyScaling(t *testing.T) {
	const n = 4096
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	functions := make([]authored.Function, n)
	typeOfs := make([]staticoperators.TypeOf, n)
	contracts := make([]staticcontracts.FunctionContract, n)
	for index := 0; index < n; index++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		functionBody := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+2))
		functions[index] = authored.Function{Owner: body, Body: functionBody}
		typeOfs[index] = staticoperators.TypeOf{Scope: cell, Operand: function}
	}
	rows := make([][]keyspace.Term, n+1)
	rows[0] = []keyspace.Term{bind}
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, n+1), checkCount(keyspace.FamilyCell, 1), checkCount(keyspace.FamilyBind, 1),
		checkCount(keyspace.FamilyValues, 1), checkCount(keyspace.FamilyFunction, n), checkCount(keyspace.FamilyTypeOf, n),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-positionless-function-scaling.lua", counts: counts, rows: rows,
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values}}},
			Functions: authored.FunctionsInput{Rows: functions},
		},
		static: static.Input{Contracts: staticcontracts.Input{Function: contracts}, Operators: staticoperators.Input{TypeOf: typeOfs}},
	})
	err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.flowView.ModuleID(), fixture.entry)
	if err != nil {
		t.Fatalf("Validate positionless Function scaling: %v", err)
	}
}
