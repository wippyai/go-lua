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

func localReadFixture(t *testing.T) *checkFixture {
	t.Helper()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	staticRead := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	values := keyspace.MakeTerm(keyspace.FamilyValues, 1)
	actuals := keyspace.MakeTerm(keyspace.FamilyValues, 2)
	bind := keyspace.MakeTerm(keyspace.FamilyBind, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	counts := checkCounts(
		checkCount(keyspace.FamilyBody, 1), checkCount(keyspace.FamilyCell, 1),
		checkCount(keyspace.FamilyRead, 2), checkCount(keyspace.FamilyValues, 2),
		checkCount(keyspace.FamilyBind, 1), checkCount(keyspace.FamilyCall, 1), checkCount(keyspace.FamilyTypeOf, 1),
	)
	return newCheckFixture(t, checkSpec{
		name: "staticcheck-local-read.lua", counts: counts,
		rows:  [][]keyspace.Term{{bind, call}},
		binds: []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: body}, {Owner: body}}},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}},
				Reads: []authored.Read{{Owner: body, Source: cell}, {Owner: body, Source: cell}},
				Binds: []authored.Bind{{Owner: body, Values: values}},
			},
			Calls: []authored.Call{{Owner: body, Callee: read, Actuals: actuals}},
		},
		static: static.Input{
			Contracts: staticcontracts.Input{Call: []staticcontracts.CallContract{{}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: call, Operand: staticRead}}},
		},
	})
}

func TestStaticCheckReadMembershipAndBindVisibilityPhases(t *testing.T) {
	fixture := localReadFixture(t)
	tree, err := buildContext(fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies, fixture.bindings, fixture.entry)
	if err != nil {
		t.Fatalf("buildContext: %v", err)
	}
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	points := newObservationPoints(fixture.sourceView, fixture.flowView, fixture.forest, tree)
	if err := validateStaticReadAt(fixture.flowView, fixture.forest, fixture.bindings, points, read); err != nil {
		t.Fatalf("validateStaticRead: %v", err)
	}
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	pre, ok := tree.cellScope[keyspace.TermOrdinal(cell)], true
	if !ok || pre == 0 || tree.cellVisible(pre, cell) {
		t.Fatalf("Bind cell visible at pre-Bind observation point: point=%d", pre)
	}
	post, ok := tree.pointAt(fixture.entry, 1)
	if !ok || !tree.cellVisible(post, cell) {
		t.Fatalf("Bind cell not visible after Bind: point=%d", post)
	}
	if err := Validate(
		fixture.sourceView, fixture.flowView, fixture.staticView, fixture.bodies,
		fixture.bindings, fixture.forest, fixture.proof, fixture.access,
		fixture.flowView.ModuleID(), fixture.entry,
	); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
