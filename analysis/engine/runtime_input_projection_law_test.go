package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
)

// TestRuntimeInputReadValidationRetainsNoFactorMirror keeps malformed
// descriptor geometry at Seal without putting Factor tuples on the input
// projection. The descriptor and runtime Factor table remain the only two
// authorities checked by this allocation-free pass.
func TestRuntimeInputReadValidationRetainsNoFactorMirror(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, memberOK := newGeneratedMember(generatedMemberTestSpec(t, fixture, 0, 0))
	descriptor, descriptorOK := newExactIdentityDescriptor(0, 1, 0, 0, 0, 1, 1)
	if !memberOK || !descriptorOK {
		t.Fatal("generated input validation fixture")
	}
	program := &runtimeProgram{
		memberTable:       []memberRow{{generated: member}},
		generatedPrograms: []generated.CompiledRule{descriptor},
		generatedPresent:  true,
		factorTable: []factorRecord{{
			key:   compositionKeyOf(coldKey(999_101)),
			slot:  fixture.slot,
			owner: 0,
		}},
		factorOwners:  []runtimeFactor{fixture.factor},
		programSealed: true,
	}
	span := memberSpan{start: 0, end: 1}
	if !validateRuntimeInputReads(program, span, 1) {
		t.Fatal("canonical descriptor input was refused")
	}

	if validateRuntimeInputReads(program, span, 2) {
		t.Fatal("descriptor with foreign Group input width was admitted")
	}
	program.factorTable = nil
	if validateRuntimeInputReads(program, span, 1) {
		t.Fatal("descriptor with an absent Factor row was admitted")
	}
}

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

// TestRuntimeInputProjectionTransportPreservesTheCompleteFactorProduct proves
// that one predecessor-State port transports its complete opaque root vector.
// Factor identity is not copied into the projection: the CompiledRule reads
// own those identities, while carrier owns preservation of every root.
func TestRuntimeInputProjectionTransportPreservesTheCompleteFactorProduct(t *testing.T) {
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
		cache := &epoch.producers[rowIndex]
		stateIndex, sourceOK := epoch.producerInputSourceState(&producer, cache, 0)
		if !sourceOK || stateIndex < 0 || stateIndex >= len(epoch.points) {
			t.Fatalf("row %d source state", rowIndex)
		}
		source := epoch.points[stateIndex]
		beforeRequired, beforeRequiredOK := source.HandleAt(required.slot)
		beforeSibling, beforeSiblingOK := source.HandleAt(sibling.slot)
		if !beforeRequiredOK || !beforeSiblingOK {
			t.Fatalf("row %d source lost required/sibling root", rowIndex)
		}
		transported, transportedOK := epoch.transportProducerInput(&producer, cache, 0)
		if !transportedOK {
			t.Fatalf("row %d sealed transport refused", rowIndex)
		}
		afterRequired, afterRequiredOK := transported.HandleAt(required.slot)
		afterSibling, afterSiblingOK := transported.HandleAt(sibling.slot)
		if !afterRequiredOK || !afterSiblingOK || afterRequired != beforeRequired || afterSibling != beforeSibling {
			t.Fatalf("row %d transport changed required/sibling root", rowIndex)
		}
		return
	}
	t.Fatal("read-lane fixture has no dense input projection")
}
