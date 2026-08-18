package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func TestSolveSealsPrivatePointStateSnapshot(t *testing.T) {
	solver, _, state := newBorrowedQueryFixture(t)
	sealed, sealedOK := solver.PublishedSnapshot(state)
	published := sealed.Snapshot()
	if !sealedOK || !state.solved.pointAxis.Available() {
		t.Fatal("completed solve did not seal a point-state axis")
	}
	if published.Schema() != state.solved.schema || state.solved.pointAxis.SchemaID != published.Schema() {
		t.Fatal("the point-state axis is not on the sealed snapshot")
	}
	if published.Columns() != solvedStoreColumns || published.Queries().Len() != solvedAxisCount {
		t.Fatalf("store columns/queries = %d/%d, want %d/%d", published.Columns(), published.Queries().Len(), solvedStoreColumns, solvedAxisCount)
	}
	if solver.runtime == nil || solver.runtime.graph == nil {
		t.Fatal("the fixture solver has no graph")
	}
	count := 0
	for index := 0; index < solver.runtime.graph.PointCount(); index++ {
		if !solver.runtime.activePoints[index] {
			continue
		}
		point, ok := solver.runtime.graph.PointAt(schedule.Node(index))
		if !ok || !point.Key().Available() {
			t.Fatalf("active point %d does not resolve", index)
		}
		key := point.Key()
		held, readable := readPointState(published, key)
		if !readable || !held.Valid() {
			t.Fatalf("point %x is not a published point state", key.ID[:4])
		}
		count++
	}
	if count == 0 {
		t.Fatal("the fixture declared no active points")
	}
}

func TestNewDeltaInheritsSealedPointState(t *testing.T) {
	solver, _, state := newBorrowedQueryFixture(t)
	sealed, sealedOK := solver.PublishedSnapshot(state)
	published := sealed.Snapshot()
	if !sealedOK || !solver.lastSolved.pointAxis.Available() {
		t.Fatal("completed solve did not seal a point-state axis")
	}
	var key composition.Key
	for index := 0; index < solver.runtime.graph.PointCount(); index++ {
		if !solver.runtime.activePoints[index] {
			continue
		}
		point, ok := solver.runtime.graph.PointAt(schedule.Node(index))
		if !ok || !point.Key().Available() {
			t.Fatalf("active point %d does not resolve", index)
		}
		key = point.Key()
		break
	}
	if !key.Available() {
		t.Fatal("the fixture declared no active points")
	}
	held, readable := readPointState(published, key)
	if !readable || !held.Valid() {
		t.Fatalf("point %x is not a published point state", key.ID[:4])
	}
	epoch, epochOK := newRuntimeEpoch(solver.runtime, solver.relation, context.Background())
	if !epochOK {
		t.Fatal("new epoch")
	}
	defer epoch.discard()
	generation := solver.completion.Next()
	if !generation.Available() {
		t.Fatal("no successor generation")
	}
	publication, opened := beginSolvedPublication(solver, epoch, generation)
	if !opened || publication == nil {
		t.Fatal("begin solved publication")
	}
	if publication.plan == nil || !canDeltaSolvedPublication(solver.lastSolved, publication.solved.schema, solver.store, generation, publication.plan.queryKeys, publication.plan.observationKeys, publication.plan.pointMembers) {
		t.Fatal("successor generation is not a delta")
	}
	overlay, status := snapshot.ReadOverlay(&publication.builder, publication.pointAxis, key)
	if status != snapshot.ReadHit || !overlay.Valid() {
		t.Fatalf("delta overlay = %v valid=%t, want inherited hit", status, overlay.Valid())
	}
	if epoch.work.OwnsPointState(overlay) {
		t.Fatal("delta overlay holds this epoch's initial point state")
	}
}
