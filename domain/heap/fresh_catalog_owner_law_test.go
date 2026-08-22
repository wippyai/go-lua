package heap_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
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
	type freshSemantic struct {
		application identity.ContentID
		result      identity.ContentID
		ordinal     uint32
	}
	type freshTemplate struct {
		operation   vocabulary.Operation
		outcome     int
		resultIndex int
		result      identity.ContentID
		ordinal     uint32
		kinds       runtimekind.Set
	}
	templates := make([]freshTemplate, 0, target.Operations.OperationCount())
	for operationIndex := 0; operationIndex < target.Operations.OperationCount(); operationIndex++ {
		operation, operationOK := target.Operations.OperationAt(operationIndex)
		if !operationOK {
			t.Fatalf("OperationAt(%d) failed", operationIndex)
		}
		for outcome := 0; outcome < target.Operations.OutcomeCount(operation); outcome++ {
			for freshIndex := 0; freshIndex < target.Operations.FreshResultCount(operation, outcome); freshIndex++ {
				result, ordinal, kind, freshOK := target.Operations.FreshResultAt(operation, outcome, freshIndex)
				outcomeResult, outcomeResultOK := target.OutcomeResultID(operation, outcome, int(result))
				mapped, mappedOK := fresh.KindsFor(kind)
				if !freshOK || !outcomeResultOK || !outcomeResult.Available() {
					t.Fatalf("fresh operation %d result %d did not expose an exact Target result", operationIndex, freshIndex)
				}
				if !mappedOK {
					continue
				}
				templates = append(templates, freshTemplate{
					operation: operation, outcome: outcome, resultIndex: int(result),
					result: outcomeResult, ordinal: ordinal, kinds: mapped,
				})
			}
		}
	}
	want := make(map[freshSemantic]runtimekind.Set)
	programs := make(map[identity.ContentID]programschema.Program, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			t.Fatal("fixture returned an unavailable artifact mount")
		}
		program := mount.Snapshot.Program()
		if !program.Available() {
			t.Fatal("fixture artifact mount returned an unavailable Program")
		}
		programs[mount.ModuleKey] = program
	}
	applications := linked.Project().Applications().Calls()
	for applicationIndex := 0; applicationIndex < applications.Count(); applicationIndex++ {
		application, applicationOK := applications.At(applicationIndex)
		applicationID, moduleID, callID, mountedOK := applications.MountedIdentity(application)
		if !applicationOK || !mountedOK || !applicationID.Available() || !moduleID.Available() || !callID.Available() {
			t.Fatalf("call application %d failed exact Project mounting", applicationIndex)
		}
		program, programOK := programs[moduleID]
		if !programOK {
			t.Fatalf("application %d has no canonical Program for module %v", applicationIndex, moduleID)
		}
		callResult, callResultOK := program.CallResultForID(callID)
		for _, template := range templates {
			if !callResultOK || !callResult.AdmitsResult(uint32(template.resultIndex)) {
				continue
			}
			if !callResult.Available() || callResult.CallID() != callID {
				t.Fatalf("application %d admitted a mismatched canonical CallResult", applicationIndex)
			}
			semantic := freshSemantic{application: applicationID, result: template.result, ordinal: template.ordinal}
			if _, duplicate := want[semantic]; duplicate {
				t.Fatalf("application %d duplicated fresh semantic tuple", applicationIndex)
			}
			want[semantic] = template.kinds
		}
	}
	if len(want) != schema.FreshCount() {
		t.Fatalf("semantic fresh set=%d, FreshCount=%d", len(want), schema.FreshCount())
	}

	var previousID identity.ContentID
	for index := 0; index < schema.FreshCount(); index++ {
		id, key, keyOK := schema.FreshAt(index)
		if !keyOK || !key.Valid() || id == (identity.ContentID{}) {
			t.Fatalf("FreshAt(%d)=%v/%v/%v", index, id, key, keyOK)
		}
		if index > 0 && bytes.Compare(previousID[:], id[:]) >= 0 {
			t.Fatalf("FreshAt IDs are not strict ascending at %d: %x then %x", index, previousID, id)
		}
		previousID = id
		if got, idOK := key.ContentID(); !idOK || got != id {
			t.Fatalf("FreshAt(%d) KeyID=%v/%v, owner id=%v", index, got, idOK, id)
		}
		application, outcomeResult, ordinal, freshOK := key.FreshResultID()
		expectedKinds, expectedOK := want[freshSemantic{application: application, result: outcomeResult, ordinal: ordinal}]
		if !freshOK || !application.Available() || !expectedOK {
			t.Fatalf("FreshAt(%d) FreshResultID=%v/%v/%d/%v lacks a Target semantic match", index, application, outcomeResult, ordinal, freshOK)
		}
		delete(want, freshSemantic{application: application, result: outcomeResult, ordinal: ordinal})
		reference, referenceOK := schema.Reference(key, materialization.Recent)
		selector, selectorOK := schema.ReferenceSelector(reference)
		if !referenceOK || !selectorOK || selector.RuntimeKinds() != expectedKinds {
			t.Fatalf("FreshAt(%d) kinds=%b/%v, want %b", index, selector.RuntimeKinds(), selectorOK, expectedKinds)
		}
		inverse, inverseOK := schema.KeyForID(id)
		if !inverseOK || inverse != key {
			t.Fatalf("FreshAt(%d) KeyForID(%v)=%v/%v, want owner key", index, id, inverse, inverseOK)
		}
	}
	if len(want) != 0 {
		t.Fatalf("FreshAt omitted %d Target semantic rows", len(want))
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
