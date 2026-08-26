package expand_test

import (
	"sort"
	"testing"

	deltaeval "github.com/wippyai/go-lua/analysis/engine/relation/eval/delta"
	"github.com/wippyai/go-lua/analysis/engine/relation/eval/step"
	expandfixture "github.com/wippyai/go-lua/analysis/engine/relation/runtime/testdata/targetfixture/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/solve/fixpoint"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// TestLaterExpandReaderLineageOnlyMatchesFullSuccessor proves the true
// R-only Later path on the relcompiled Expand fixture. The decode-only
// payload is republished with the same opaque token and a new authenticated
// lineage, so the database delta changes exactly one R payload extent while
// preserving its key. Later must redeem only the C rows named by that key and
// produce the same ordered tuples as a full evaluation at the successor.
func TestLaterExpandReaderLineageOnlyMatchesFullSuccessor(t *testing.T) {
	fixture := expandfixture.New(t)
	delta, ok := fixture.ReaderPayloadLineageDelta()
	if !ok || !delta.Available() {
		t.Fatal("reader payload lineage delta")
	}
	if delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatalf("reader payload delta semantic=%t lineage=%t", delta.SemanticChanged(), delta.LineageChanged())
	}
	changed := delta.ChangedColumnIDs()
	if len(changed) != 1 || changed[0] != fixture.ReaderPayload1Column() {
		t.Fatalf("changed columns=%v, want reader payload-1 only", changed)
	}
	change, ok := delta.Change(fixture.ReaderPayload1Column())
	if !ok || change.Len() != 1 {
		t.Fatalf("reader payload change available=%t extents=%d", ok, change.Len())
	}
	entry, ok := change.At(0)
	if !ok {
		t.Fatal("reader payload change extent")
	}
	before, beforePresence, beforeOK := entry.Before()
	after, afterPresence, afterOK := entry.After()
	if !beforeOK || !afterOK || !before.Same(after) || !beforePresence.Is(model.AuthenticatedOpaque) || !afterPresence.Is(model.AuthenticatedOpaque) {
		t.Fatal("reader payload delta changed opaque value or presence")
	}
	beforeLineage, beforeLineageOK := entry.BeforeLineage()
	afterLineage, afterLineageOK := entry.AfterLineage()
	if !beforeLineageOK || !afterLineageOK || beforeLineage == afterLineage || !afterLineage.Available() {
		t.Fatal("reader payload delta did not advance lineage")
	}

	world := fixture.World()
	later, ok := fixpoint.Later(delta)
	if !ok {
		t.Fatal("later root")
	}
	execution := world.Mounted().Arrangement().Execution()
	schedule, ok := execution.Dependency(fixture.Dependency())
	if !ok || !schedule.Available() || schedule.Node().Kind() != algebra.KindExpand {
		t.Fatal("relcompiled Expand schedule")
	}
	session, ok := deltaeval.New(world.Mounted(), later, world.View())
	if !ok || !session.Available() {
		t.Fatal("later session")
	}
	laterResult, ok := session.Evaluate(schedule)
	if !ok || !laterResult.Available() || laterResult.Kind() != algebra.KindExpand {
		t.Fatalf("later Expand result ok=%t available=%t kind=%v", ok, laterResult.Available(), laterResult.Kind())
	}

	fullSession, ok := step.New(world.Mounted(), delta.Next(), world.View())
	if !ok || !fullSession.Available() {
		t.Fatal("successor full session")
	}
	fullResult, ok := fullSession.Evaluate(schedule)
	if !ok || !fullResult.Available() || fullResult.Kind() != algebra.KindExpand {
		t.Fatalf("successor full Expand result ok=%t available=%t kind=%v", ok, fullResult.Available(), fullResult.Kind())
	}

	laterBatches, fullBatches := laterResult.Batches(), fullResult.Batches()
	laterRows, fullRows := 0, 0
	for _, batch := range laterBatches {
		laterRows += batch.Len()
	}
	for _, batch := range fullBatches {
		fullRows += batch.Len()
	}
	if len(laterBatches) != len(fullBatches) || len(laterBatches) != 1 || laterBatches[0].Len() != 3 || fullBatches[0].Len() != 3 {
		t.Fatalf("Later/full batches=%d/%d rows=%d/%d", len(laterBatches), len(fullBatches), laterRows, fullRows)
	}
	if !laterBatches[0].Scope().Same(fullBatches[0].Scope()) {
		t.Fatal("Later changed the Expand output scope")
	}
	mainScope, ok := world.Mounted().Scope(fixture.MainScope())
	if !ok || !laterBatches[0].Scope().Same(mainScope) {
		t.Fatal("Later Expand output escaped its owner scope")
	}
	for index := 0; index < laterBatches[0].Len(); index++ {
		laterTuple, laterOK := laterBatches[0].At(index)
		fullTuple, fullOK := fullBatches[0].At(index)
		if !laterOK || !fullOK || !laterTuple.Same(fullTuple) || laterTuple.Lineage() != fullTuple.Lineage() {
			t.Fatalf("affected tuple %d diverged from full successor", index)
		}
	}

	// The changed R key wakes the C rows named by frozen evidence. Their
	// encounter order is the sealed C owner directory order, not the cold
	// evidence-vector order; derive the expected transport order through the
	// same Mounted.RowIndex authority used by the runtime replay.
	candidateOrder := []model.RowID{fixture.CandidateA(), fixture.CandidateB()}
	sort.SliceStable(candidateOrder, func(left, right int) bool {
		leftIndex, leftOK := world.Mounted().RowIndex(fixture.Contract().Candidate(), candidateOrder[left])
		rightIndex, rightOK := world.Mounted().RowIndex(fixture.Contract().Candidate(), candidateOrder[right])
		return leftOK && rightOK && leftIndex < rightIndex
	})
	wantCandidates := make([]model.RowID, 0, 3)
	wantReaders := make([]model.RowID, 0, 3)
	wantKeys := make([]identity.ContentID, 0, 3)
	for _, candidate := range candidateOrder {
		keys := []identity.ContentID{fixture.Key1()}
		if candidate == fixture.CandidateA() {
			keys = []identity.ContentID{fixture.Key2(), fixture.Key1()}
		}
		for _, key := range keys {
			wantCandidates = append(wantCandidates, candidate)
			wantReaders = append(wantReaders, fixture.ReaderRowFor(key))
			wantKeys = append(wantKeys, key)
		}
	}
	for index := range wantCandidates {
		value, ok := laterBatches[0].At(index)
		if !ok || value.SourceLen() != 2 {
			t.Fatalf("tuple %d source width=%d", index, value.SourceLen())
		}
		candidate, candidateOK := value.SourceAt(0)
		reader, readerOK := value.SourceAt(1)
		if !candidateOK || !readerOK || candidate != wantCandidates[index] || reader != wantReaders[index] {
			t.Fatalf("tuple %d sources=(%v,%v), want=(%v,%v)", index, candidate, reader, wantCandidates[index], wantReaders[index])
		}
		if !value.Scope().Same(mainScope) {
			t.Fatalf("tuple %d scope changed", index)
		}
		keyCell, keyOK := value.CellFor(fixture.ReaderKey())
		if !keyOK || !keyCell.Value().Available() || keyCell.Value().Opaque() != wantKeys[index] {
			t.Fatalf("tuple %d key=%v, want %v", index, keyCell.Value().Opaque(), wantKeys[index])
		}
		payload1, payload1OK := value.CellFor(fixture.ReaderPayload1Column())
		if !payload1OK || !payload1.Value().Available() || payload1.Value().Opaque() != fixture.ReaderPayload1For(wantKeys[index]) {
			t.Fatalf("tuple %d payload-1=%v", index, payload1.Value().Opaque())
		}
	}
}

