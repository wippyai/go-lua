package publish_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

// These laws deliberately cross the public state seam twice: the mounted
// application is published into a real database root, then the committed
// root is borrowed through read.Reader and normalized into the small logical
// reference relation.  No fake Reader, physical row, or fallback cell is
// constructed by the test.
//
// The reference Apply currently derives its own output row IDs from a logical
// input row, while the production apply lease carries an already authenticated
// destination CellToken. Those row-identity contracts do not line up, so this
// lane differentially compares the common Publish boundary and records the
// exact Apply-shape gap rather than inventing a mapping.

func payloadLayout(t *testing.T, fixture fixture) arrangement.Layout {
	t.Helper()
	for _, candidate := range fixture.mounted.Arrangement().Layouts() {
		columns := candidate.Columns()
		if candidate.Access().Relation() == fixture.row.Relation() && len(columns) == 2 && columns[0] == fixture.keyColumn && columns[1] == fixture.column {
			return candidate
		}
	}
	t.Fatal("mounted arrangement has no full key/payload layout for differential read")
	return arrangement.Layout{}
}

func oracleScopeForRow(t *testing.T, row model.RowID) relationoracle.Scope {
	t.Helper()
	// read.Row intentionally keeps the mounted scope formula opaque. The
	// deterministic token below exists only because relationoracle.NewRow
	// requires a scope. A logical Relation is one-fiber, so normalize by
	// relation identity rather than by row identity; per-row scope tokens
	// make a valid multi-row committed relation impossible to represent in the
	// reference model.
	content := row.Relation().Content()
	formula, ok := identity.DeriveContentID("engine/relation/publish/w3-scope/v1", content[:])
	if !ok {
		t.Fatal("oracle scope")
	}
	scope, ok := relationoracle.NewScope(formula)
	if !ok {
		t.Fatal("oracle scope token")
	}
	return scope
}

func logicalState(t *testing.T, root database.Version, fixture fixture, layout arrangement.Layout) relationoracle.Relation {
	t.Helper()
	scratch := store.NewReadScratch(fixture.manager)
	reader, ok := read.Bind(root, layout, fixture.view, scratch)
	if !ok || !reader.Available() {
		t.Fatal("bind committed payload reader")
	}
	rows := make([]relationoracle.Row, 0)
	completed, valid := reader.Scan(func(source read.Row) bool {
		if source == nil || !source.Available() {
			t.Fatal("reader emitted unavailable row")
		}
		cells := make([]relationoracle.Cell, 0, len(source.Cells()))
		for _, sourceCell := range source.Cells() {
			cell, cellOK := oracleCell(t, sourceCell)
			if !cellOK {
				t.Fatal("normalize committed cell")
			}
			cells = append(cells, cell)
		}
		row, rowOK := relationoracle.NewRow(source.ID(), oracleScopeForRow(t, source.ID()), cells)
		if !rowOK {
			t.Fatal("normalize committed row")
		}
		rows = append(rows, row)
		return true
	})
	if !completed || !valid {
		t.Fatal("committed payload scan")
	}
	relation, ok := relationoracle.NewRelation(fixture.row.Relation(), rows)
	if !ok {
		t.Fatal("normalize committed relation")
	}
	return relation
}

func oracleCell(t *testing.T, source read.Cell) (relationoracle.Cell, bool) {
	t.Helper()
	value := relationoracle.ValueToken{}
	if source.Value().Available() {
		var ok bool
		value, ok = relationoracle.NewValueToken(source.Type(), source.Value().Opaque())
		if !ok {
			return relationoracle.Cell{}, false
		}
	}
	return relationoracle.NewCell(source.Column(), source.Type(), value, source.Presence())
}

