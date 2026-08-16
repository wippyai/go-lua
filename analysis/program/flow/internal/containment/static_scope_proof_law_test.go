package containment

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticScopeProofCellIntroductionAndSourceOccurrence(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyCell, 1),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyTypeValue, 1),
		c(keyspace.FamilyTypeAlias, 1),
		c(keyspace.FamilyTypeOf, 2),
		c(keyspace.FamilyTypePrimitive, 2),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	typeValuePrimitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("source coordinate fixture is invalid")
	}
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{bind, alias}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			TypeValues: []authored.TypeValue{{Owner: body}},
		},
		static: static.Input{
			Types: static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveAny}, {Kind: static.PrimitiveString}}},
			Declarations: static.DeclarationsInput{Alias: []static.TypeAlias{{
				Owner: body, Target: primitive, Name: 1, NameCoordinate: coordinate,
			}}},
			Operands: static.OperandsInput{TypeValue: []static.TypeValueTarget{{Target: typeValuePrimitive}}},
			Operators: static.OperatorsInput{TypeOf: []static.TypeOf{
				{Scope: cell, Operand: typeValue},
				{Scope: alias, Operand: typeValue},
			}},
		},
		module: emptyModule(t),
		binds:  []source.BindCells{{Cells: []keyspace.Term{cell}}},
	})
	_, proof, err := fixture.proveWithScope()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	sourceID := fixture.preimage.Identity().ContentID()
	flowID := fixture.flowView.Cold().ContentID()
	moduleID := fixture.moduleView.ContentID()
	staticID := fixture.staticView.ContentID()
	if !proof.Matches(sourceID, flowID, staticID, moduleID) {
		t.Fatal("scope proof does not match its exact Source/Flow/Module/Static owners")
	}
	if bodyFor, ok := proof.Body(cell); !ok || bodyFor != body {
		t.Fatalf("Cell Body = %v/%v, want %v/true", bodyFor, ok, body)
	}
	if kind, observed, ok := proof.Observation(cell); !ok || kind != ScopeObservationCellIntroduction || observed != cell {
		t.Fatalf("Cell observation = %v/%v/%v, want CellIntroduction/%v/true", kind, observed, ok, cell)
	}
	if bodyFor, ok := proof.Body(alias); !ok || bodyFor != body {
		t.Fatalf("Alias Body = %v/%v, want %v/true", bodyFor, ok, body)
	}
	if kind, observed, ok := proof.Observation(alias); !ok || kind != ScopeObservationSourceOccurrence || observed != alias {
		t.Fatalf("Alias observation = %v/%v/%v, want SourceOccurrence/%v/true", kind, observed, ok, alias)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		proof.Matches(sourceID, flowID, staticID, moduleID)
		proof.Body(cell)
		proof.Observation(alias)
	}); allocations != 0 {
		t.Fatalf("scope proof queries allocated %.2f times", allocations)
	}
	foreignSource := sourceID
	foreignSource[0] ^= 0xff
	if proof.Matches(foreignSource, flowID, staticID, moduleID) {
		t.Fatal("scope proof accepted an equal-cardinality foreign Source identity")
	}
	foreignFlow := flowID
	foreignFlow[0] ^= 0xff
	if proof.Matches(sourceID, foreignFlow, staticID, moduleID) {
		t.Fatal("scope proof accepted an equal-cardinality foreign Flow identity")
	}
	// Keep the exact fixture counts/rows while changing only the foreign
	// scalar owner identity: a scope proof must reject this splice before its
	// lexical rows are observed.
	foreignStatic := staticID
	foreignStatic[0] ^= 0xff
	if proof.Matches(sourceID, flowID, foreignStatic, moduleID) {
		t.Fatal("scope proof accepted an equal-cardinality foreign Static identity")
	}
	foreignModule := moduleID
	foreignModule[0] ^= 0xff
	if proof.Matches(sourceID, flowID, staticID, foreignModule) {
		t.Fatal("scope proof accepted an equal-cardinality foreign Module identity")
	}
	zero := *proof
	zero.staticID = identity.ContentID{}
	if zero.Matches(sourceID, flowID, staticID, moduleID) {
		t.Fatal("zero-provenance scope proof bypassed Matches")
	}
	if bodyFor, ok := zero.Body(cell); ok || bodyFor != 0 {
		t.Fatalf("zero-provenance scope Body = %v/%v, want 0/false", bodyFor, ok)
	}
	if observationKind, observed, ok := zero.Observation(alias); ok || observationKind != ScopeObservationInvalid || observed != 0 {
		t.Fatalf("zero-provenance scope Observation = %v/%v/%v, want invalid/0/false", observationKind, observed, ok)
	}
}