func TestLaterExpandUnrelatedReaderKeyIsAuthenticatedEmpty(t *testing.T) {
	fixture := expandfixture.New(t)
	delta, ok := fixture.UnrelatedReaderPayloadLineageDelta()
	if !ok || !delta.Available() || delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatal("unrelated reader lineage delta")
	}
	changed := delta.ChangedColumnIDs()
	if len(changed) != 1 || changed[0] != fixture.ReaderPayload1Column() {
		t.Fatalf("unrelated changed columns=%v", changed)
	}
	later, ok := fixpoint.Later(delta)
	if !ok {
		t.Fatal("unrelated later root")
	}
	world := fixture.World()
	execution := world.Mounted().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.Dependency())
	if !ok {
		t.Fatal("unrelated Expand schedule")
	}
	session, ok := deltaeval.New(world.Mounted(), later, world.View())
	if !ok || !session.Available() {
		t.Fatal("unrelated later session")
	}
	result, ok := session.Evaluate(entry)
	if !ok || !result.Available() || result.Kind() != algebra.KindExpand || len(result.Batches()) != 0 || len(result.Applications()) != 0 || len(result.Settlements()) != 0 {
		t.Fatalf("unrelated Later result ok=%t available=%t kind=%v batches=%d applications=%d settlements=%d", ok, result.Available(), result.Kind(), len(result.Batches()), len(result.Applications()), len(result.Settlements()))
	}
	fullSession, ok := step.New(world.Mounted(), delta.Next(), world.View())
	if !ok || !fullSession.Available() {
		t.Fatal("unrelated successor full session")
	}
	fullResult, ok := fullSession.Evaluate(entry)
	if !ok || !fullResult.Available() || len(fullResult.Batches()) != 1 || fullResult.Batches()[0].Len() != 3 {
		t.Fatal("unrelated successor full Expand was not populated")
	}
	if !result.InputDelta().Next().Same(delta.Next()) || !result.Base().Same(delta.Base()) || !result.Next().Same(delta.Next()) {
		t.Fatal("unrelated empty lost its authenticated delta roots")
	}
}