func oracleProposals(t *testing.T, application interface {
	Proposals() (binding.ProposalBatch, bool)
}, relation model.RelationID, defaultType model.TypeID) relationoracle.Relation {
	t.Helper()
	batch, ok := application.Proposals()
	if !ok || !batch.Available() {
		t.Fatal("proposal batch")
	}
	byRow := make(map[model.RowID][]relationoracle.Cell)
	for index := 0; index < batch.Len(); index++ {
		proposal, proposalOK := batch.At(index)
		if !proposalOK || !proposal.Available() {
			t.Fatal("proposal")
		}
		typeID := defaultType
		value := relationoracle.ValueToken{}
		if proposal.Value().Available() {
			typeID = proposal.Value().Type()
			value, ok = relationoracle.NewValueToken(typeID, proposal.Value().Opaque())
			if !ok {
				t.Fatal("proposal value")
			}
		}
		cell, cellOK := relationoracle.NewCell(proposal.Destination().Column(), typeID, value, proposal.Presence())
		if !cellOK {
			t.Fatal("proposal cell")
		}
		rowID := proposal.Destination().Row()
		if rowID.Relation() != relation {
			t.Fatal("proposal relation")
		}
		byRow[rowID] = append(byRow[rowID], cell)
	}
	rows := make([]relationoracle.Row, 0, len(byRow))
	for rowID, cells := range byRow {
		row, rowOK := relationoracle.NewRow(rowID, oracleScopeForRow(t, rowID), cells)
		if !rowOK {
			t.Fatal("proposal row")
		}
		rows = append(rows, row)
	}
	result, ok := relationoracle.NewRelation(relation, rows)
	if !ok {
		t.Fatal("proposal relation")
	}
	return result
}

func oracleRegistry(t *testing.T, typeID model.TypeID) relationoracle.AlgebraRegistry {
	t.Helper()
	entry, ok := relationoracle.NewAlgebraEntry(typeID, relationoracle.IdentityAlgebra{})
	if !ok {
		t.Fatal("oracle algebra entry")
	}
	registry, ok := relationoracle.NewAlgebraRegistry([]relationoracle.AlgebraEntry{entry})
	if !ok {
		t.Fatal("oracle algebra registry")
	}
	return registry
}

func assertLogicalEqual(t *testing.T, want, got relationoracle.Relation) {
	t.Helper()
	if !want.Available() || !got.Available() || want.ID() != got.ID() {
		t.Fatalf("logical relation availability/id want=%v/%v got=%v/%v", want.Available(), want.ID(), got.Available(), got.ID())
	}
	wantRows, gotRows := want.Rows(), got.Rows()
	if len(wantRows) != len(gotRows) {
		t.Fatalf("logical row count want=%d got=%d", len(wantRows), len(gotRows))
	}
	for _, wantRow := range wantRows {
		gotRow, ok := got.Row(wantRow.ID())
		if !ok {
			t.Fatalf("logical row %v missing", wantRow.ID())
		}
		wantCells, gotCells := wantRow.Cells(), gotRow.Cells()
		if len(wantCells) != len(gotCells) {
			t.Fatalf("row %v cell count want=%d got=%d", wantRow.ID(), len(wantCells), len(gotCells))
		}
		for _, wantCell := range wantCells {
			gotCell, ok := gotRow.Cell(wantCell.Column())
			if !ok || gotCell.Type() != wantCell.Type() || gotCell.Presence() != wantCell.Presence() {
				t.Fatalf("row %v column %v mismatch", wantRow.ID(), wantCell.Column())
			}
			wantValue, wantOK := wantCell.Value()
			gotValue, gotOK := gotCell.Value()
			if wantOK != gotOK || wantOK && !wantValue.Equal(gotValue) {
				t.Fatalf("row %v column %v value mismatch", wantRow.ID(), wantCell.Column())
			}
		}
	}
}

