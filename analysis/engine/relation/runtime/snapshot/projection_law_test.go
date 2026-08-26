package snapshot_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/runtime"
	projection "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
)

func TestArithmeticRootPublishesCanonicalOpaqueProjection(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("arithmetic solve")
	}
	published, ok := projection.Publish(result, fixture.View())
	if !ok || !published.Available() {
		t.Fatal("canonical projection publication")
	}
	if published.Store() != fixture.Mounted().Fence().StoreID() || published.Generation() != identity.Generation(result.Root().Revision()) {
		t.Fatalf("publication fence = (%d, %d), want (%d, %d)", published.Store(), published.Generation(), fixture.Mounted().Fence().StoreID(), result.Root().Revision())
	}

	ids := fixture.IDs()
	keys := published.Keys(ids.OutputWrite)
	if len(keys) != 2 {
		t.Fatalf("output denominator keys = %d, want 2", len(keys))
	}
	want := fixture.Expected()
	seen := make(map[model.RowID]identity.ContentID, len(keys))
	for _, key := range keys {
		if !key.Available() || key.Relation != ids.Output || key.Row.Relation() != ids.Output {
			t.Fatalf("invalid projected output key: %#v", key)
		}
		cell, status := published.Read(ids.OutputWrite, key)
		if status != canonical.ReadHit || !cell.Available() || !cell.ValidFor(result.Root().Fence()) {
			t.Fatalf("output cell status=%s available=%v valid=%v", status, cell.Available(), cell.ValidFor(result.Root().Fence()))
		}
		if cell.Column != ids.OutputWrite || cell.Type != ids.Type || !cell.Value.Available() {
			t.Fatalf("output cell metadata: column=%v type=%v value=%v", cell.Column, cell.Type, cell.Value.Available())
		}
		seen[key.Row] = cell.Value.Opaque()
	}
	for row, wantOpaque := range want {
		if got, ok := seen[row]; !ok || got != wantOpaque {
			t.Fatalf("row %v opaque=%v present=%v, want %v", row, got, ok, wantOpaque)
		}
	}
}

// TestArithmeticObservationRedeemsTerminalApplyByExactKey proves the
// observation side is downstream of the same immutable terminal result as the
// relation projection.  The arithmetic Publish root carries a child Apply;
// Snapshot must redeem that retained extent by the sealed
// (Dependency, Operation) key and preserve the child Output row identities.
func TestArithmeticObservationRedeemsTerminalApplyByExactKey(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("arithmetic solve")
	}
	relations, ok := projection.Publish(result, fixture.View())
	if !ok || !relations.Available() {
		t.Fatal("canonical relation projection")
	}
	observed, ok := projection.Project(result, relations.Snapshot(), fixture.Observation())
	if !ok || !observed.Available() {
		t.Fatal("terminal observation projection")
	}

	ids := fixture.IDs()
	keys := observed.Keys()
	if len(keys) != 2 {
		t.Fatalf("observation parent keys=%d, want 2", len(keys))
	}
	wantRows := []model.RowID{ids.SourceA, ids.SourceB}
	wantDestinations := []model.RowID{ids.OutputA, ids.OutputB}
	for index, key := range keys {
		if key.Relation != ids.Source || key.Row != wantRows[index] {
			t.Fatalf("observation key %d relation/row mismatch", index)
		}
		value, status := observed.Read(key)
		if status != canonical.ReadHit || !value.Available() || value.Outcome().Code != outcome.Produced {
			t.Fatalf("observation read %d status=%s available=%v outcome=%v", index, status, value.Available(), value.Outcome().Code)
		}
		outputs := value.Outputs()
		if len(outputs) != 2 || len(value.OutputsFor(ids.OutputAddress)) != 1 || len(value.OutputsFor(ids.OutputWrite)) != 1 {
			t.Fatalf("observation outputs %d total=%d address=%d write=%d", index, len(outputs), len(value.OutputsFor(ids.OutputAddress)), len(value.OutputsFor(ids.OutputWrite)))
		}
		for outputIndex, output := range outputs {
			wantColumn := []model.ColumnID{ids.OutputAddress, ids.OutputWrite}[outputIndex]
			outputScope, scopeOK := fixture.Mounted().ScopeForToken(output.Destination.Scope())
			if !scopeOK || output.Column != wantColumn || output.Destination.Relation() != ids.Output || output.Destination.Row() != wantDestinations[index] || !outputScope.Same(key.Scope) {
				t.Fatalf("observation output %d:%d destination mismatch", index, outputIndex)
			}
		}
	}
}

