package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/program/flow/internal/executable"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// openPendingMatrixFixture is the same complete production assembly used by
// the matrix tests. Keeping the four owners and their proofs together here
// makes provenance tests exercise SealPending's actual authority boundary,
// rather than manufacturing a Result with matching-looking IDs.
func openPendingMatrixFixture(t *testing.T, name string, flow authored.Input) *pendingFixture {
	t.Helper()
	body := pendingTerm(keyspace.FamilyBody, 1)
	return openPendingFixture(t, name, pendingRuntimeMatrixCounts(), pendingRuntimeMatrixRows(), flow, []source.BindCells{
		{Bind: pendingTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 2)}},
		{Bind: pendingTerm(keyspace.FamilyBind, 2), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 3)}},
		{Bind: pendingTerm(keyspace.FamilyBind, 3), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 4)}},
	}, nil, nil, pendingSourceExtras{
		keys: []source.KeyInput{
			source.NameKey(body, "field-list"), source.NameKey(body, "field-name"), source.NameKey(body, "method"),
		},
		exactAtoms: []keyspace.LiteralValue{
			{Kind: keyspace.LiteralString, String: "field-list"},
			{Kind: keyspace.LiteralString, String: "field-name"},
			{Kind: keyspace.LiteralString, String: "method"},
		},
	})
}

func TestSealPendingProductionRejectsEachProvenanceQuartetMismatch(t *testing.T) {
	first := openPendingMatrixFixture(t, "pending-provenance-a.lua", pendingRuntimeMatrixFlow())
	foreignSource := openPendingMatrixFixture(t, "pending-provenance-b.lua", pendingRuntimeMatrixFlow())
	foreignFlowInput := pendingRuntimeMatrixFlow()
	foreignFlowInput.Operators.Selects[0].Op = kind.SelectOr
	foreignFlow := openPendingMatrixFixture(t, "pending-provenance-a.lua", foreignFlowInput)
	foreignStaticID := keyspace.ContentID{0: 0xA1}
	foreignModuleID := keyspace.ContentID{0: 0xB2}

	cases := []struct {
		name   string
		source source.View
		flow   authored.View
		exec   *executable.Result
		cand   *candidates.Result
		static keyspace.ContentID
		module keyspace.ContentID
	}{
		{
			name: "Source", source: foreignSource.sourceView, flow: first.flowView,
			exec: first.executable, cand: first.candidates, static: first.staticID, module: first.moduleID,
		},
		{
			name: "Flow", source: first.sourceView, flow: foreignFlow.flowView,
			exec: first.executable, cand: first.candidates, static: first.staticID, module: first.moduleID,
		},
		{
			name: "Static", source: first.sourceView, flow: first.flowView,
			exec: first.executable, cand: first.candidates, static: foreignStaticID, module: first.moduleID,
		},
		{
			name: "Module", source: first.sourceView, flow: first.flowView,
			exec: first.executable, cand: first.candidates, static: first.staticID, module: foreignModuleID,
		},
		{
			name: "foreign executable", source: first.sourceView, flow: first.flowView,
			exec: foreignSource.executable, cand: first.candidates, static: first.staticID, module: first.moduleID,
		},
		{
			name: "foreign candidates", source: first.sourceView, flow: first.flowView,
			exec: first.executable, cand: foreignSource.candidates, static: first.staticID, module: first.moduleID,
		},
	}
	for _, cases := range cases {
		t.Run(cases.name, func(t *testing.T) {
			if pending, err := SealPending(cases.source, cases.flow, cases.exec, cases.cand, cases.static, cases.module); err == nil || pending != nil {
				t.Fatalf("SealPending accepted %s mismatch: err=%v", cases.name, err)
			}
		})
	}
}

func TestSealPendingProductionRejectsUnavailableStaticAndModule(t *testing.T) {
	fixture := openPendingMatrixFixture(t, "pending-provenance-unavailable.lua", pendingRuntimeMatrixFlow())
	zero := keyspace.ContentID{}
	if pending, err := SealPending(fixture.sourceView, fixture.flowView, fixture.executable, fixture.candidates, zero, fixture.moduleID); err == nil || pending != nil {
		t.Fatalf("SealPending accepted unavailable Static: pending=%v err=%v", pending, err)
	}
	if pending, err := SealPending(fixture.sourceView, fixture.flowView, fixture.executable, fixture.candidates, fixture.staticID, zero); err == nil || pending != nil {
		t.Fatalf("SealPending accepted unavailable Module: pending=%v err=%v", pending, err)
	}
}
