package heap_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	fresh "github.com/wippyai/go-lua/domain/heap/internal/fresh"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

func TestHeapFreshOwnerEnumerationIsStableAndKeyIDRoundTrips(t *testing.T) {
	linked, schema, mounts := compactHeapFixture(t, "fresh-owner-enumeration", `return selected(1)`, compactFreshSpec(
		compactFreshOperation("left", schematype.FreshClassTable),
		compactFreshOperation("right", schematype.FreshClassFunction),
	))
	if got := schema.FreshCount(); got != 2 {
		t.Fatalf("FreshCount()=%d, want 2", got)
	}

	target, targetOK := linked.Boundary().Target()
	if !targetOK || target == nil {
		t.Fatal("fixture omitted sealed Target contract")
	}
	want := make([]struct {
		result identity.ContentID
		kinds  runtimekind.Set
	}, 0, target.Operations.OperationCount())
	for operationIndex := 0; operationIndex < target.Operations.OperationCount(); operationIndex++ {
		operation, operationOK := target.Operations.OperationAt(operationIndex)
		if !operationOK {
			t.Fatalf("OperationAt(%d) failed", operationIndex)
		}
		result, _, kind, freshOK := target.Operations.FreshResultAt(operation, 0, 0)
		outcomeResult, outcomeResultOK := target.OutcomeResultID(operation, 0, int(result))
		mapped, mappedOK := fresh.KindsFor(kind)
		if !freshOK || !outcomeResultOK || !mappedOK {
			t.Fatalf("fresh operation %d did not expose an exact Target result", operationIndex)
		}
		want = append(want, struct {
			result identity.ContentID
			kinds  runtimekind.Set
		}{outcomeResult, mapped})
	}

	for index, expected := range want {
		id, key, keyOK := schema.FreshAt(index)
		if !keyOK || !key.Valid() || id == (identity.ContentID{}) {
			t.Fatalf("FreshAt(%d)=%v/%v/%v", index, id, key, keyOK)
		}
		if got, idOK := key.ContentID(); !idOK || got != id {
			t.Fatalf("FreshAt(%d) KeyID=%v/%v, owner id=%v", index, got, idOK, id)
		}
		application, outcomeResult, ordinal, freshOK := key.FreshResultID()
		if !freshOK || !application.Available() || outcomeResult != expected.result || ordinal != 0 {
			t.Fatalf("FreshAt(%d) FreshResultID=%v/%v/%d/%v, want exact %v/0", index, application, outcomeResult, ordinal, freshOK, expected.result)
		}
		reference, referenceOK := schema.Reference(key, materialization.Recent)
		selector, selectorOK := schema.ReferenceSelector(reference)
		if !referenceOK || !selectorOK || selector.RuntimeKinds() != expected.kinds {
			t.Fatalf("FreshAt(%d) kinds=%b/%v, want %b", index, selector.RuntimeKinds(), selectorOK, expected.kinds)
		}
		inverse, inverseOK := schema.KeyForID(id)
		if !inverseOK || inverse != key {
			t.Fatalf("FreshAt(%d) KeyForID(%v)=%v/%v, want owner key", index, id, inverse, inverseOK)
		}
	}
	if _, _, ok := schema.FreshAt(-1); ok {
		t.Fatal("FreshAt accepted a negative index")
	}
	if _, _, ok := schema.FreshAt(schema.FreshCount()); ok {
		t.Fatal("FreshAt accepted the count boundary")
	}

	rebound, failure := heapdomain.SealWithArtifacts(linked, mounts)
	if failure != heapdomain.SealFailureNone {
		t.Fatalf("re-sealing fixture failed: %v", failure)
	}
	if rebound.FreshCount() != schema.FreshCount() {
		t.Fatalf("re-sealed FreshCount=%d, want %d", rebound.FreshCount(), schema.FreshCount())
	}
	for index := 0; index < schema.FreshCount(); index++ {
		id, key, ok := schema.FreshAt(index)
		reboundID, reboundKey, reboundOK := rebound.FreshAt(index)
		if !ok || !reboundOK || id != reboundID {
			t.Fatalf("fresh order changed at %d: %v/%v versus %v/%v", index, id, ok, reboundID, reboundOK)
		}
		leftApplication, leftResult, leftOrdinal, leftFresh := key.FreshResultID()
		rightApplication, rightResult, rightOrdinal, rightFresh := reboundKey.FreshResultID()
		if !leftFresh || !rightFresh || leftApplication != rightApplication || leftResult != rightResult || leftOrdinal != rightOrdinal {
			t.Fatalf("fresh identity changed at %d", index)
		}
	}
}
