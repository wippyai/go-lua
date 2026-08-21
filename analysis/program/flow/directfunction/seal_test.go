package directfunction

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealRejectsSameDenominatorForeignOwners(t *testing.T) {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 3, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	base := directSpec{
		sourceName: "directfunction-owner-a.lua",
		counts:     counts,
		rows:       [][]keyspace.Term{{bind, call}, {}},
		binds:      []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell, outer}}},
		forms:      []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{function, keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body1},
				},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: outer}},
			},
			Calls: []authored.Call{{Owner: body1, Callee: read, Actuals: callValues}},
		},
	}
	foreign := base
	foreign.sourceName = "directfunction-owner-b.lua"
	foreign.flow.Values.Rows = append([]authored.Value(nil), base.flow.Values.Rows...)
	foreign.flow.Functions.Captures = []authored.Capture{{Inner: inner, Outer: cell}}
	left := openDirectFixture(t, base)
	right := openDirectFixture(t, foreign)
	staticID := left.staticView.ContentID()
	moduleID := left.flow.ModuleID()

	if _, err := Seal(left.source, right.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Flow with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, right.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign executable proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, right.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign containment proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, right.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign source-control proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, right.bodies, left.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Body proof with equal denominators was accepted")
	}
	if _, err := Seal(left.source, left.flow, left.bodies, right.bindings, left.forest, left.control, left.executable, staticID, moduleID); err == nil {
		t.Fatal("foreign Binding proof with equal denominators was accepted")
	}
	foreignStaticID := staticID
	foreignStaticID[0] ^= 1
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, foreignStaticID, moduleID); err == nil {
		t.Fatal("foreign Static identity with equal denominators was accepted")
	}
	foreignModuleID := moduleID
	foreignModuleID[0] ^= 1
	if _, err := Seal(left.source, left.flow, left.bodies, left.bindings, left.forest, left.control, left.executable, staticID, foreignModuleID); err == nil {
		t.Fatal("foreign Module identity with equal denominators was accepted")
	}
}

func TestDirectFunctionProvenanceRejectsEqualDenominatorForeignOwners(t *testing.T) {
	baseSpec := directProvenanceSpec()
	base := openDirectFixture(t, baseSpec)

	foreignSourceSpec := baseSpec
	foreignSourceSpec.sourceName = "directfunction-provenance-foreign-source.lua"
	foreignSource := openDirectFixture(t, foreignSourceSpec)

	foreignFlowSpec := baseSpec
	foreignFlowSpec.flow.Values.Rows = append([]authored.Value(nil), baseSpec.flow.Values.Rows...)
	foreignFlowSpec.flow.Functions.Captures = []authored.Capture{{
		Inner: keyspace.MakeTerm(keyspace.FamilyCell, 2),
		Outer: keyspace.MakeTerm(keyspace.FamilyCell, 1),
	}}
	foreignFlow := openDirectFixture(t, foreignFlowSpec)

	sourceID := base.source.Identity().ContentID()
	flowID := base.flow.ContentID()
	staticID := base.staticView.ContentID()
	moduleID := base.flow.ModuleID()
	foreignSourceID := foreignSource.source.Identity().ContentID()
	foreignFlowID := foreignFlow.flow.ContentID()
	foreignSourceStaticID := foreignSource.staticView.ContentID()
	foreignSourceModuleID := foreignSource.flow.ModuleID()
	foreignFlowStaticID := foreignFlow.staticView.ContentID()
	foreignFlowModuleID := foreignFlow.flow.ModuleID()
	if !Matches(base.result, sourceID, flowID, staticID, moduleID) ||
		!Matches(foreignSource.result, foreignSourceID, flowID, foreignSourceStaticID, foreignSourceModuleID) ||
		!Matches(foreignFlow.result, sourceID, foreignFlowID, foreignFlowStaticID, foreignFlowModuleID) {
		t.Fatal("direct-function result did not retain exact four owner identities")
	}
	if sourceID == foreignSourceID || flowID == foreignFlowID ||
		base.source.Identity().TermCount() != foreignSource.source.Identity().TermCount() ||
		base.flow.Values().Count() != foreignFlow.flow.Values().Count() {
		t.Fatal("foreign direct-function fixtures did not preserve equal denominators with distinct identities")
	}
	if Matches(base.result, foreignSourceID, flowID, staticID, moduleID) ||
		Matches(foreignSource.result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("direct-function provenance accepted an equal-denominator foreign Source")
	}
	if Matches(base.result, sourceID, foreignFlowID, staticID, moduleID) ||
		Matches(foreignFlow.result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("direct-function provenance accepted an equal-denominator foreign Flow")
	}
	ids := [4]identity.ContentID{sourceID, flowID, staticID, moduleID}
	for index, name := range []string{"Source", "Flow", "Static", "Module"} {
		foreign := ids[index]
		foreign[0] ^= 1
		candidate := ids
		candidate[index] = foreign
		if Matches(base.result, candidate[0], candidate[1], candidate[2], candidate[3]) {
			t.Fatalf("direct-function Matches accepted foreign %s identity", name)
		}
	}
}

func directProvenanceSpec() directSpec {
	body1 := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	body2 := keyspace.MakeTerm(keyspace.FamilyBody, 2)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	inner := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	outer := keyspace.MakeTerm(keyspace.FamilyCell, 3)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	bindValues := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	callValues := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyNil: 1, keyspace.FamilyBody: 2, keyspace.FamilyCell: 3, keyspace.FamilyValues: 2,
		keyspace.FamilyBind: 1, keyspace.FamilyFunction: 1,
		keyspace.FamilyRead: 1, keyspace.FamilyCall: 1,
	}
	return directSpec{
		sourceName: "directfunction-provenance.lua",
		counts:     counts,
		rows:       [][]keyspace.Term{{bind, call}, {}},
		binds:      []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell, outer}}},
		forms:      []source.FunctionFormals{{Function: function}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body1, Fixed: authored.Range{End: 2}}, {Owner: body1, Fixed: authored.Range{Start: 2, End: 2}}},
				Terms: []keyspace.Term{function, keyspace.MakeTerm(keyspace.FamilyNil, 1)},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: body1},
					{Kind: authored.CellLocal, Body: body2},
					{Kind: authored.CellLocal, Body: body1},
				},
				Reads: []authored.Read{{Owner: body1, Source: cell}},
				Binds: []authored.Bind{{Owner: body1, Values: bindValues}},
			},
			Functions: authored.FunctionsInput{
				Rows:     []authored.Function{{Owner: body1, Body: body2, Captures: authored.Range{End: 1}}},
				Captures: []authored.Capture{{Inner: inner, Outer: outer}},
			},
			Calls: []authored.Call{{Owner: body1, Callee: read, Actuals: callValues}},
		},
	}
}
