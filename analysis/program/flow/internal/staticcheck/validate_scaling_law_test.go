package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
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

	typeParams := make([]static.TypeParam, n)
	contractParams := make([]keyspace.Term, n)
	typeOfs := make([]static.TypeOf, n)
	for index := 0; index < n; index++ {
		param := keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(index+1))
		typeParams[index] = static.TypeParam{Owner: function, Name: 1}
		contractParams[index] = param
		typeOfs[index] = static.TypeOf{Scope: param, Operand: keyspace.MakeTerm(keyspace.FamilyNil, uint32(index+1))}
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
			Declarations: static.DeclarationsInput{TypeParam: typeParams},
			Operators:    static.OperatorsInput{TypeOf: typeOfs},
			Contracts:    static.ContractsInput{Function: []static.FunctionContract{{TypeParams: contractParams}}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(result.TypeOf) != n {
		t.Fatalf("TypeOf result length = %d, want %d", len(result.TypeOf), n)
	}
}

func TestStaticCheckValidatePositionlessFunctionBodyScaling(t *testing.T) {
	const n = 4096
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	functions := make([]authored.Function, n)
	typeOfs := make([]static.TypeOf, n)
	contracts := make([]static.FunctionContract, n)
	for index := 0; index < n; index++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		functionBody := keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+2))
		functions[index] = authored.Function{Owner: body, Body: functionBody}
		typeOfs[index] = static.TypeOf{Scope: cell, Operand: function}
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
		static: static.Input{Contracts: static.ContractsInput{Function: contracts}, Operators: static.OperatorsInput{TypeOf: typeOfs}},
	})
	result, err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.moduleView.ContentID(), fixture.entry)
	if err != nil {
		t.Fatalf("Validate positionless Function scaling: %v", err)
	}
	if len(result.TypeOf) != n {
		t.Fatalf("TypeOf result length = %d, want %d", len(result.TypeOf), n)
	}
}
