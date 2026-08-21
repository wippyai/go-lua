package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	packdomain "github.com/wippyai/go-lua/domain/pack"
)

// TestMountedActualProjectionKeepsAuthoredOrder exercises the canonical Pack
// bridge used by Call. The selector fixture is a method call, so receiver
// order is observable without reopening the artifact Program in the test.
func TestMountedActualProjectionKeepsAuthoredOrder(t *testing.T) {
	contract, _ := selectorLawContract(t)
	fixture := selectorLawSchema(t, contract, "mounted_actual_projection")
	projection, ok := fixture.schema.MountedActualProjection(fixture.module, fixture.callID)
	if !ok || !projection.Valid() || !projection.OwnedBy(fixture.schema) {
		t.Fatal("sealed Pack did not issue an owner-fenced mounted actual projection")
	}
	if projection.ActualCount() != 3 {
		t.Fatalf("fixed actual count = %d, want receiver plus two arguments", projection.ActualCount())
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		candidate, candidateOK := fixture.schema.MountedActualProjection(fixture.module, fixture.callID)
		if !candidateOK || candidate.ActualCount() != 3 {
			t.Fatal("mounted actual projection lookup")
		}
	}); allocations != 0 {
		t.Fatalf("mounted actual projection allocations = %v", allocations)
	}
	first, firstOK := projection.ActualAt(0)
	second, secondOK := projection.ActualAt(1)
	if !firstOK || !secondOK || first.ID() != fixture.receiver || second.ID() != fixture.argument0 {
		t.Fatal("Pack projection lost receiver-first authored order")
	}
	if _, ok := projection.ActualAt(-1); ok {
		t.Fatal("negative mounted actual index accepted")
	}
	if _, ok := projection.ActualAt(projection.ActualCount()); ok {
		t.Fatal("out-of-range mounted actual index accepted")
	}
	if tailID, hasTail := projection.TailID(); hasTail {
		if !tailID.Available() {
			t.Fatal("Pack projection fabricated an invalid tail identity")
		}
	} else if tailID != (identity.ContentID{}) {
		t.Fatal("fixed-only Pack projection exposed a tail identity")
	}
}

// TestMountedActualProjectionRejectsForeignPack proves that detached module
// and call bytes cannot redeem a projection issued by another Pack owner.
func TestMountedActualProjectionRejectsForeignPack(t *testing.T) {
	contract, _ := selectorLawContract(t)
	first := selectorLawSchema(t, contract, "mounted_actual_projection_first")
	second := selectorLawSchema(t, contract, "mounted_actual_projection_second")
	projection, ok := first.schema.MountedActualProjection(first.module, first.callID)
	if !ok || !projection.Valid() {
		t.Fatal("first Pack projection")
	}
	if second.schema.OwnsSemanticSource(projectionSource(t, projection, 0)) || projection.OwnedBy(second.schema) {
		t.Fatal("foreign Pack accepted first projection")
	}
	if _, ok := second.schema.MountedActualProjection(second.module, second.callID); !ok {
		t.Fatal("second Pack did not retain its own mounted projection")
	}
}

func projectionSource(t *testing.T, projection interface {
	ActualAt(int) (packdomain.SemanticSource, bool)
}, index int) packdomain.SemanticSource {
	t.Helper()
	source, ok := projection.ActualAt(index)
	if !ok {
		t.Fatal("projection source")
	}
	return source
}
