package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func TestStaticCheckTypeOfScopeAndStaticOperandIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	child := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	formalCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	return1 := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	return2 := keyspace.MakeTerm(keyspace.FamilyReturn, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyRead, 1),
		checkCount(keyspace.FamilyReturn, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-typeof-integration.lua", counts: counts,
		rows:    [][]keyspace.Term{{bind, return1}, {return2}},
		binds:   []source.BindCells{{Bind: bind, Cells: []keyspace.Term{outerCell}}},
		formals: []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{formalCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{Start: 0, End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}, {Owner: child, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: child}},
				Reads: []authored.Read{{Owner: child, Source: formalCell}},
				Binds: []authored.Bind{{Owner: body, Values: values1}},
			},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: values2}, {Owner: child, Values: values3}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: child}}},
		},
		static: static.Input{
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: formalCell, Operand: read}}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("Validate seeded static Read: %v", err)
	}
	if len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) {
		t.Fatalf("TypeOf result = %#v", result.TypeOf)
	}
}

func TestStaticCheckAnnotationScopeOwnerAndStaticValuesIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyValues, 2),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyDeclaredType, 1), checkCount(keyspace.FamilyAnnotation, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-annotation-integration.lua", counts: counts,
		rows:  [][]keyspace.Term{{bind}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values1}},
			},
		},
		static: static.Input{
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{Cell: cell, Target: primitive}}},
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Operands: staticoperands.Input{Annotation: []staticoperands.Annotation{{
				Scope: cell, Target: primitive, Name: 1, Values: values2,
			}}},
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
	if len(result.Annotations) != 1 || result.Annotations[0] != keyspace.MakeTerm(keyspace.FamilyAnnotation, 1) {
		t.Fatalf("Annotation result = %#v", result.Annotations)
	}
}

func TestStaticCheckTypeFunctionSourceOccurrenceIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyTypeAlias, 1),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyTypeFunction, 1),
	)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("invalid source coordinate")
	}
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-source-occurrence.lua", counts: counts,
		rows:   [][]keyspace.Term{{alias}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "alias"}},
		static: static.Input{
			Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}},
			Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{
				Owner: body, Target: primitive, Name: 1, NameCoordinate: coordinate,
			}}},
			Signatures: staticsig.Input{TypeFunction: []staticsig.TypeFunction{{
				Scope: alias,
			}}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("SourceOccurrence Validate: %v", err)
	}
	if len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("SourceOccurrence result = %#v", result)
	}
}

func TestStaticCheckSeededFunctionLensBaseReadIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	globalCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outerRead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	baseRead := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyCell, 2), checkCount(keyspace.FamilyRead, 2),
		checkCount(keyspace.FamilyLensExact, 1), checkCount(keyspace.FamilyKey, 1),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyReturn, 1),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-seeded-function-lens.lua", counts: counts,
		rows:   [][]keyspace.Term{{bind}, {returnTerm}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{scopeCell}}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}},
		keys:   []source.KeyInput{source.NameKey(functionBody, "field")},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: functionBody, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{outerRead},
			},
			Access: authored.AccessInput{Exact: []authored.ExactLens{{Owner: functionBody, Base: baseRead, Source: key, Kind: kind.FieldName}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellGlobal, Key: 1}},
				Reads: []authored.Read{{Owner: functionBody, Source: lens}, {Owner: functionBody, Source: globalCell}},
				Binds: []authored.Bind{{Owner: body, Values: values2}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: functionBody, Values: values1}}},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("seeded Function lens Validate: %v", err)
	}
	if len(result.TypeOf) != 1 || result.TypeOf[0] != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) {
		t.Fatalf("seeded Function lens TypeOf result = %#v", result.TypeOf)
	}
}