func TestApplyPublishDifferentialPreservesAllTerminalOutcomes(t *testing.T) {
	fixture := newFixture(t)
	layout := payloadLayout(t, fixture)
	registry := oracleRegistry(t, fixture.typeID)
	base := logicalState(t, fixture.aggregate, fixture, layout)

	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		t.Run(fmt.Sprintf("outcome-%d", code), func(t *testing.T) {
			application := fixture.application(t, code)
			_, hasBatch := application.Proposals()
			if code == outcome.Refused && hasBatch || code != outcome.Refused && !hasBatch {
				t.Fatalf("outcome=%v batch-presence=%v", code, hasBatch)
			}
			settlement := fixture.door.Publish(fixture.aggregate, nil, application, witness.WideningPermit{})
			if !settlement.Available() || settlement.Outcome().Code != code || settlement.Changed() {
				t.Fatalf("outcome=%v settlement=%v changed=%v", code, settlement.Available(), settlement.Changed())
			}
			got := logicalState(t, settlement.Next(), fixture, layout)
			assertLogicalEqual(t, relationoracle.Publish(base, base, registry), got)
		})
	}

	fixture.worker.result = outcome.Result{Code: outcome.Produced}
	application, ok := apply.Apply(fixture.mounted, fixture.operationIdentity(), fixture.scope, fixture.lineage, binding.NewOwnerNamedDestination(fixture.row.Relation()), fixture.inputSlot(t))
	if !ok {
		t.Fatal("produced application")
	}
	settlement := fixture.door.Publish(fixture.aggregate, fixture.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() || settlement.Outcome().Code != outcome.Produced {
		t.Fatal("produced settlement")
	}
	proposals := oracleProposals(t, application, fixture.row.Relation(), fixture.typeID)
	want := relationoracle.Publish(base, proposals, registry)
	assertLogicalEqual(t, want, logicalState(t, settlement.Next(), fixture, layout))
	if delta, ok := settlement.Delta(); !ok || !delta.Available() || !delta.Next().Same(settlement.Next()) {
		t.Fatal("produced settlement lost its database delta")
	}
}

func TestPublishDifferentialMultiRowDeltaIsFullRecomputation(t *testing.T) {
	fixture := newMultiFixture(t)
	layout := payloadLayout(t, fixture)
	registry := oracleRegistry(t, fixture.typeID)
	base := logicalState(t, fixture.aggregate, fixture, layout)
	application := fixture.application(t, outcome.Produced)
	settlement := fixture.door.Publish(fixture.aggregate, fixture.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() {
		t.Fatal("multi-row publication")
	}
	proposals := oracleProposals(t, application, fixture.row.Relation(), fixture.typeID)
	want := relationoracle.Publish(base, proposals, registry)
	got := logicalState(t, settlement.Next(), fixture, layout)
	assertLogicalEqual(t, want, got)
	if len(got.Rows()) != len(fixture.rows) {
		t.Fatalf("multi-row commit rows=%d want=%d", len(got.Rows()), len(fixture.rows))
	}
	if delta, ok := settlement.Delta(); !ok || !delta.Available() || !delta.Base().Same(fixture.aggregate) || !delta.Next().Same(settlement.Next()) {
		t.Fatal("multi-row commit did not expose an exact aggregate delta")
	}

	invalid := newMultiFixture(t)
	invalidApplication := invalid.application(t, outcome.Produced)
	// The application owns one lineage for the whole invocation. There is no
	// caller-side second sidecar whose mismatch could partially publish.
	if rejected := invalid.door.Publish(invalid.aggregate, invalid.readScratch, invalidApplication, witness.WideningPermit{}); !rejected.Available() {
		t.Fatal("application-owned multi-row lineage was rejected")
	}
	if invalid.aggregate.Revision() != 1 {
		t.Fatal("atomic rejection changed the predecessor root")
	}
}

func TestPublishDifferentialRejectsBoundedForeignAndStaleLeases(t *testing.T) {
	overflow := newBoundedOverflowFixture(t)
	overflowApplication := overflow.application(t, outcome.Produced)
	overflowSettlement := overflow.door.Publish(overflow.aggregate, nil, overflowApplication, witness.WideningPermit{})
	if !overflowSettlement.Available() || overflowSettlement.Outcome().Code != outcome.Refused || overflowSettlement.Changed() {
		t.Fatal("bounded proposal overflow was not a semantic refusal")
	}

	stale := newFixture(t)
	staleApplication := stale.application(t, outcome.Produced)
	if stale.worker.buffer == nil || !stale.worker.buffer.Reset() {
		t.Fatal("could not invalidate proposal lease")
	}
	if rejected := stale.door.Publish(stale.aggregate, stale.readScratch, staleApplication, witness.WideningPermit{}); rejected.Available() {
		t.Fatal("stale proposal lease crossed publication")
	}

	local := newFixture(t)
	foreign := newFixtureAt(t, 0x75)
	foreignApplication := foreign.application(t, outcome.Produced)
	if rejected := local.door.Publish(local.aggregate, local.readScratch, foreignApplication, witness.WideningPermit{}); rejected.Available() {
		t.Fatal("foreign proposal lease crossed publication")
	}
}
