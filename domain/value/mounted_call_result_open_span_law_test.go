package value_test

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestMountedCallResultOpenSpanAdmitsCanonicalEmptySpan pins the canonical
// CallResult span law that Value's mounted geometry seal must consume: only an
// exact-multiplicity result owns a dense child span in the CallResultSlot
// plane, while an open-multiplicity result publishes the canonical empty span
// (offset 0, count 0) regardless of how many slots earlier results own.
// programschema.CallResult.Available() rejects any other open span, so a seal
// that demands a running cursor from every row refuses every program in which
// an open result follows a slot-bearing one.
func TestMountedCallResultOpenSpanAdmitsCanonicalEmptySpan(t *testing.T) {
	const source = "local function fib(n)\n" +
		"\tif n < 2 then\n" +
		"\t\treturn n\n" +
		"\tend\n" +
		"\treturn fib(n - 1) + fib(n - 2)\n" +
		"end\n" +
		"return fib(10)\n"

	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "mounted_call_result_open_span.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)

	// The law is only exercised when the compiled program actually publishes
	// an open result after a slot-bearing one.
	published := snapshot.Program()
	count, countOK := published.CallResultCount()
	if !countOK || count == 0 {
		t.Fatalf("call result count = %d/%t", count, countOK)
	}
	owned := uint32(0)
	openAfterSlots := false
	for index := 0; index < count; index++ {
		row, rowOK := published.CallResultAt(index)
		if !rowOK || !row.Available() {
			t.Fatalf("call result row %d unavailable", index)
		}
		open, openOK := row.ResultsOpen()
		offset, width, spanOK := row.SlotSpan()
		if !openOK || !spanOK {
			t.Fatalf("call result row %d span=%t open=%t", index, spanOK, openOK)
		}
		if open {
			if offset != 0 || width != 0 {
				t.Fatalf("open call result row %d span = (%d,%d), want the canonical empty span", index, offset, width)
			}
			if owned != 0 {
				openAfterSlots = true
			}
			continue
		}
		if offset != owned {
			t.Fatalf("exact call result row %d offset = %d, want the running slot cursor %d", index, offset, owned)
		}
		owned += width
	}
	if !openAfterSlots {
		t.Fatal("fixture publishes no open call result after a slot-bearing result; the law is not exercised")
	}
	slotCount, slotCountOK := published.CallResultSlotCount()
	if !slotCountOK || uint32(slotCount) != owned {
		t.Fatalf("exact results own %d slots, plane holds %d/%t", owned, slotCount, slotCountOK)
	}

	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("seal heap schema: %s", heapFailure)
	}
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal value schema: %s", valueFailure)
	}
}