// TestLaterExpandMixedCandidateAndReaderMatchesFullSuccessor proves the
// positive simultaneous pivot. Every candidate affected through either the
// compound C child or the R key frontier is recomputed exactly once at the
// successor epoch. The resulting ordered transport must equal a full
// successor evaluation, including the changed C and R lineage.
func TestLaterExpandMixedCandidateAndReaderMatchesFullSuccessor(t *testing.T) {
	fixture := expandfixture.New(t)
	delta, ok := fixture.MixedCandidateReaderDelta()
	if !ok || !delta.Available() {
		t.Fatal("mixed candidate/reader delta")
	}
	if delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatalf("mixed delta semantic=%t lineage=%t", delta.SemanticChanged(), delta.LineageChanged())
	}
	changed := delta.ChangedColumnIDs()
	if len(changed) != 2 {
		t.Fatalf("mixed changed columns=%v", changed)
	}
	seenCandidate, seenReader := false, false
	for _, column := range changed {
		seenCandidate = seenCandidate || column == fixture.CandidatePayload()
		seenReader = seenReader || column == fixture.ReaderPayload1Column()
	}
	if !seenCandidate || !seenReader {
		t.Fatalf("mixed changed columns=%v, want candidate payload and reader payload-1", changed)
	}

	world := fixture.World()
	later, ok := fixpoint.Later(delta)
	if !ok {
		t.Fatal("later root")
	}
	execution := world.Mounted().Arrangement().Execution()
	entry, ok := execution.Dependency(fixture.Dependency())
	if !ok || !entry.Available() || entry.Node().Kind() != algebra.KindExpand {
		t.Fatal("relcompiled Expand schedule")
	}
	laterSession, ok := deltaeval.New(world.Mounted(), later, world.View())
	if !ok || !laterSession.Available() {
		t.Fatal("later session")
	}
	laterResult, ok := laterSession.Evaluate(entry)
	if !ok || !laterResult.Available() || laterResult.Kind() != algebra.KindExpand {
		t.Fatalf("later Expand result ok=%t available=%t kind=%v", ok, laterResult.Available(), laterResult.Kind())
	}
	fullSession, ok := step.New(world.Mounted(), delta.Next(), world.View())
	if !ok || !fullSession.Available() {
		t.Fatal("successor full session")
	}
	fullResult, ok := fullSession.Evaluate(entry)
	if !ok || !fullResult.Available() || fullResult.Kind() != algebra.KindExpand {
		t.Fatalf("successor full Expand result ok=%t available=%t kind=%v", ok, fullResult.Available(), fullResult.Kind())
	}
	laterBatches, fullBatches := laterResult.Batches(), fullResult.Batches()
	if len(laterBatches) != 1 || len(fullBatches) != 1 || laterBatches[0].Len() != 3 || fullBatches[0].Len() != 3 {
		t.Fatalf("Later/full batches=%d/%d rows=%d/%d", len(laterBatches), len(fullBatches), batchRows(laterBatches), batchRows(fullBatches))
	}
	for index := 0; index < laterBatches[0].Len(); index++ {
		laterTuple, laterOK := laterBatches[0].At(index)
		fullTuple, fullOK := fullBatches[0].At(index)
		if !laterOK || !fullOK || !laterTuple.Same(fullTuple) || laterTuple.Lineage() != fullTuple.Lineage() {
			t.Fatalf("mixed tuple %d diverged from full successor", index)
		}
	}
}

func batchRows(batches []tuple.Batch) int {
	rows := 0
	for _, batch := range batches {
		rows += batch.Len()
	}
	return rows
}
