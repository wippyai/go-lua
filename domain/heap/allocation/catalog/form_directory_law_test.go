// Two-way parity between the sealed global allocation-form directory
// (domain/heap/allocation_form_directory.go) and the per-mount catalog this
// package owns. The global directory is the successor derivation; these laws
// prove it is both a pure function of the sealed program and a total,
// duplicate-free refinement of the per-mount maps it replaces.
package catalog_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	"github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestAllocationFormDirectoryIsAFunctionOfTheSealedProgram seals the same
// program text into two independent Link/Heap seals and shows the global
// closed/empty directories are the same value both times: same counts, same
// KeyID identity at every ordinal, same owning Heap ContentID. It also shows
// the directories are schema-fenced: a key from one seal is refused by the
// other seal's ordinal lookup, so the directory is not an ambient global that
// happens to line up.
func TestAllocationFormDirectoryIsAFunctionOfTheSealedProgram(t *testing.T) {
	text := `local e = {}
local k = 1
local t = { [k] = k }
return t, e
`
	heapA, _ := formDirectoryFixture(t, "form-directory.lua", text)
	heapB, _ := formDirectoryFixture(t, "form-directory.lua", text)

	if heapA.ContentID() != heapB.ContentID() {
		t.Fatal("two independent seals of the same program text produced different Heap ContentID")
	}

	closedCount := heapA.ClosedAllocationCount()
	emptyCount := heapA.EmptyAllocationCount()
	if closedCount == 0 || emptyCount == 0 {
		t.Fatalf("fixture did not exercise both forms: closed=%d empty=%d", closedCount, emptyCount)
	}
	if got := heapB.ClosedAllocationCount(); got != closedCount {
		t.Fatalf("ClosedAllocationCount diverged across independent seals: a=%d b=%d", closedCount, got)
	}
	if got := heapB.EmptyAllocationCount(); got != emptyCount {
		t.Fatalf("EmptyAllocationCount diverged across independent seals: a=%d b=%d", emptyCount, got)
	}

	for index := 0; index < closedCount; index++ {
		keyA, okA := heapA.ClosedAllocationAt(index)
		keyB, okB := heapB.ClosedAllocationAt(index)
		if !okA || !okB {
			t.Fatalf("ClosedAllocationAt(%d) unavailable: a=%t b=%t", index, okA, okB)
		}
		idA, idAOK := heapA.KeyID(keyA)
		idB, idBOK := heapB.KeyID(keyB)
		if !idAOK || !idBOK || idA != idB {
			t.Fatalf("closed ordinal %d KeyID diverged across independent seals: a=%v(%t) b=%v(%t)", index, idA, idAOK, idB, idBOK)
		}
	}
	for index := 0; index < emptyCount; index++ {
		keyA, okA := heapA.EmptyAllocationAt(index)
		keyB, okB := heapB.EmptyAllocationAt(index)
		if !okA || !okB {
			t.Fatalf("EmptyAllocationAt(%d) unavailable: a=%t b=%t", index, okA, okB)
		}
		idA, idAOK := heapA.KeyID(keyA)
		idB, idBOK := heapB.KeyID(keyB)
		if !idAOK || !idBOK || idA != idB {
			t.Fatalf("empty ordinal %d KeyID diverged across independent seals: a=%v(%t) b=%v(%t)", index, idA, idAOK, idB, idBOK)
		}
	}

	// Cross-schema fence: a key issued by A's owner must not resolve an
	// ordinal against B's directory, even though both directories agree on
	// content.
	crossKey, crossOK := heapA.ClosedAllocationAt(0)
	if !crossOK {
		t.Fatal("cross-schema fixture key")
	}
	if _, ok := heapB.ClosedAllocationOrdinal(crossKey); ok {
		t.Fatal("ClosedAllocationOrdinal accepted a key from a foreign Heap schema")
	}
}

