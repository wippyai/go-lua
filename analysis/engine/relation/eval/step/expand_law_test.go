package step_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	expandfixture "github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	arrangementexpand "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// TestEvaluateRelcompiledExpandRedeemsCommittedRows is the bounded W3
// vertical gate for the real relcompiled Expand path:
//
//	relcompile.Compile -> certificate.Check -> witness.Specialize ->
//	arrangement binding -> step evaluation over one committed root.
func TestEvaluateRelcompiledExpandRedeemsCommittedRows(t *testing.T) {
	fixture := expandfixture.New(t)
	world := fixture.World()
	if !world.Mounted().Available() || !world.Base().Available() {
		t.Fatal("expand target world is unavailable")
	}

	session, ok := step.New(world.Mounted(), world.Base(), world.View())
	if !ok || !session.Available() {
		t.Fatal("expand evaluator session")
	}
	execution := world.Mounted().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.Dependency())
	if !ok || !entry.Available() {
		t.Fatal("relcompiled Expand schedule entry")
	}
	if entry.Node().Kind() != algebra.KindExpand {
		t.Fatalf("root kind=%v, want Expand", entry.Node().Kind())
	}
	bindingValue, ok := entry.Node().Expand()
	if !ok || !bindingValue.Available() || bindingValue.Contract() != fixture.Contract() {
		t.Fatal("Expand arrangement binding")
	}
	// Runtime redemption is candidate-directed.  C/P correspondence is
	// validated and included in the frozen evidence digest, while no P-row
	// inverse accessor is exposed on the hot path.
	ownerVector, ok := bindingValue.Evidence().VectorAt(fixture.CandidateA())
	if !ok || !ownerVector.Available() || ownerVector.Candidate() != fixture.CandidateA() || ownerVector.KeyCount() != 3 {
		t.Fatal("owner-issued C/P vector was not specialized")
	}
	for index, expected := range []identity.ContentID{fixture.Key2(), fixture.Key3(), fixture.Key1()} {
		key, keyOK := ownerVector.KeyAt(index)
		if !keyOK || !key.Available() || key.Opaque() != expected {
			t.Fatalf("owner vector key %d=%v, want %v", index, key.Opaque(), expected)
		}
	}

	result, ok := session.Evaluate(entry)
	if !ok || !result.Available() || result.Kind() != algebra.KindExpand {
		t.Fatalf("Expand result=(ok:%v available:%v kind:%v)", ok, result.Available(), result.Kind())
	}
	batches := result.Batches()
	if len(batches) != 1 || !batches[0].Available() || batches[0].Len() != 3 {
		t.Fatalf("Expand batches=%d, len=%d; want one ordered batch of three", len(batches), batchLen(batches))
	}
	mainScope, ok := world.Mounted().Scope(fixture.MainScope())
	if !ok || !mainScope.Available() || !batches[0].Scope().Same(mainScope) {
		t.Fatal("Expand widened or replaced the committed output scope")
	}

	expectedKeys := map[model.RowID][]identity.ContentID{
		fixture.CandidateA(): {fixture.Key2(), fixture.Key1()},
		fixture.CandidateB(): {fixture.Key1()},
	}
	seenKeys := make(map[model.RowID]int, len(expectedKeys))
	var previousCandidate model.RowID
	for index := 0; index < batches[0].Len(); index++ {
		value, ok := batches[0].At(index)
		if !ok || !value.Available() || value.SourceLen() != 2 || value.Len() != 5 {
			t.Fatalf("tuple %d shape: ok=%v available=%v sources=%d cells=%d", index, ok, value.Available(), value.SourceLen(), value.Len())
		}
		candidate, candidateOK := value.SourceAt(0)
		reader, readerOK := value.SourceAt(1)
		if !candidateOK || !readerOK {
			t.Fatalf("tuple %d missing candidate/reader sources", index)
		}
		keys, candidateKnown := expectedKeys[candidate]
		position := seenKeys[candidate]
		if !candidateKnown || position >= len(keys) {
			t.Fatalf("tuple %d unexpected candidate source %v", index, candidate)
		}
		expectedKey := keys[position]
		if index > 0 && candidate != previousCandidate && seenKeys[candidate] != 0 {
			t.Fatalf("tuple %d candidate %v reappeared after another candidate", index, candidate)
		}
		previousCandidate = candidate
		seenKeys[candidate] = position + 1
		if reader != fixture.ReaderRowFor(expectedKey) {
			t.Fatalf("tuple %d reader source=%v, want %v", index, reader, fixture.ReaderRowFor(expectedKey))
		}
		wantColumns := []model.ColumnID{fixture.CandidateAddress(), fixture.CandidatePayload(), fixture.ReaderKey(), fixture.ReaderPayload1Column(), fixture.ReaderPayload2Column()}
		wantValues := []identity.ContentID{
			fixture.CandidateAddressValue(candidate), fixture.CandidatePayloadValue(candidate), expectedKey,
			fixture.ReaderPayload1For(expectedKey), fixture.ReaderPayload2For(expectedKey),
		}
		wantSources := []uint32{0, 0, 1, 1, 1}
		for cellIndex, cell := range value.Cells() {
			if cell.Source() != wantSources[cellIndex] || cell.Column() != wantColumns[cellIndex] || !cell.Value().Available() || cell.Value().Opaque() != wantValues[cellIndex] {
				t.Fatalf("tuple %d cell %d=(source:%d column:%v value:%v), want source:%d column:%v value:%v", index, cellIndex, cell.Source(), cell.Column(), cell.Value().Opaque(), wantSources[cellIndex], wantColumns[cellIndex], wantValues[cellIndex])
			}
		}
	}
	if seenKeys[fixture.CandidateA()] != 2 || seenKeys[fixture.CandidateB()] != 1 {
		t.Fatalf("Expand candidate vector counts=%v, want A:2 B:1", seenKeys)
	}

	// A nominally mismatched P row is refused by the same mounted evidence
	// fence before runtime can redeem any vector.
	issuer, ok := binding.NewIssuer(world.Mounted().RuntimeFence())
	if !ok {
		t.Fatal("Expand evidence issuer")
	}
	foreignPublisher, ok := model.IssueRowID(fixture.ForeignPublisherRelation(), fixture.PublisherA().Content())
	if !ok {
		t.Fatal("foreign publisher row")
	}
	bad, ok := arrangementexpand.NewVector(fixture.CandidateA(), foreignPublisher, []identity.ContentID{fixture.Key1()})
	if !ok {
		t.Fatal("foreign publisher vector")
	}
	if evidence, frozen := arrangementexpand.Freeze(issuer.Fence(), issuer, fixture.Contract(), fixture.TypeID(), []arrangementexpand.Vector{bad}); frozen || evidence.Available() {
		t.Fatal("Expand admitted a nominal C/P mismatch")
	}
}

func batchLen(values []tuple.Batch) int {
	if len(values) != 1 {
		return 0
	}
	return values[0].Len()
}