func TestStaticCheckRejectsInvisibleStaticLensBaseRead(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	baseCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outerRead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	baseRead := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	lens := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	key := keyspace.MakeTerm(keyspace.FamilyKey, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	baseBind := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyRead, 2), checkCount(keyspace.FamilyLensExact, 1), checkCount(keyspace.FamilyKey, 1),
		checkCount(keyspace.FamilyBind, 2), checkCount(keyspace.FamilyReturn, 1), checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-invisible-lens-base.lua", counts: counts,
		rows:   [][]keyspace.Term{{bind}, {returnTerm, baseBind}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{scopeCell}}, {Bind: baseBind, Cells: []keyspace.Term{baseCell}}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}}, keys: []source.KeyInput{source.NameKey(functionBody, "field")},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: functionBody, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}, {Owner: functionBody, Fixed: authored.Range{Start: 1, End: 1}}}, Terms: []keyspace.Term{outerRead}},
			Access:    authored.AccessInput{Exact: []authored.ExactLens{{Owner: functionBody, Base: baseRead, Source: key, Kind: kind.FieldName}}},
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: functionBody}}, Reads: []authored.Read{{Owner: functionBody, Source: lens}, {Owner: functionBody, Source: baseCell}}, Binds: []authored.Bind{{Owner: body, Values: values2}, {Owner: functionBody, Values: values3}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: body, Body: functionBody}}}, Control: authored.ControlInput{Returns: []authored.Return{{Owner: functionBody, Values: values1}}},
		},
		static: static.Input{Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}}, Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}}},
	})
	if !fixture.forest.Static(baseRead) || !fixture.forest.Static(outerRead) {
		t.Fatal("lens reads were not static test occurrences")
	}
	result, err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.moduleView.ContentID(), fixture.entry)
	if err == nil || len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("invisible lens base Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckSeededFunctionCaptureIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	functionBody := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	scopeCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	innerCell := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 2), checkCount(keyspace.FamilyFunction, 1),
		checkCount(keyspace.FamilyCell, 3), checkCount(keyspace.FamilyBind, 2),
		checkCount(keyspace.FamilyReturn, 1), checkCount(keyspace.FamilyValues, 3),
		checkCount(keyspace.FamilyTypeOf, 1),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-seeded-function-capture.lua", counts: counts,
		rows:  [][]keyspace.Term{{bind1, bind2}, {returnTerm}},
		binds: []source.BindCells{{Bind: bind1, Cells: []keyspace.Term{outerCell}}, {Bind: bind2, Cells: []keyspace.Term{scopeCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 0}},
				{Owner: body, Fixed: authored.Range{Start: 0, End: 0}},
				{Owner: functionBody, Fixed: authored.Range{Start: 0, End: 0}},
			}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: functionBody},
				},
				Binds: []authored.Bind{{Owner: body, Values: values1}, {Owner: body, Values: values2}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body, Body: functionBody, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: innerCell, Outer: outerCell}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: functionBody, Values: values3}}},
		},
		static: static.Input{
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: scopeCell, Operand: function}}},
			Contracts: staticcontracts.Input{Function: []staticcontracts.FunctionContract{{}}},
		},
	})
	if !fixture.forest.Static(function) {
		t.Fatal("capture Function occurrence is not static")
	}
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("seeded Function capture Validate: %v", err)
	}
	if len(result.TypeOf) != 1 {
		t.Fatalf("seeded Function capture TypeOf result = %#v", result.TypeOf)
	}
}

func TestStaticCheckSeededAnnotationReadClosureIntegration(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	readCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyRead, 1), checkCount(keyspace.FamilyValues, 2),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyTypePrimitive, 1),
		checkCount(keyspace.FamilyTypeAlias, 1), checkCount(keyspace.FamilyAnnotation, 1),
	)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("invalid annotation source coordinate")
	}
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-seeded-annotation-read.lua", counts: counts,
		rows:   [][]keyspace.Term{{bind, alias}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{readCell}}},
		exacts: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "alias"}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}},
				Terms: []keyspace.Term{read},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Reads: []authored.Read{{Owner: body, Source: readCell}},
				Binds: []authored.Bind{{Owner: body, Values: bindValues}},
			},
		},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{Owner: body, Target: primitive, Name: 1, NameCoordinate: coordinate}}},
			Operands:     staticoperands.Input{Annotation: []staticoperands.Annotation{{Scope: alias, Target: primitive, Name: 1, Values: values}}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err != nil {
		t.Fatalf("seeded Annotation Read Validate: %v", err)
	}
	if len(result.Annotations) != 1 || result.Annotations[0] != keyspace.MakeTerm(keyspace.FamilyAnnotation, 1) {
		t.Fatalf("seeded Annotation result = %#v", result.Annotations)
	}
}

func TestStaticCheckRejectsConflictingSharedAnnotationSeed(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind1 := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bind2 := keyspace.MakeTerm(keyspace.FamilyBind, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bindValues1 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bindValues2 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 2),
		checkCount(keyspace.FamilyBind, 2), checkCount(keyspace.FamilyValues, 3),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyDeclaredType, 1),
		checkCount(keyspace.FamilyAnnotation, 2),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-conflicting-seed.lua", counts: counts,
		rows:  [][]keyspace.Term{{bind1, bind2}},
		binds: []source.BindCells{{Bind: bind1, Cells: []keyspace.Term{cell1}}, {Bind: bind2, Cells: []keyspace.Term{cell2}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{
				{Owner: body, Fixed: authored.Range{End: 0}}, {Owner: body, Fixed: authored.Range{Start: 0, End: 0}}, {Owner: body, Fixed: authored.Range{Start: 0, End: 0}},
			}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: bindValues1}, {Owner: body, Values: bindValues2}},
			},
		},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{Cell: cell1, Target: primitive}}},
			Operands: staticoperands.Input{Annotation: []staticoperands.Annotation{
				{Scope: cell1, Target: primitive, Name: 1, Values: values},
				{Scope: cell2, Target: primitive, Name: 2, Values: values},
			}},
		},
	})
	result, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err == nil || len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("conflicting shared seed Validate = %#v/%v", result, err)
	}
}

