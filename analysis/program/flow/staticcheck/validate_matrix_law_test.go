package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func TestStaticCheckValidateRejectsImplicitStaticRead(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	root := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	baseRead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	assignValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyRead, 2), checkCount(keyspace.FamilyKey, 1), checkCount(keyspace.FamilyLensExact, 1),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyAssign, 1), checkCount(keyspace.FamilyWrite, 1),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyTypePrimitive, 1),
		checkCount(keyspace.FamilyDeclaredType, 1), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-implicit-read-matrix.lua", counts: counts,
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
		keys:   []source.KeyInput{source.NameKey(body, "global")},
		rows:   [][]keyspace.Term{{bind, assign}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: baseRead, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: root}, {Owner: body, Source: root, Implicit: true}},
				Binds:   []authored.Bind{{Owner: body, Values: values}},
				Assigns: []authored.Assign{{Owner: body, Values: assignValues}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}},
			},
		},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{Cell: cell, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}}},
			Operators:    staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: cell, Operand: read}}},
		},
	})
	err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.flowView.ModuleID(), fixture.entry,
	)
	if err == nil {
		t.Fatal("Validate accepted an implicit static Read")
	}
}

func TestStaticCheckValidateCombinedCanonicalResult(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	rootCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	nil3 := keyspace.MakeTerm(keyspace.FamilyNil, 3)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	assign := keyspace.MakeTerm(keyspace.FamilyAssign, 1)
	typeRef := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyNil, 3), checkCount(keyspace.FamilyKey, 1),
		checkCount(keyspace.FamilyRead, 1), checkCount(keyspace.FamilyLensExact, 1),
		checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyBind, 1),
		checkCount(keyspace.FamilyAssign, 1), checkCount(keyspace.FamilyWrite, 1),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyTypeRef, 1),
		checkCount(keyspace.FamilyTypeOf, 1), checkCount(keyspace.FamilyAnnotation, 1),
		checkCount(keyspace.FamilyTypePublication, 1), checkCount(keyspace.FamilyDeclaredType, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-combined-result.lua", counts: counts,
		rows:   [][]keyspace.Term{{bind, assign}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(body, "field")},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{scopeCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{nil1, nil2},
			},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: body, Base: read, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: rootCell}},
				Binds:   []authored.Bind{{Owner: body, Values: values1}},
				Assigns: []authored.Assign{{Owner: body, Values: values2}},
				Writes:  []authored.Write{{Assign: assign, Target: lens}},
			},
		},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{Cell: scopeCell, Target: primitive}}},
			References:   staticrefs.Input{TypeRef: []staticrefs.TypeRef{{Resolution: staticrefs.CanonicalPath, Root: rootCell, Source: []keyspace.Key{1, 2}, Canonical: []keyspace.Key{1}}}},
			Operators:    staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: nil3}}},
			Operands:     staticoperands.Input{Annotation: []staticoperands.Annotation{{Scope: scopeCell, Target: primitive, Name: 1, Values: values3}}},
			Publications: staticpubs.Input{Type: []staticpubs.Publication{{Assign: assign, Pair: 0, Target: typeRef}}},
		},
	})
	err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.flowView.ModuleID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("combined Validate: %v", err)
	}
}

func TestStaticCheckValidateFunctionVarargHeader(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	varargCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	vararg := keyspace.MakeTerm(keyspace.FamilyVararg, 1)
	return1 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return2 := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	functionValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	returnValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyCell, 1), checkCount(keyspace.FamilyVararg, 1),
		checkCount(keyspace.FamilyReturn, 2), checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-function-vararg-header.lua", counts: counts,
		rows:    [][]keyspace.Term{{return1}, {return2}},
		formals: []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: body, Fixed: authored.Range{End: 1}},
					{Owner: functionBody, Fixed: authored.Range{Start: 1, End: 1}},
				},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: functionBody}},
				Varargs: []authored.Vararg{{Owner: functionBody, Cell: varargCell}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody, Vararg: varargCell}}},
			Control: authored.ControlInput{Returns: []authored.Return{
				{Owner: body, Values: functionValues}, {Owner: functionBody, Values: returnValues},
			}},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: function, Operand: vararg}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	if err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.flowView.ModuleID(), fixture.entry,
	); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