func TestStaticScopeProofDistinguishesFunctionGenericAndHeader(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 2),
		c(keyspace.FamilyCell, 2),
		c(keyspace.FamilyValues, 2),
		c(keyspace.FamilyBind, 1),
		c(keyspace.FamilyReturn, 1),
		c(keyspace.FamilyRead, 2),
		c(keyspace.FamilyFunction, 1),
		c(keyspace.FamilyTypeParam, 1),
		c(keyspace.FamilyTypeOf, 2),
	)
	outer := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	outerCell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	innerCell := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outerRead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	innerRead := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	generic := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{{keyspace.MakeTerm(keyspace.FamilyBind, 1), keyspace.MakeTerm(keyspace.FamilyReturn, 1)}, nil},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: outer}, {Owner: outer, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{function},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: outer},
					{Kind: authored.CellLocal, Body: inner},
				},
				Reads: []authored.Read{
					{Owner: outer, Source: outerCell},
					{Owner: inner, Source: innerCell},
				},
				Binds: []authored.Bind{{Owner: outer, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)}},
			},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: outer, Values: keyspace.MakeTerm(keyspace.FamilyValues, 2)}}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: outer, Body: inner}}},
		},
		static: static.Input{
			Declarations: static.DeclarationsInput{TypeParam: []static.TypeParam{{Owner: function, Name: 1}}},
			Contracts:    static.ContractsInput{Function: []static.FunctionContract{{TypeParams: []keyspace.Term{generic}}}},
			Operators: static.OperatorsInput{TypeOf: []static.TypeOf{
				{Scope: generic, Operand: outerRead},
				{Scope: function, Operand: innerRead},
			}},
		},
		module:  emptyModule(t),
		binds:   []source.BindCells{{Cells: []keyspace.Term{outerCell}}},
		formals: []source.FunctionFormals{{Function: function, Formals: []keyspace.Term{innerCell}}},
	})
	_, proof, err := fixture.proveWithScope()
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if body, ok := proof.Body(generic); !ok || body != outer {
		t.Fatalf("generic Body = %v/%v, want %v/true", body, ok, outer)
	}
	if kind, observed, ok := proof.Observation(generic); !ok || kind != ScopeObservationFunctionGeneric || observed != function {
		t.Fatalf("generic observation = %v/%v/%v", kind, observed, ok)
	}
	if body, ok := proof.Body(function); !ok || body != inner {
		t.Fatalf("header Body = %v/%v, want %v/true", body, ok, inner)
	}
	if kind, observed, ok := proof.Observation(function); !ok || kind != ScopeObservationFunctionHeader || observed != function {
		t.Fatalf("header observation = %v/%v/%v", kind, observed, ok)
	}
}

func TestStaticScopeProofRejectsForwardingCycle(t *testing.T) {
	counts := countsFor(
		c(keyspace.FamilyBody, 1),
		c(keyspace.FamilyNil, 1),
		c(keyspace.FamilyValues, 1),
		c(keyspace.FamilyTypeOf, 1),
		c(keyspace.FamilyAnnotation, 1),
		c(keyspace.FamilyTypePrimitive, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilValue := keyspace.MakeTerm(keyspace.FamilyNil, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	annotation := keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)
	fixture := newProofFixture(t, proofSpec{
		counts: counts,
		rows:   [][]keyspace.Term{nil},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}},
		},
		static: static.Input{
			Types:     static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveAny}}},
			Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{Scope: annotation, Operand: nilValue}}},
			Operands: static.OperandsInput{Annotation: []static.Annotation{{
				Scope: annotation, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Values: values, Name: 1,
			}}},
		},
		module: emptyModule(t),
	})
	if _, _, err := fixture.proveWithScope(); err == nil {
		t.Fatal("Prove accepted a cyclic static-scope forwarding chain")
	}
}
