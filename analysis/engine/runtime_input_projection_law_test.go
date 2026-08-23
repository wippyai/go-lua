package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

// TestRuntimeInputProjectionIsDenseAndGenerationFenced proves that every
// Group port has exactly one sealed source address and that the address cannot
// be reused under another mounted execution generation. The test deliberately
// exercises the real sealed read-lane program; it does not manufacture a graph
// point or a state plane for the projection.
func TestRuntimeInputProjectionIsDenseAndGenerationFenced(t *testing.T) {
	lane := newReadLaneFixture(t)
	solver, failure, sealed := lane.program.Seal(nil)
	if !sealed || solver == nil {
		t.Fatalf("seal read-lane solver failure=%v", failure)
	}
	runtime := solver.runtime
	if runtime == nil || runtime.graph == nil || runtime.program == nil || runtime.executionPlan == nil {
		t.Fatal("sealed runtime input projection plane")
	}
	found := false
	for _, producer := range runtime.producers {
		if producer.group.InputCount() == 0 {
			continue
		}
		found = true
		if len(producer.inputProjection) != producer.group.InputCount() {
			t.Fatalf("projection width=%d input width=%d", len(producer.inputProjection), producer.group.InputCount())
		}
		for index, projection := range producer.inputProjection {
			if !projection.validFor(runtime.graph, runtime.executionPlan.Generation()) {
				t.Fatalf("port %d lost sealed source identity", index)
			}
			foreignGeneration := runtime.executionPlan.Generation().Next()
			if projection.validFor(runtime.graph, foreignGeneration) {
				t.Fatalf("port %d accepted foreign generation", index)
			}
			foreign := projection
			foreign.sourceComposition = runtime.graph.CompositionID()
			foreign.sourceComposition[0] ^= 0xff
			if foreign.validFor(runtime.graph, runtime.executionPlan.Generation()) {
				t.Fatalf("port %d accepted foreign source composition", index)
			}
		}
	}
	if !found {
		t.Fatal("read-lane fixture has no dense input Group")
	}
}

// TestRuntimeInputProjectionRejectsForeignFactorMetadata proves that a
// malformed or foreign CompiledRule factor cannot be represented as a valid
// sealed port. This is the factor half of the same source-address fence; no
// fallback factor is inferred from the output or carrier slot.
func TestRuntimeInputProjectionRejectsForeignFactorMetadata(t *testing.T) {
	lane := newReadLaneFixture(t)
	solver, failure, sealed := lane.program.Seal(nil)
	if !sealed || solver == nil {
		t.Fatalf("seal read-lane solver failure=%v", failure)
	}
	runtime := solver.runtime
	if runtime == nil || runtime.graph == nil || runtime.executionPlan == nil {
		t.Fatal("sealed runtime factor plane")
	}
	for _, producer := range runtime.producers {
		if len(producer.inputProjection) == 0 {
			continue
		}
		projection := producer.inputProjection[0]
		projection.readFactorPresent = true
		projection.readFactor = ^uint32(0)
		projection.readFactorKey = composition.Key{}
		if projection.validFor(runtime.graph, runtime.executionPlan.Generation()) {
			t.Fatal("foreign factor metadata accepted")
		}
		return
	}
	t.Fatal("read-lane fixture has no dense input projection")
}

// TestRuntimeInputProjectionTransportPreservesRequiredAndSiblingRoots proves
// that the sealed factor is consumed at the one carrier transport boundary.
// TransportPointState carries the complete opaque root vector; the required
// root and an unrelated sibling therefore remain the same readable roots
// after the source reindex. A foreign or missing factor tuple is refused
// before transport. Exact authored-target removal is intentionally absent
// here; that remains the ContributionPlan.SealCarryExclusions cut.
func TestRuntimeInputProjectionTransportPreservesRequiredAndSiblingRoots(t *testing.T) {
	lane := newReadLaneFixture(t)
	solver, failure, sealed := lane.program.Seal(nil)
	if !sealed || solver == nil {
		t.Fatalf("seal read-lane solver failure=%v", failure)
	}
	runtime := solver.runtime
	if runtime == nil || runtime.program == nil || runtime.executionPlan == nil {
		t.Fatal("sealed runtime transport plane")
	}
	required, requiredOK := runtime.program.factorRecordAt(0)
	sibling, siblingOK := runtime.program.factorRecordAt(1)
	if !requiredOK || !siblingOK || required.slot < 0 || sibling.slot < 0 || required.slot == sibling.slot {
		t.Fatal("read-lane fixture lacks two distinct factor slots")
	}
	epoch, opened := newRuntimeEpoch(runtime, solver.relation, context.Background())
	if !opened || epoch == nil {
		t.Fatal("open runtime epoch")
	}
	defer epoch.discard()

	for rowIndex, row := range runtime.producerRows.rows {
		producer := runtime.producers[row.group]
		if len(producer.inputProjection) == 0 {
			continue
		}
		projection := append([]runtimeInputProjection(nil), producer.inputProjection...)
		projection[0].readFactorPresent = true
		projection[0].readFactor = 0
		projection[0].readFactorKey = required.key
		projection[0].readFactorSlot = required.slot
		bound := producer
		bound.inputProjection = projection
		cache := &epoch.producers[rowIndex]
		stateIndex, sourceOK := epoch.producerInputSourceState(&bound, cache, 0)
		if !sourceOK || stateIndex < 0 || stateIndex >= len(epoch.points) {
			t.Fatalf("row %d source state", rowIndex)
		}
		source := epoch.points[stateIndex]
		beforeRequired, beforeRequiredOK := source.HandleAt(required.slot)
		beforeSibling, beforeSiblingOK := source.HandleAt(sibling.slot)
		if !beforeRequiredOK || !beforeSiblingOK {
			t.Fatalf("row %d source lost required/sibling root", rowIndex)
		}
		transported, transportedOK := epoch.transportProducerInput(&bound, cache, 0)
		if !transportedOK {
			t.Fatalf("row %d sealed transport refused", rowIndex)
		}
		afterRequired, afterRequiredOK := transported.HandleAt(required.slot)
		afterSibling, afterSiblingOK := transported.HandleAt(sibling.slot)
		if !afterRequiredOK || !afterSiblingOK || afterRequired != beforeRequired || afterSibling != beforeSibling {
			t.Fatalf("row %d transport changed required/sibling root", rowIndex)
		}

		foreign := projection[0]
		foreign.readFactorKey = sibling.key
		if foreign.factorOwnedBy(runtime.program) {
			t.Fatalf("row %d accepted foreign factor key", rowIndex)
		}
		missing := projection[0]
		missing.readFactor = ^uint32(0)
		missing.readFactorKey = composition.Key{}
		missing.readFactorSlot = shape.Slot(-1)
		if missing.factorOwnedBy(runtime.program) {
			t.Fatalf("row %d accepted missing factor", rowIndex)
		}
		return
	}
	t.Fatal("read-lane fixture has no dense input projection")
}
