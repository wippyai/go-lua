package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	"github.com/wippyai/go-lua/domain/type/typecontract"
)

func placementHeapFixture(t testing.TB) heap.Schema {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "placement-owner-law.lua", Text: []byte("local child = { value = 1 }; return { child = child, alias = child }")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: typecontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []project.Module{{Name: "placement-owner-law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := composite.Build()
	if !ok {
		t.Fatal("program schema receipt")
	}
	shard, ok := linked.Project().Mounts().At(0)
	if !ok {
		t.Fatal("mounted shard")
	}
	mounted, ok := linked.Project().Mounts().Program(shard)
	if !ok || mounted == nil {
		t.Fatal("mounted program")
	}
	module, ok := linked.Project().ModuleKey(shard)
	if !ok {
		t.Fatal("module key")
	}
	_, ok = linked.Project().Mounts().ProgramID(shard)
	if !ok {
		t.Fatal("program id")
	}
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	if !grammarOK || !issuanceOK {
		t.Fatal("artifact compiler inputs")
	}
	artifact, failure := artifactcompiler.CompileDetailed(mounted, grammar, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("compile artifact: %v", failure)
	}
	mount, ok := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !ok {
		t.Fatal("heap artifact mount")
	}
	schema, sealFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	if sealFailure != heap.SealFailureNone || !schema.Valid() {
		t.Fatalf("heap seal: %v", sealFailure)
	}
	return schema
}

func placementOwnerFixture(t testing.TB) (placement.Schema, axis.Algebra[placement.Fact]) {
	t.Helper()
	heaps := placementHeapFixture(t)
	projected, ok := placement.NewSchema(heaps)
	if !ok {
		t.Fatal("placement schema")
	}
	builder := engine.NewSchema()
	semantic, ok := vocabulary.Key("factor/placement")
	if !ok {
		t.Fatal("placement semantic")
	}
	fold, ok := vocabulary.Key("factor/placement/summary-coordinatewise")
	if !ok {
		t.Fatal("placement fold semantic")
	}
	fragment, ok := placementowner.DeclareSchema(builder, semantic, fold)
	if !ok {
		t.Fatal("placement schema fragment")
	}
	sealed, ok := builder.Seal()
	if !ok {
		t.Fatal("placement engine schema")
	}
	binding := engine.NewSchemaBinding(sealed)
	hot, ok := placementowner.BindHot(binding, fragment, projected)
	if !ok {
		t.Fatal("placement hot owner")
	}
	algebra, ok := placementowner.AlgebraAxis(hot)
	if !ok {
		t.Fatal("placement axis algebra")
	}
	return projected, algebra
}

func TestPlacementSchemaKeepsHeapOwnerFenceAndOrdinal(t *testing.T) {
	heaps := placementHeapFixture(t)
	projected, ok := placement.NewSchema(heaps)
	if !ok || !projected.Valid() || projected.Heap().ContentID() != heaps.ContentID() {
		t.Fatal("placement schema lost its Heap owner fence")
	}
	if _, ok := placement.NewSchema(heap.Schema{}); ok {
		t.Fatal("Placement admitted an unavailable Heap schema")
	}
	if projected.KeyCount() != heaps.AllocationKeyCount() || projected.DenseKeyCount() != heaps.AllocationKeyCount() {
		t.Fatalf("placement dense count=%d/%d, heap allocations=%d", projected.KeyCount(), projected.DenseKeyCount(), heaps.AllocationKeyCount())
	}
	for index := 0; index < heaps.AllocationKeyCount(); index++ {
		left, leftOK := projected.KeyAt(index)
		right, rightOK := heaps.AllocationKeyAt(index)
		if !leftOK || !rightOK || left.Kind() != right.Kind() {
			t.Fatalf("placement ordinal %d diverged from Heap", index)
		}
		leftID, leftIDOK := left.ContentID()
		rightID, rightIDOK := right.ContentID()
		if !leftIDOK || !rightIDOK || leftID != rightID {
			t.Fatalf("placement ordinal %d issued a second or foreign identity", index)
		}
	}
}

func TestPlacementEmptyHeapKeepsZeroWidthFactorAndSummary(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "placement-owner-empty-law.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: typecontract.NewSemantics()})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []project.Module{{Name: "placement-owner-empty-law", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, receiptOK := composite.Build()
	grammar := receipt.ExecutionSchemaID()
	grammarOK := grammar.Available()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(receipt)
	artifact, artifactFailure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if !receiptOK || !grammarOK || !issuanceOK || artifactFailure.Available() || artifact == nil {
		t.Fatalf("empty Placement artifact receipt=%t grammar=%t issuance=%t failure=%v artifact=%v", receiptOK, grammarOK, issuanceOK, artifactFailure, artifact)
	}
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !shardOK || !moduleOK || !programIDOK || !mountOK {
		t.Fatalf("empty Placement mount shard=%t module=%t program=%t mount=%t", shardOK, moduleOK, programIDOK, mountOK)
	}
	heaps, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	projected, projectedOK := placement.NewSchema(heaps)
	if heapFailure != heap.SealFailureNone || !projectedOK || projected.KeyCount() != 0 {
		t.Fatalf("empty Placement authority heap=%v placement=%t keys=%d", heapFailure, projectedOK, projected.KeyCount())
	}

	builder := engine.NewSchema()
	semantic, semanticOK := vocabulary.Key("factor/placement")
	fold, foldOK := vocabulary.Key("factor/placement/summary-coordinatewise")
	fragment, declared := placementowner.DeclareSchema(builder, semantic, fold)
	sealed, sealedOK := builder.Seal()
	if !semanticOK || !foldOK || !declared || !sealedOK {
		t.Fatal("empty Placement declaration")
	}
	binding := engine.NewSchemaBinding(sealed)
	hot, bound := placementowner.BindHot(binding, fragment, projected)
	if !bound {
		t.Fatal("empty Placement factor binding")
	}
	spec, specOK := hot.FactorSpec()
	algebra, algebraOK := placementowner.AlgebraAxis(hot)
	if !specOK || spec.KeyEnd != 0 || !algebraOK || algebra.KeyEnd != 0 {
		t.Fatalf("empty Placement width spec=%d/%t algebra=%d/%t", spec.KeyEnd, specOK, algebra.KeyEnd, algebraOK)
	}
	if spec.AdmitAt(0, placement.BottomFact()) || algebra.AdmitAt(0, placement.BottomFact()) {
		t.Fatal("empty Placement factor admitted a phantom coordinate")
	}

	observation := placement.BeginPlacementSummary(projected)
	observation, observed := placement.AccumulatePlacementSummaryRows(projected, observation, 0, func(int) (placement.Fact, bool, bool) {
		t.Fatal("empty Placement summary addressed a coordinate")
		return placement.BottomFact(), false, false
	})
	if !observed || !observation.Valid || len(observation.Values) != 0 || len(observation.Present) != 0 || observation.Rows != 0 {
		t.Fatalf("empty Placement summary = %#v/%t", observation, observed)
	}
}

func TestPlacementFactorContainsOnlyAllocationsWithStackDefault(t *testing.T) {
	projected, algebra := placementOwnerFixture(t)
	foundAllocation := false
	for index := 0; index < projected.KeyCount(); index++ {
		key, ok := projected.KeyAt(index)
		if !ok || key.Kind() != heap.RootAllocation {
			t.Fatalf("placement ordinal %d is not an allocation root", index)
		}
		foundAllocation = true
		if algebra.AdmitAt(uint64(index), placement.BottomFact()) ||
			algebra.AdmitAt(uint64(index), placement.Fact{Class: placement.Interpreter, RetainEscape: placement.EvidenceRefuted}) ||
			algebra.AdmitAt(uint64(index), placement.Fact{Class: placement.Register, RetainEscape: placement.EvidenceRefuted}) {
			t.Fatalf("allocation ordinal %d admitted a non-placement value", index)
		}
	}
	if !foundAllocation {
		t.Fatal("fixture has no allocation root")
	}
	if algebra.Default != placement.DefaultFact() {
		t.Fatalf("Placement default = %v, want Stack", algebra.Default)
	}
}

func TestPlacementLatticeAndOneComponentRank(t *testing.T) {
	projected, algebra := placementOwnerFixture(t)
	if projected.KeyCount() == 0 {
		t.Fatal("placement fixture has no dense coordinates")
	}
	allocationIndex := -1
	for index := 0; index < projected.KeyCount(); index++ {
		key, ok := projected.KeyAt(index)
		if ok && key.Kind() == heap.RootAllocation {
			allocationIndex = index
			break
		}
	}
	if allocationIndex < 0 {
		t.Fatal("placement fixture has no allocation coordinate")
	}
	values := []placement.Fact{
		placement.DefaultFact(),
		{Class: placement.OwnedHeap, RetainEscape: placement.EvidenceRefuted},
		{Class: placement.SharedHeap, RetainEscape: placement.EvidenceRefuted},
		placement.UnknownFact(),
	}
	wantRanks := []uint64{3, 2, 1, 0}
	for index, value := range values {
		if !algebra.AdmitAt(uint64(allocationIndex), value) {
			t.Fatalf("allocation coordinate rejected %v", value)
		}
		if rank := algebra.Widen.At(uint64(allocationIndex), value, 0); rank != wantRanks[index] {
			t.Fatalf("rank(%v) = %d, want %d", value, rank, wantRanks[index])
		}
	}
	lattice := placement.FactLattice()
	if lattice.Bottom() != placement.BottomFact() || lattice.Top() != placement.UnknownFact() {
		t.Fatal("placement lattice endpoints")
	}
	for beforeIndex, before := range values {
		for afterIndex, after := range values {
			if !lattice.LessOrEq(before, after) {
				continue
			}
			if beforeIndex == afterIndex || lattice.Equal(before, after) {
				continue
			}
			beforeRank := algebra.Widen.At(uint64(allocationIndex), before, 0)
			afterRank := algebra.Widen.At(uint64(allocationIndex), after, 0)
			if afterRank >= beforeRank {
				t.Fatalf("strict ascent %v -> %v did not descend: %d -> %d", before, after, beforeRank, afterRank)
			}
			if widened := lattice.Widen(before, after); !lattice.Equal(widened, after) {
				t.Fatalf("placement Widen(%v, %v) = %v, want %v", before, after, widened, after)
			}
		}
	}
	for _, polarity := range []placement.EvidenceState{placement.EvidenceRefuted, placement.EvidenceProven} {
		fact := placement.Fact{Class: placement.Stack, RetainEscape: polarity}
		if rank := algebra.Widen.At(uint64(allocationIndex), fact, 1); rank != 1 {
			t.Fatalf("retain rank(%v) = %d, want 1", polarity, rank)
		}
	}
	if rank := algebra.Widen.At(uint64(allocationIndex), placement.UnknownFact(), 1); rank != 0 {
		t.Fatalf("retain rank(unknown) = %d, want 0", rank)
	}
}
