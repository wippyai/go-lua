package staticcheck

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestStaticCheckRejectsForeignStaticAndModuleProofSplice(t *testing.T) {
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1),
		checkCount(keyspace.FamilyCell, 1), checkCount(keyspace.FamilyValues, 1), checkCount(keyspace.FamilyBind, 1),
		checkCount(keyspace.FamilyTypePrimitive, 1),
		checkCount(keyspace.FamilyTypeFunction, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	first := newCheckFixture(t, checkSpec{
		name: "staticcheck-provenance.lua", counts: counts,
		rows: [][]keyspace.Term{{bind}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}}, Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values}},
		}},
		static: static.Input{
			Types:      static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
			Signatures: static.SignaturesInput{TypeFunction: []static.TypeFunction{{Scope: cell, ReturnsKnown: true, Returns: []keyspace.Term{primitive}}}},
		},
	})
	second := newCheckFixture(t, checkSpec{
		name: "staticcheck-provenance.lua", counts: counts,
		rows: [][]keyspace.Term{{bind}}, binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}}}, Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values}},
		}},
		static: static.Input{
			Types:      static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveString}}},
			Signatures: static.SignaturesInput{TypeFunction: []static.TypeFunction{{Scope: cell, ReturnsKnown: true, Returns: []keyspace.Term{primitive}}}},
		},
	})
	if _, err := Validate(
		first.sourceView, first.flowView, second.staticView, first.bodies,
		first.bindings, first.forest, first.proof, first.access,
		first.moduleView.ContentID(), first.entry,
	); err == nil {
		t.Fatal("Validate accepted a same-cardinality foreign Static proof splice")
	}
	foreignModule := first.moduleView.ContentID()
	foreignModule[0] ^= 0xff
	if _, err := Validate(
		first.sourceView, first.flowView, first.staticView, first.bodies,
		first.bindings, first.forest, first.proof, first.access,
		foreignModule, first.entry,
	); err == nil {
		t.Fatal("Validate accepted a foreign Module proof splice")
	}
	if _, err := Validate(
		first.sourceView, first.flowView, first.staticView, first.bodies,
		first.bindings, first.forest, first.proof, second.access,
		first.moduleView.ContentID(), first.entry,
	); err == nil {
		t.Fatal("Validate accepted a foreign selector proof splice")
	}
}

func TestStaticCheckNilOrMalformedInputsReturnEmptyResult(t *testing.T) {
	counts := checkCounts(checkCount(keyspace.FamilyBody, 1))
	fixture := newCheckFixture(t, checkSpec{counts: counts})
	empty := static.CommitInput{}
	got, err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, nil, fixture.proof, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err == nil || len(got.TypeOf) != len(empty.TypeOf) || len(got.Annotations) != len(empty.Annotations) || len(got.Publications) != len(empty.Publications) {
		t.Fatalf("nil containment result = %#v/%v", got, err)
	}
	got, err = Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, nil, fixture.access,
		fixture.moduleView.ContentID(), fixture.entry,
	)
	if err == nil || len(got.TypeOf) != 0 || len(got.Annotations) != 0 || len(got.Publications) != 0 {
		t.Fatalf("nil scope proof = %#v/%v", got, err)
	}
}

func TestStaticCheckRejectsEveryForeignIdentitySubstitution(t *testing.T) {
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyRead, 2), checkCount(keyspace.FamilyValues, 3),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyCall, 2),
		checkCount(keyspace.FamilyTypePrimitive, 1), checkCount(keyspace.FamilyTypeFunction, 1),
	)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	values1 := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	values2 := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	values3 := keyspace.MakeTerm(keyspace.FamilyValues, 3)
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	makeSpec := func(name string, swapped bool) checkSpec {
		actuals := values2
		other := values3
		if swapped {
			actuals, other = other, actuals
		}
		return checkSpec{
			name: name, counts: counts,
			rows:  [][]keyspace.Term{{bind, call1, call2}},
			binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
			flow: authored.Input{
				Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}, {Owner: body}}},
				Storage: authored.StorageInput{
					Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
					Reads: []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: cell}},
					Binds: []authored.Bind{{Owner: body, Values: values1}},
				},
				Calls: []authored.Call{{Owner: body, Callee: read, Actuals: actuals}, {Owner: body, Callee: read2, Actuals: other}},
			},
			static: static.Input{
				Types:      static.TypesInput{Primitive: []static.Primitive{{Kind: static.PrimitiveNumber}}},
				Signatures: static.SignaturesInput{TypeFunction: []static.TypeFunction{{Scope: cell, ReturnsKnown: true, Returns: []keyspace.Term{primitive}}}},
				Contracts:  static.ContractsInput{Call: []static.CallContract{{}, {}}},
			},
		}
	}
	base := newCheckFixture(t, makeSpec("staticcheck-four-id.lua", false))
	foreignSource := newCheckFixture(t, makeSpec("staticcheck-four-id-foreign.lua", false))
	if _, err := Validate(
		foreignSource.sourceView, base.flowView, base.staticView, base.bodies,
		base.bindings, base.forest, base.proof, base.access, base.moduleView.ContentID(), base.entry,
	); err == nil {
		t.Fatal("Validate accepted a foreign Source identity")
	}
	foreignFlow := newCheckFixture(t, makeSpec("staticcheck-four-id.lua", true))
	if _, err := Validate(
		base.sourceView, base.flowView, base.staticView, base.bodies,
		foreignFlow.bindings, base.forest, base.proof, base.access, base.moduleView.ContentID(), base.entry,
	); err == nil {
		t.Fatal("Validate accepted a foreign Binding result")
	}
	if _, err := Validate(
		base.sourceView, foreignFlow.flowView, base.staticView, base.bodies,
		base.bindings, base.forest, base.proof, base.access, base.moduleView.ContentID(), base.entry,
	); err == nil {
		t.Fatal("Validate accepted a foreign Flow identity")
	}
}