func TestStaticCheckObservationSeedChainIsRowOrderIndependent(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	typeOf1 := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	typeOf2 := keyspace.MakeTerm(keyspace.FamilyTypeOf, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1), checkCount(keyspace.FamilyNil, 2),
		checkCount(keyspace.FamilyCall, 2), checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyValues, 3), checkCount(keyspace.FamilyTypeOf, 2),
	)
	build := func(t *testing.T, rows []staticoperators.TypeOf) (*checkFixture, static.CommitInput, error) {
		fixture := newCheckFixture(t, checkSpec{
			name: "staticcheck-seed-chain.lua", counts: counts, rows: [][]keyspace.Term{{bind}},
			binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
			flow: authored.Input{
				Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}, {Owner: body}}},
				Storage: authored.StorageInput{
					Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values1}},
				},
				Calls: []authored.Call{{Owner: body, Callee: nil1, Actuals: values2}, {Owner: body, Callee: nil2, Actuals: values3}},
			},
			static: static.Input{
				Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}, {}}},
				Operators: staticoperators.Input{TypeOf: rows},
			},
		})
		result, err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.moduleView.ContentID(), fixture.entry)
		return fixture, result, err
	}
	first, result, err := build(t, []staticoperators.TypeOf{{Scope: cell, Operand: call1}, {Scope: call1, Operand: call2}})
	if err != nil || len(result.TypeOf) != 2 || result.TypeOf[0] != typeOf1 || result.TypeOf[1] != typeOf2 {
		t.Fatalf("forward seed chain = %#v/%v", result, err)
	}
	second, result, err := build(t, []staticoperators.TypeOf{{Scope: call1, Operand: call2}, {Scope: cell, Operand: call1}})
	if err != nil || len(result.TypeOf) != 2 || result.TypeOf[0] != typeOf1 || result.TypeOf[1] != typeOf2 {
		t.Fatalf("permuted seed chain = %#v/%v", result, err)
	}
	_ = first
	_ = second
}

func TestStaticCheckRejectsCyclicObservationSeedDescriptors(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nil1 := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	nil2 := keyspace.MakeTerm(keyspace.FamilyNil, 2)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCall, 2), checkCount(keyspace.FamilyNil, 2), checkCount(keyspace.FamilyValues, 2),
		checkCount(keyspace.FamilyTypeOf, 2),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-cyclic-seed-descriptors.lua", counts: counts,
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Storage: authored.StorageInput{},
			Calls:   []authored.Call{{Owner: body, Callee: nil1, Actuals: values1}, {Owner: body, Callee: nil2, Actuals: values2}},
		},
		static: static.Input{
			Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}, {}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: call2, Operand: call1}, {Scope: call1, Operand: call2}}},
		},
	})
	result, err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.moduleView.ContentID(), fixture.entry)
	if err == nil || len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("cyclic seed descriptors = %#v/%v", result, err)
	}
}

func TestStaticCheckRejectsSamePointConflictingDescriptors(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell1 := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	cell2 := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 2), checkCount(keyspace.FamilyBind, 1),
		checkCount(keyspace.FamilyValues, 2), checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyDeclaredType, 1),
		checkCount(keyspace.FamilyAnnotation, 2),
	)
	fixture := newCheckFixture(t, checkSpec{
		name: "staticcheck-same-point-descriptor-conflict.lua", counts: counts,
		rows: [][]keyspace.Term{{bind}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell1, cell2}}},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Storage: authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values1}}},
		},
		static: static.Input{
			Types:        statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{Cell: cell1, Target: primitive}}},
			Operands:     staticoperands.Input{Annotation: []staticoperands.Annotation{{Scope: cell1, Target: primitive, Name: 1, Values: values2}, {Scope: cell2, Target: primitive, Name: 2, Values: values2}}},
		},
	})
	result, err := Validate(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.forest, fixture.proof, fixture.access, fixture.moduleView.ContentID(), fixture.entry)
	if err == nil || len(result.TypeOf) != 0 || len(result.Annotations) != 0 || len(result.Publications) != 0 {
		t.Fatalf("same-point conflicting descriptors = %#v/%v", result, err)
	}
}