// TestAllocationFormDirectoriesAreTotalOverThePerMountCatalog builds the
// per-mount catalog (catalog.Seal) beside the new global directory
// over a two-module link and proves the global directory is exactly the
// union of the per-mount maps it replaces: same membership both directions,
// no duplicates, dense inverse ordinals, and correct ClosedAllocation /
// EmptyAllocation destination-projection exclusivity.
func TestAllocationFormDirectoriesAreTotalOverThePerMountCatalog(t *testing.T) {
	text := `local e = {}
local k = 1
local t = { [k] = k }
return t, e
`
	mainProgram := formDirectoryProgram(t, "form-directory-main.lua", `local second = require("second")
`+text+`, second
`)
	secondProgram := formDirectoryProgram(t, "form-directory-second.lua", text)

	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{
		{Name: "main", Program: mainProgram},
		{Name: "second", Program: secondProgram},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if linked.Project().Mounts().Count() != 2 {
		t.Fatalf("fixture mount count=%d, want 2", linked.Project().Mounts().Count())
	}

	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK {
		t.Fatal("form directory artifact receipt")
	}

	projectMounts := linked.Project().Mounts()
	heapMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	valueMounts := make([]programmount.MountedArtifact, projectMounts.Count())
	modules := make([]identity.ContentID, projectMounts.Count())
	for index := 0; index < projectMounts.Count(); index++ {
		shard, shardOK := projectMounts.At(index)
		programAtShard, programOK := projectMounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !programOK || programAtShard == nil || !moduleOK {
			t.Fatal("form directory mount shard")
		}
		artifact, failure := artifactcompiler.CompileDetailed(programAtShard, executionSchemaID, issuance)
		if failure.Available() || artifact == nil {
			t.Fatalf("form directory artifact compile: %v", failure)
		}
		var heapOK, valueOK bool
		heapMounts[index], heapOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		valueMounts[index], valueOK = programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
		if !heapOK || !valueOK {
			t.Fatal("form directory mount receipt")
		}
		modules[index] = module
	}

	heap, heapFailure := heapdomain.SealWithArtifacts(linked, heapMounts)
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("form directory heap seal: %v", heapFailure)
	}
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("form directory structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, calltest.MustSeal(t, linked, valueMounts), valueMounts, structural)
	if valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("form directory value seal: %v", valueFailure)
	}

	perMount, sealOK := catalog.Seal(heap, values, heapMounts)
	if !sealOK || perMount == nil {
		t.Fatal("per-mount catalog seal")
	}

	closedCount := heap.ClosedAllocationCount()
	emptyCount := heap.EmptyAllocationCount()
	if closedCount == 0 {
		t.Fatal("fixture produced zero closed allocations")
	}
	if emptyCount == 0 {
		t.Fatal("fixture produced zero empty allocations")
	}

	closedFromMounts := make(map[uint32]bool)
	emptyFromMounts := make(map[uint32]bool)

	for _, module := range modules {
		occurrenceMount, occurrenceMountOK := heap.OccurrenceMountForModule(module)
		if !occurrenceMountOK {
			t.Fatalf("occurrence mount for module missing")
		}
		mount, mountOK := perMount.ForMount(module)
		if !mountOK {
			t.Fatalf("per-mount catalog missing module")
		}
		for index := 0; index < occurrenceMount.AllocationCount(); index++ {
			id, _, allocationOK := occurrenceMount.AllocationAt(index)
			if !allocationOK {
				t.Fatalf("occurrence AllocationAt(%d) failed", index)
			}

			closed, closedOK := mount.ClosedForOccurrence(id)
			globalClosedKey, globalClosedOK := heap.ClosedAllocationForMountedOccurrence(module, id)
			if closedOK != globalClosedOK {
				t.Fatalf("closed membership diverged: mount=%t global=%t", closedOK, globalClosedOK)
			}
			if closedOK {
				if closed.Key() != globalClosedKey {
					t.Fatal("closed key diverged between per-mount catalog and global directory")
				}
				ordinal, ordinalOK := heap.ClosedAllocationOrdinal(globalClosedKey)
				if !ordinalOK {
					t.Fatal("closed key not present in global directory ordinal")
				}
				if closedFromMounts[ordinal] {
					t.Fatalf("closed ordinal %d observed twice across mounts", ordinal)
				}
				closedFromMounts[ordinal] = true
			}

			root, rootOK := mount.RootForOccurrence(id)
			globalEmptyKey, globalEmptyOK := heap.EmptyAllocationForMountedOccurrence(module, id)
			mountIsEmpty := rootOK && root.Form() == source.FormEmpty
			if mountIsEmpty != globalEmptyOK {
				t.Fatalf("empty membership diverged: mount=%t global=%t", mountIsEmpty, globalEmptyOK)
			}
			if mountIsEmpty {
				if root.Key() != globalEmptyKey {
					t.Fatal("empty key diverged between per-mount catalog and global directory")
				}
				ordinal, ordinalOK := heap.EmptyAllocationOrdinal(globalEmptyKey)
				if !ordinalOK {
					t.Fatal("empty key not present in global directory ordinal")
				}
				if emptyFromMounts[ordinal] {
					t.Fatalf("empty ordinal %d observed twice across mounts", ordinal)
				}
				emptyFromMounts[ordinal] = true
			}
		}
	}

	if len(closedFromMounts) != closedCount {
		t.Fatalf("per-mount closed membership size=%d, global directory size=%d", len(closedFromMounts), closedCount)
	}
	if len(emptyFromMounts) != emptyCount {
		t.Fatalf("per-mount empty membership size=%d, global directory size=%d", len(emptyFromMounts), emptyCount)
	}

	for index := 0; index < closedCount; index++ {
		if !closedFromMounts[uint32(index)] {
			t.Fatalf("global closed ordinal %d has no per-mount member", index)
		}
		key, keyOK := heap.ClosedAllocationAt(index)
		if !keyOK {
			t.Fatalf("ClosedAllocationAt(%d) unavailable", index)
		}
		ordinal, ordinalOK := heap.ClosedAllocationOrdinal(key)
		if !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("closed ordinal inverse broke at %d: got=%d ok=%t", index, ordinal, ordinalOK)
		}
		selfProjected, selfOK := key.ClosedAllocation()
		if !selfOK || selfProjected != key {
			t.Fatalf("ClosedAllocation() projection broke at ordinal %d", index)
		}
		if _, emptyOK := key.EmptyAllocation(); emptyOK {
			t.Fatalf("closed-form key at ordinal %d was also accepted by EmptyAllocation()", index)
		}
	}
	for index := 0; index < emptyCount; index++ {
		if !emptyFromMounts[uint32(index)] {
			t.Fatalf("global empty ordinal %d has no per-mount member", index)
		}
		key, keyOK := heap.EmptyAllocationAt(index)
		if !keyOK {
			t.Fatalf("EmptyAllocationAt(%d) unavailable", index)
		}
		ordinal, ordinalOK := heap.EmptyAllocationOrdinal(key)
		if !ordinalOK || ordinal != uint32(index) {
			t.Fatalf("empty ordinal inverse broke at %d: got=%d ok=%t", index, ordinal, ordinalOK)
		}
		selfProjected, selfOK := key.EmptyAllocation()
		if !selfOK || selfProjected != key {
			t.Fatalf("EmptyAllocation() projection broke at ordinal %d", index)
		}
		if _, closedOK := key.ClosedAllocation(); closedOK {
			t.Fatalf("empty-form key at ordinal %d was also accepted by ClosedAllocation()", index)
		}
	}
}

func formDirectoryProgram(t testing.TB, name, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func formDirectoryFixture(t testing.TB, name, text string) (heapdomain.Schema, *valuedomain.Schema) {
	t.Helper()
	p := formDirectoryProgram(t, name, text)
	requireOperation, requireErr := testfixture.ScopedRequireOperation()
	if requireErr != nil {
		t.Fatal(requireErr)
	}
	target, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{requireOperation}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	executionSchemaID := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if !compilationOK || !executionSchemaID.Available() || !issuanceOK || !shardOK || !moduleOK {
		t.Fatal("form directory single-mount receipt")
	}
	artifact, failure := artifactcompiler.CompileDetailed(p, executionSchemaID, issuance)
	if failure.Available() || artifact == nil {
		t.Fatalf("form directory single-mount artifact: %v", failure)
	}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !heapMountOK {
		t.Fatal("form directory heap mount")
	}
	heap, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("form directory heap seal: %v", heapFailure)
	}
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshottest.MustLower(t, artifact), module)
	if !valueMountOK {
		t.Fatal("form directory value mount")
	}
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("form directory structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heap, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone {
		t.Fatalf("form directory value seal: %v", valueFailure)
	}
	return heap, values
}