// TestObservationProjectionRefusesForeignOrMissingTerminalKeys prevents a
// caller from selecting a raw Apply extent or a descriptor from another
// mounted result.  The only admissible input is the terminal catalog entry
// bound by the sealed dependency and operation identities.
func TestObservationProjectionRefusesForeignOrMissingTerminalKeys(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !result.Available() {
		t.Fatal("arithmetic solve")
	}
	relations, ok := projection.Publish(result, fixture.View())
	if !ok || !relations.Available() {
		t.Fatal("canonical relation projection")
	}
	foreignFamily, ok := identity.DeriveContentID("test/foreign-observation/v1")
	if !ok {
		t.Fatal("foreign observation identity")
	}
	if _, ok := projection.Project(result, relations.Snapshot(), foreignFamily); ok {
		t.Fatal("foreign observation family was accepted")
	}
	contract, ok := fixture.Mounted().Observation(fixture.Observation())
	if !ok || !contract.Available() {
		t.Fatal("arithmetic observation contract")
	}
	cleared, ok := result.Applications().ClearDependency(contract.Dependency())
	if !ok {
		t.Fatal("clear terminal observation")
	}
	missing, ok := terminal.New(result.Root(), result.Evaluations(), result.Publications(), cleared)
	if !ok || !missing.Available() {
		t.Fatal("missing-key terminal result")
	}
	if _, ok := projection.Project(missing, relations.Snapshot(), fixture.Observation()); ok {
		t.Fatal("missing terminal observation key was accepted")
	}

	foreign := arithmetic.New(t, 0xB2)
	foreignResult, ok := runtime.Solve(foreign.Mounted(), foreign.Base(), foreign.View())
	if !ok || !foreignResult.Available() {
		t.Fatal("foreign arithmetic solve")
	}
	if _, ok := projection.Project(foreignResult, relations.Snapshot(), fixture.Observation()); ok {
		t.Fatal("foreign terminal result was accepted against local snapshot")
	}

	wrongGeneration := canonical.NewBuilder(relations.Schema(), relations.Store(), relations.Generation()-1)
	wrongBase, err := wrongGeneration.Seal()
	if err != nil || !wrongBase.Published() {
		t.Fatalf("wrong-generation base: %v", err)
	}
	if _, ok := projection.Project(result, wrongBase, fixture.Observation()); ok {
		t.Fatal("same-store older snapshot was accepted")
	}
	foreignStore, ok := identity.IssueStore()
	if !ok {
		t.Fatal("foreign store identity")
	}
	wrongStore := canonical.NewBuilder(relations.Schema(), foreignStore, relations.Generation())
	wrongStoreBase, err := wrongStore.Seal()
	if err != nil || !wrongStoreBase.Published() {
		t.Fatalf("wrong-store base: %v", err)
	}
	if _, ok := projection.Project(result, wrongStoreBase, fixture.Observation()); ok {
		t.Fatal("same-schema foreign snapshot was accepted")
	}
}

func TestProjectionTypedAndForeignNearestNegativesRefuse(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok {
		t.Fatal("arithmetic solve")
	}
	published, ok := projection.Publish(result, fixture.View())
	if !ok {
		t.Fatal("canonical projection publication")
	}
	ids := fixture.IDs()
	keys := published.Keys(ids.OutputWrite)
	if len(keys) == 0 {
		t.Fatal("empty output projection")
	}
	key := keys[0]
	column, ok := published.Column(ids.OutputWrite)
	if !ok {
		t.Fatal("output axis")
	}

	// The canonical store checks the static value type on recovery.  A
	// matching row key with a foreign value type is invalid, never a miss.
	sealed := published.Snapshot()
	wrongAxis := canonical.Axis[projection.RowKey, uint64]{SchemaID: published.Schema(), Slot: column.Axis().Slot}
	if _, status := canonical.Read(&sealed, wrongAxis, key); status != canonical.ReadInvalid {
		t.Fatalf("wrong value type status=%s, want invalid", status)
	}

	// A foreign mount's scope token cannot redeem a row in this publication.
	foreign := arithmetic.New(t, 0xB2)
	foreignResult, ok := runtime.Solve(foreign.Mounted(), foreign.Base(), foreign.View())
	if !ok {
		t.Fatal("foreign arithmetic solve")
	}
	foreignProjection, ok := projection.Publish(foreignResult, foreign.View())
	if !ok {
		t.Fatal("foreign canonical projection publication")
	}
	foreignKeys := foreignProjection.Keys(ids.OutputWrite)
	if len(foreignKeys) == 0 {
		t.Fatal("empty foreign output projection")
	}
	if _, status := published.Read(ids.OutputWrite, foreignKeys[0]); status != canonical.ReadInvalid {
		t.Fatalf("foreign scope status=%s, want invalid", status)
	}

	foreignContent, ok := identity.DeriveContentID("test/foreign-column/v1")
	if !ok {
		t.Fatal("foreign column content")
	}
	foreignColumn, ok := model.IssueColumnID(ids.Output, foreignContent)
	if !ok {
		t.Fatal("foreign column identity")
	}
	if _, status := published.Read(foreignColumn, key); status != canonical.ReadInvalid {
		t.Fatalf("foreign column status=%s, want invalid", status)
	}
}

