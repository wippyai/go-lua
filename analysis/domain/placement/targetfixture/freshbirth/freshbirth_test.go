package freshbirth

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

func TestFreshBirthTargetDeclarationCertificateMountSolveSnapshot(t *testing.T) {
	fixture := New(t)
	result, ok := fixture.World.Solve()
	if !ok || !result.Available() {
		t.Fatal("freshbirth target solve")
	}
	projection, ok := fixture.World.Snapshot(result)
	if !ok || !projection.Available() {
		t.Fatal("freshbirth target snapshot")
	}
	keys := projection.Keys(fixture.IDs.OutputPayload)
	if len(keys) != 1 {
		t.Fatalf("freshbirth output keys = %d, want one closed member", len(keys))
	}
	key := keys[0]
	if !key.Available() || key.Relation != fixture.IDs.Output || key.Row != fixture.IDs.OutputRow {
		t.Fatal("freshbirth output row identity")
	}
	cell, status := projection.Read(fixture.IDs.OutputPayload, key)
	if status != canonical.ReadHit || !cell.Available() || !cell.Presence.Is(model.Present) {
		t.Fatalf("freshbirth output status=%s available=%v presence=%s", status, cell.Available(), cell.Presence.Kind())
	}
	fact, ok := fixture.Columns.Placement.Decode(cell.Value)
	if !ok || !placementdomain.EqualFact(fact, fixture.Expected) {
		t.Fatalf("freshbirth output fact=%#v/%v, want explicit %#v", fact, ok, fixture.Expected)
	}
}

func TestFreshBirthTargetSnapshotKeepsForeignScopeOpaque(t *testing.T) {
	fixture := New(t)
	result, ok := fixture.World.Solve()
	if !ok {
		t.Fatal("freshbirth target solve")
	}
	projection, ok := fixture.World.Snapshot(result)
	if !ok {
		t.Fatal("freshbirth target snapshot")
	}
	foreign := New(t, 0xF8)
	foreignResult, ok := foreign.World.Solve()
	if !ok {
		t.Fatal("foreign freshbirth target solve")
	}
	foreignProjection, ok := foreign.World.Snapshot(foreignResult)
	if !ok {
		t.Fatal("foreign freshbirth target snapshot")
	}
	foreignKeys := foreignProjection.Keys(foreign.IDs.OutputPayload)
	if len(foreignKeys) != 1 {
		t.Fatal("foreign freshbirth output key")
	}
	if _, status := projection.Read(fixture.IDs.OutputPayload, snapshot.RowKey{
		Relation: fixture.IDs.Output,
		Row:      fixture.IDs.OutputRow,
		Scope:    foreignKeys[0].Scope,
	}); status != canonical.ReadInvalid {
		t.Fatalf("foreign scope read status=%s, want invalid", status)
	}
}