func TestRefusedProjectionReadRequiresNoValue(t *testing.T) {
	fixture := arithmetic.New(t)
	result, ok := runtime.Solve(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok {
		t.Fatal("arithmetic solve")
	}
	published, ok := projection.Publish(result, fixture.View())
	if !ok {
		t.Fatal("canonical projection publication")
	}
	ids := fixture.IDs()
	column, ok := published.Column(ids.OutputWrite)
	if !ok {
		t.Fatal("output axis")
	}
	keys := published.Keys(ids.OutputWrite)
	if len(keys) == 0 {
		t.Fatal("empty output projection")
	}
	key := keys[0]
	cell, status := published.Read(ids.OutputWrite, key)
	if status != canonical.ReadHit {
		t.Fatalf("seed read status=%s", status)
	}

	reasonContent, ok := identity.DeriveContentID("test/refused-projection/v1")
	if !ok {
		t.Fatal("refusal content")
	}
	reason, ok := model.IssueRefusalID(ids.Owner, reasonContent)
	if !ok {
		t.Fatal("refusal identity")
	}
	refused, ok := model.NewRefused(reason)
	if !ok {
		t.Fatal("refused presence")
	}
	cell.Presence = refused
	cell.Value = binding.ValueToken{}
	if !cell.Available() || !cell.ValidFor(result.Root().Fence()) {
		t.Fatal("value-less refused cell must remain valid")
	}

	// Publish the exact refused cell through the canonical builder, then read
	// it back through the same typed axis used by the projection.
	builder := canonical.NewBuilder(published.Schema(), published.Store(), published.Generation()+1)
	isolatedAxis := canonical.Axis[projection.RowKey, projection.Cell]{SchemaID: published.Schema(), Slot: 0}
	content := canonical.Content[projection.RowKey, projection.Cell]{
		Rows:        map[projection.RowKey]projection.Cell{key: cell},
		Denominator: column.DenominatorID,
		Members:     []projection.RowKey{key},
	}
	if err := canonical.PutColumn(&builder, isolatedAxis, content); err != nil {
		t.Fatalf("refused column: %v", err)
	}
	if err := builder.Publish(column.PublicationID, isolatedAxis.Slot); err != nil {
		t.Fatalf("refused publication: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("refused seal: %v", err)
	}
	read, status := canonical.Read(&sealed, isolatedAxis, key)
	if status != canonical.ReadHit || !read.Available() || !read.Presence.Is(model.Refused) || read.Value.Available() {
		t.Fatalf("refused read status=%s available=%v presence=%v value=%v", status, read.Available(), read.Presence.Kind(), read.Value.Available())
	}

	// A refusal carrying a value is malformed, rather than a present value.
	cell.Value = publishedCellValue(t, published, ids.OutputWrite, key)
	if cell.Available() || cell.ValidFor(result.Root().Fence()) {
		t.Fatal("value-bearing refused cell must be rejected")
	}
}

func publishedCellValue(t *testing.T, published projection.Projection, id model.ColumnID, key projection.RowKey) binding.ValueToken {
	t.Helper()
	cell, status := published.Read(id, key)
	if status != canonical.ReadHit || !cell.Value.Available() {
		t.Fatal("projected value")
	}
	return cell.Value
}
