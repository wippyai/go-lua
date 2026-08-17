package engine

import (
	"context"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// The materializer laws drive the inert publication end to end. They build a
// real solve through the shared engine fixtures, publish its completed results
// as snapshot columns, and then prove the publication against the readers the
// solve itself offers: the same values, the four read outcomes, one content
// identity per published row set, and a change-set priced republication.

var (
	sinkAnswer          Answer
	sinkAnswerStatus    snapshot.ReadStatus
	sinkAnswerRows      []uint64
	sinkAnswerReadable  bool
	sinkAnswerPlan      snapshot.QueryPlan[identity.ContentID, Answer]
	sinkAnswerOpened    bool
	sinkSolvedSnapshot  snapshot.Snapshot
	syntheticRowSchema  = identity.ContentID{0x51, 0xA0, 0x0B}
	syntheticRowStore   = identity.StoreID(4_099)
	syntheticFirstStamp = identity.Generation(1)
	syntheticNextStamp  = identity.Generation(2)
)

// materializedQueryFixture publishes one completed solve that answers a query
// row, together with the reader that solve offers for it.
func materializedQueryFixture(t testing.TB) (*Solver, ReceiptQuery, *State, SolvedSnapshot) {
	t.Helper()
	solver, query, state := newBorrowedQueryFixture(t)
	materialized, failure := materializeCompletedState(solver, state)
	if failure.Available() || !materialized.Available() {
		t.Fatalf("materialize completed query state = %s", failure)
	}
	return solver, query, state, materialized
}

// materializedObservationFixture is the observation-lane counterpart.
func materializedObservationFixture(t testing.TB) (*Solver, ReceiptObservation[[]uint64], *State, SolvedSnapshot) {
	t.Helper()
	solver, observation, state := newBorrowedObservationFixture(t)
	materialized, failure := materializeCompletedState(solver, state)
	if failure.Available() || !materialized.Available() {
		t.Fatalf("materialize completed observation state = %s", failure)
	}
	return solver, observation, state, materialized
}

// openAnswerColumn opens the result column of one published family.
func openAnswerColumn(t testing.TB, published *snapshot.Snapshot, family identity.ContentID) snapshot.QueryPlan[identity.ContentID, Answer] {
	t.Helper()
	plan, opened := snapshot.OpenQuery[identity.ContentID, Answer](published, family)
	if !opened || !plan.Available() {
		t.Fatalf("open result column %s", family)
	}
	return plan
}

// TestSnapshotMaterializePublishesEveryStateReader is the equivalence law: a
// result the solve's own readers reach is a published answer of the
// materialization, at the same value and by the same borrow, and the
// publication anchors name the store and completion revision that published
// the state.
func TestSnapshotMaterializePublishesEveryStateReader(t *testing.T) {
	solver, query, state, materialized := materializedQueryFixture(t)
	published := materialized.Snapshot()

	if published.Store() != solver.store || published.Generation() != state.completion.serial {
		t.Fatalf("publication anchors = (%d, %d), want (%d, %d)", published.Store(), published.Generation(), solver.store, state.completion.serial)
	}
	if published.Columns() != solvedAxisCount || published.Queries().Len() != solvedAxisCount {
		t.Fatalf("published columns/queries = %d/%d, want %d", published.Columns(), published.Queries().Len(), solvedAxisCount)
	}
	if len(solver.runtime.queries) == 0 {
		t.Fatal("the query fixture declares no query row")
	}

	borrowed, readable := ReceiptQueryResult[[]uint64](query, solver, state)
	if !readable || len(borrowed) != 2 {
		t.Fatalf("state reader = %#v/%t", borrowed, readable)
	}
	plan := openAnswerColumn(t, &published, materialized.QueryFamily())
	for _, declared := range solver.runtime.queries {
		key := solvedRowKey(declared.query().Key())
		answer, status := snapshot.Query(&published, plan, key)
		if status != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("query row %s = %v/%t, want hit", key, status, answer.Available())
		}
		value, typed := AnswerValue[[]uint64](answer)
		if !typed || len(value) != len(borrowed) || &value[0] != &borrowed[0] {
			t.Fatalf("published answer %#v/%t is not the borrowed result %#v", value, typed, borrowed)
		}
	}

	// The observation axis of this solve declares no row, so it publishes an
	// empty result column: it is openable and answers nothing.
	empty := openAnswerColumn(t, &published, materialized.ObservationFamily())
	if _, status := snapshot.Query(&published, empty, solvedRowKey(solver.runtime.queries[0].query().Key())); status != snapshot.ReadMiss {
		t.Fatalf("empty observation column status = %v, want miss", status)
	}

	observationSolver, observation, observationState, observationMaterialized := materializedObservationFixture(t)
	observationPublished := observationMaterialized.Snapshot()
	if len(observationSolver.runtime.observations) == 0 {
		t.Fatal("the observation fixture declares no observation row")
	}
	observationBorrowed, observationReadable := ReceiptObservationResult[[]uint64](observation, observationSolver, observationState)
	if !observationReadable || len(observationBorrowed) != 2 {
		t.Fatalf("observation state reader = %#v/%t", observationBorrowed, observationReadable)
	}
	observationPlan := openAnswerColumn(t, &observationPublished, observationMaterialized.ObservationFamily())
	for _, declared := range observationSolver.runtime.observations {
		key := declared.observationID()
		answer, status := snapshot.Query(&observationPublished, observationPlan, key)
		if status != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("observation row %s = %v/%t, want hit", key, status, answer.Available())
		}
		value, typed := AnswerValue[[]uint64](answer)
		if !typed || len(value) != len(observationBorrowed) || &value[0] != &observationBorrowed[0] {
			t.Fatalf("published observation %#v/%t is not the borrowed result %#v", value, typed, observationBorrowed)
		}
		detached, owned := DetachAnswer[[]uint64](answer)
		if !owned || len(detached) != len(value) || &detached[0] == &value[0] {
			t.Fatalf("detached observation %#v/%t reached the published value", detached, owned)
		}
	}
}

// TestSnapshotMaterializeAnswersFourOutcomes fixes the read outcomes of a
// published result column. A declared, answered row is a hit; a key the axis
// never declared is a miss; a declared row the publication does not answer --
// withdrawn by a delta, or declared unanswered -- is a proven absence; and a
// family the snapshot does not publish opens nothing while a minted plan reads
// nothing.
func TestSnapshotMaterializeAnswersFourOutcomes(t *testing.T) {
	solver, _, state, materialized := materializedQueryFixture(t)
	published := materialized.Snapshot()
	plan := openAnswerColumn(t, &published, materialized.QueryFamily())
	key := solvedRowKey(solver.runtime.queries[0].query().Key())

	if _, status := snapshot.Query(&published, plan, key); status != snapshot.ReadHit {
		t.Fatalf("declared answered row = %v, want hit", status)
	}
	if _, status := snapshot.Query(&published, plan, syntheticRowKey(9_001)); status != snapshot.ReadMiss {
		t.Fatalf("undeclared key = %v, want miss", status)
	}

	withdrawn, failure := materialized.republish(resultLaneQuery, key, nil, state.completion.serial.Next())
	if failure.Available() {
		t.Fatalf("withdraw published answer = %s", failure)
	}
	withdrawnPlan := openAnswerColumn(t, &withdrawn, materialized.QueryFamily())
	if _, status := snapshot.Query(&withdrawn, withdrawnPlan, key); status != snapshot.ReadProvenAbsent {
		t.Fatalf("withdrawn row = %v, want proven-absent", status)
	}
	if _, status := snapshot.Query(&published, plan, key); status != snapshot.ReadHit {
		t.Fatalf("a derived publication reached the base row = %v, want hit", status)
	}

	// A declared row that carries no value is the same published absence. The
	// input contract states it directly, so the outcome does not depend on a
	// solve that leaves a row unanswered.
	unanswered := syntheticSolvedResults(t, 4)
	unanswered.axes[0].rows = append(unanswered.axes[0].rows, solvedRow{key: syntheticRowKey(4)})
	partial, partialFailure := materializeSolvedResults(unanswered)
	if partialFailure.Available() {
		t.Fatalf("materialize partially answered axis = %s", partialFailure)
	}
	partialPublished := partial.Snapshot()
	partialPlan := openAnswerColumn(t, &partialPublished, partial.QueryFamily())
	if _, status := snapshot.Query(&partialPublished, partialPlan, syntheticRowKey(4)); status != snapshot.ReadProvenAbsent {
		t.Fatalf("declared unanswered row = %v, want proven-absent", status)
	}

	if _, opened := snapshot.OpenQuery[identity.ContentID, Answer](&published, syntheticRowKey(7_777)); opened {
		t.Fatal("an unpublished family opened a result column")
	}
	if _, opened := snapshot.OpenQuery[identity.ContentID, uint64](&published, materialized.QueryFamily()); opened {
		t.Fatal("a published family opened a result column of a foreign answer type")
	}
	minted := snapshot.QueryPlan[identity.ContentID, Answer]{SchemaID: materialized.schema, Slot: uint32(solvedAxisCount)}
	if _, status := snapshot.Query(&published, minted, key); status != snapshot.ReadInvalid {
		t.Fatalf("minted out-of-range plan = %v, want invalid", status)
	}
	foreign := snapshot.QueryPlan[identity.ContentID, Answer]{SchemaID: syntheticRowSchema, Slot: plan.Slot}
	if _, status := snapshot.Query(&published, foreign, key); status != snapshot.ReadInvalid {
		t.Fatalf("foreign schema plan = %v, want invalid", status)
	}
}

// TestSnapshotMaterializeIsContentAddressed is the determinism law. One solve
// materialized twice publishes the identical snapshot, and a second completion
// of the same solve publishes the identical content under an advanced
// publication stamp: the content identity is a function of the published rows
// alone.
func TestSnapshotMaterializeIsContentAddressed(t *testing.T) {
	solver, _, state, first := materializedQueryFixture(t)
	second, failure := materializeCompletedState(solver, state)
	if failure.Available() {
		t.Fatalf("re-materialize completed state = %s", failure)
	}
	if first.Content() != second.Content() || first.schema != second.schema {
		t.Fatalf("re-materialization content = %s, want %s", second.Content(), first.Content())
	}
	if first.QueryFamily() != second.QueryFamily() || first.ObservationFamily() != second.ObservationFamily() {
		t.Fatal("re-materialization published other families")
	}
	firstPublished, secondPublished := first.Snapshot(), second.Snapshot()
	if firstPublished.Generation() != secondPublished.Generation() || firstPublished.Store() != secondPublished.Store() {
		t.Fatal("re-materialization published another store revision")
	}
	assertEqualAnswers(t, solver, &firstPublished, &secondPublished, first)

	nextState, status := solver.Solve(context.Background())
	if status != SolveComplete || nextState == nil {
		t.Fatalf("second solve = status:%v state:%t", status, nextState != nil)
	}
	next, nextFailure := materializeCompletedState(solver, nextState)
	if nextFailure.Available() {
		t.Fatalf("materialize second completion = %s", nextFailure)
	}
	nextPublished := next.Snapshot()
	if next.Content() != first.Content() {
		t.Fatalf("second completion content = %s, want %s", next.Content(), first.Content())
	}
	if nextPublished.Store() != firstPublished.Store() || !atOrBefore(firstPublished.Generation(), nextPublished.Generation()) {
		t.Fatalf("second completion anchors = (%d, %d), want a revision of store %d at or after %d", nextPublished.Store(), nextPublished.Generation(), firstPublished.Store(), firstPublished.Generation())
	}
	assertEqualAnswers(t, solver, &firstPublished, &nextPublished, first)

	// A changed answer is a changed content identity.
	changed := syntheticSolvedResults(t, 4)
	base, baseFailure := materializeSolvedResults(changed)
	changed.axes[0].rows[2].value = syntheticAnswerValue(4_099)
	edited, editedFailure := materializeSolvedResults(changed)
	if baseFailure.Available() || editedFailure.Available() {
		t.Fatalf("materialize synthetic rows = %s/%s", baseFailure, editedFailure)
	}
	if base.Content() == edited.Content() {
		t.Fatalf("two row sets share content identity %s", base.Content())
	}
}

// assertEqualAnswers proves two publications answer every declared row of every
// axis alike.
func assertEqualAnswers(t *testing.T, solver *Solver, left, right *snapshot.Snapshot, materialized SolvedSnapshot) {
	t.Helper()
	leftQueries := openAnswerColumn(t, left, materialized.QueryFamily())
	rightQueries := openAnswerColumn(t, right, materialized.QueryFamily())
	for _, declared := range solver.runtime.queries {
		key := solvedRowKey(declared.query().Key())
		leftAnswer, leftStatus := snapshot.Query(left, leftQueries, key)
		rightAnswer, rightStatus := snapshot.Query(right, rightQueries, key)
		if leftStatus != rightStatus || !leftAnswer.Equal(rightAnswer) || leftAnswer.Fingerprint() != rightAnswer.Fingerprint() {
			t.Fatalf("query row %s = %v/%v across publications", key, leftStatus, rightStatus)
		}
	}
	leftObservations := openAnswerColumn(t, left, materialized.ObservationFamily())
	rightObservations := openAnswerColumn(t, right, materialized.ObservationFamily())
	for _, declared := range solver.runtime.observations {
		key := declared.observationID()
		leftAnswer, leftStatus := snapshot.Query(left, leftObservations, key)
		rightAnswer, rightStatus := snapshot.Query(right, rightObservations, key)
		if leftStatus != rightStatus || !leftAnswer.Equal(rightAnswer) || leftAnswer.Fingerprint() != rightAnswer.Fingerprint() {
			t.Fatalf("observation row %s = %v/%v across publications", key, leftStatus, rightStatus)
		}
	}
}

// TestSnapshotMaterializeDeltaCostsItsChangeSet is the republication cost law.
// A materialization that changes one answer pays for that answer's path and for
// nothing else: the cost is measured on two column widths, and a column ten
// times as wide must not cost ten times as much to republish.
func TestSnapshotMaterializeDeltaCostsItsChangeSet(t *testing.T) {
	const (
		narrowRows = 1_000
		wideRows   = narrowRows * 10
		bound      = 40
	)
	narrowAllocations, narrowBytes := republicationCost(t, narrowRows)
	wideAllocations, wideBytes := republicationCost(t, wideRows)
	t.Logf("%d rows: %v allocations, %d bytes", narrowRows, narrowAllocations, narrowBytes)
	t.Logf("%d rows: %v allocations, %d bytes", wideRows, wideAllocations, wideBytes)

	if narrowAllocations > bound || wideAllocations > bound {
		t.Fatalf("republication allocations = %v and %v, want at most %d", narrowAllocations, wideAllocations, bound)
	}
	if wideAllocations > narrowAllocations+4 {
		t.Fatalf("a %dx wider column costs %v allocations against %v: republication scales with the column", wideRows/narrowRows, wideAllocations, narrowAllocations)
	}
	if wideBytes > narrowBytes*2 || wideBytes > 4096 {
		t.Fatalf("a %dx wider column costs %d bytes against %d: republication copies the column", wideRows/narrowRows, wideBytes, narrowBytes)
	}

	// Structural sharing is observable from outside the storage: every untouched
	// row of the republication answers what the base answered, at the base's own
	// borrowed value, and the base keeps answering what it published.
	materialized, failure := materializeSolvedResults(syntheticSolvedResults(t, 64))
	if failure.Available() {
		t.Fatalf("materialize synthetic rows = %s", failure)
	}
	base := materialized.Snapshot()
	republished, republishFailure := materialized.republish(resultLaneQuery, syntheticRowKey(7), syntheticAnswerValue(700), syntheticNextStamp)
	if republishFailure.Available() {
		t.Fatalf("republish one answer = %s", republishFailure)
	}
	basePlan := openAnswerColumn(t, &base, materialized.QueryFamily())
	nextPlan := openAnswerColumn(t, &republished, materialized.QueryFamily())
	for index := 0; index < 64; index++ {
		key := syntheticRowKey(uint64(index))
		baseAnswer, baseStatus := snapshot.Query(&base, basePlan, key)
		nextAnswer, nextStatus := snapshot.Query(&republished, nextPlan, key)
		if index == 7 {
			if baseStatus != snapshot.ReadHit || nextStatus != snapshot.ReadHit || baseAnswer.Equal(nextAnswer) {
				t.Fatalf("changed row = %v/%v, want two published answers", baseStatus, nextStatus)
			}
			continue
		}
		if baseStatus != nextStatus || !baseAnswer.Equal(nextAnswer) {
			t.Fatalf("row %d = %v/%v across a republication", index, baseStatus, nextStatus)
		}
	}
	observations := openAnswerColumn(t, &republished, materialized.ObservationFamily())
	if _, status := snapshot.Query(&republished, observations, syntheticRowKey(1<<40)); status != snapshot.ReadHit {
		t.Fatalf("untouched observation column = %v, want hit", status)
	}
	if republished.Store() != base.Store() || !base.Generation().Precedes(republished.Generation()) {
		t.Fatalf("republication anchors = (%d, %d)", republished.Store(), republished.Generation())
	}
}

// republicationCost measures one republication of a column holding rows rows.
func republicationCost(t *testing.T, rows int) (float64, uint64) {
	t.Helper()
	materialized, failure := materializeSolvedResults(syntheticSolvedResults(t, rows))
	if failure.Available() {
		t.Fatalf("materialize %d synthetic rows = %s", rows, failure)
	}
	value := syntheticAnswerValue(700)
	republish := func() {
		published, republishFailure := materialized.republish(resultLaneQuery, syntheticRowKey(7), value, syntheticNextStamp)
		if republishFailure.Available() {
			t.Fatalf("republish = %s", republishFailure)
		}
		sinkSolvedSnapshot = published
	}
	const runs = 100
	allocations := testing.AllocsPerRun(runs, republish)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for iteration := 0; iteration < runs; iteration++ {
		republish()
	}
	runtime.ReadMemStats(&after)
	return allocations, (after.TotalAlloc - before.TotalAlloc) / runs
}

// TestSnapshotMaterializeReadAllocatesNothing is the read cost law. Opening a
// published family, answering a key, and borrowing the typed value out of the
// answer allocate nothing: the materializer publishes into the snapshot's own
// storage and wraps its reads in no adapter. Detachment allocates, which is
// exactly why it is a caller's explicit request.
func TestSnapshotMaterializeReadAllocatesNothing(t *testing.T) {
	solver, _, _, materialized := materializedQueryFixture(t)
	published := materialized.Snapshot()
	family := materialized.QueryFamily()
	key := solvedRowKey(solver.runtime.queries[0].query().Key())

	read := func() {
		sinkAnswerPlan, sinkAnswerOpened = snapshot.OpenQuery[identity.ContentID, Answer](&published, family)
		sinkAnswer, sinkAnswerStatus = snapshot.Query(&published, sinkAnswerPlan, key)
		sinkAnswerRows, sinkAnswerReadable = AnswerValue[[]uint64](sinkAnswer)
	}
	read()
	if !sinkAnswerOpened || sinkAnswerStatus != snapshot.ReadHit || !sinkAnswerReadable || len(sinkAnswerRows) != 2 {
		t.Fatalf("published read = %v/%t/%#v", sinkAnswerStatus, sinkAnswerReadable, sinkAnswerRows)
	}
	if allocations := testing.AllocsPerRun(200, read); allocations != 0 {
		t.Fatalf("published read allocated %v times per read", allocations)
	}

	answer := sinkAnswer
	detach := func() {
		sinkAnswerRows, sinkAnswerReadable = DetachAnswer[[]uint64](answer)
	}
	detach()
	if !sinkAnswerReadable || len(sinkAnswerRows) != 2 {
		t.Fatalf("detached read = %#v/%t", sinkAnswerRows, sinkAnswerReadable)
	}
	if allocations := testing.AllocsPerRun(200, detach); allocations == 0 {
		t.Fatal("a detached read charged nothing, so the borrow law proves nothing")
	}
}

// TestSnapshotMaterializeRefusesClosed fixes the materializer's admission. It
// publishes only a completed state its own solver owns, refuses an input that
// names no publication or declares one axis twice, and speaks every refusal in
// the public failure vocabulary.
func TestSnapshotMaterializeRefusesClosed(t *testing.T) {
	solver, _, state, materialized := materializedQueryFixture(t)
	foreignSolver, _, foreignState := newBorrowedQueryFixture(t)

	for name, refusal := range map[string]SolveFailure{
		"no solver":       failureOf(t, nil, state),
		"no state":        failureOf(t, solver, nil),
		"foreign state":   failureOf(t, solver, foreignState),
		"foreign solver":  failureOf(t, foreignSolver, state),
		"no publication":  refusalOf(t, solvedResults{}),
		"unnamed axis":    refusalOf(t, solvedResults{schema: syntheticRowSchema, store: syntheticRowStore, generation: syntheticFirstStamp, axes: []solvedAxis{{}}}),
		"repeated axis":   refusalOf(t, repeatedAxisResults(t)),
		"unavailable row": refusalOf(t, unavailableRowResults(t)),
		"duplicate row":   refusalOf(t, duplicateRowResults(t)),
	} {
		if !refusal.Available() {
			t.Fatalf("%s materialized a publication", name)
		}
		if refusal.Family != SolveFailureFamilyCompile || !refusal.Site.Available() || refusal.String() == "" {
			t.Fatalf("%s refusal = %s, want a sited compile-family refusal", name, refusal)
		}
	}

	if _, failure := materialized.republish(resultLaneNone, syntheticRowKey(1), syntheticAnswerValue(1), syntheticNextStamp); !failure.Available() {
		t.Fatal("a republication of an unnamed lane published a snapshot")
	}
	if _, failure := materialized.republish(resultLaneQuery, identity.ContentID{}, syntheticAnswerValue(1), syntheticNextStamp); !failure.Available() {
		t.Fatal("a republication of an unavailable key published a snapshot")
	}
	if _, failure := materialized.republish(resultLaneQuery, syntheticRowKey(1), syntheticAnswerValue(1), state.completion.serial); !failure.Available() {
		t.Fatal("a republication that does not advance the generation published a snapshot")
	}
	if _, failure := (SolvedSnapshot{}).republish(resultLaneQuery, syntheticRowKey(1), nil, syntheticNextStamp); !failure.Available() {
		t.Fatal("an unpublished materialization republished a snapshot")
	}
}

func failureOf(t *testing.T, solver *Solver, state *State) SolveFailure {
	t.Helper()
	_, failure := materializeCompletedState(solver, state)
	return failure
}

func refusalOf(t *testing.T, results solvedResults) SolveFailure {
	t.Helper()
	_, failure := materializeSolvedResults(results)
	return failure
}

func repeatedAxisResults(t *testing.T) solvedResults {
	t.Helper()
	results := syntheticSolvedResults(t, 2)
	results.axes[1].lane = resultLaneQuery
	return results
}

func unavailableRowResults(t *testing.T) solvedResults {
	t.Helper()
	results := syntheticSolvedResults(t, 2)
	results.axes[0].rows[0].key = identity.ContentID{}
	return results
}

func duplicateRowResults(t *testing.T) solvedResults {
	t.Helper()
	results := syntheticSolvedResults(t, 2)
	results.axes[0].rows[1].key = results.axes[0].rows[0].key
	return results
}

// syntheticSolvedResults states one materializer input directly: a query axis
// of rows answered rows and a one-row observation axis. It exists so the cost
// and outcome laws can state a column width no solve fixture declares.
func syntheticSolvedResults(t testing.TB, rows int) solvedResults {
	t.Helper()
	query := solvedAxis{lane: resultLaneQuery, rows: make([]solvedRow, 0, rows)}
	for index := 0; index < rows; index++ {
		query.rows = append(query.rows, solvedRow{key: syntheticRowKey(uint64(index)), value: syntheticAnswerValue(uint64(index) * 3)})
	}
	observation := solvedAxis{lane: resultLaneObservation, rows: []solvedRow{
		{key: syntheticRowKey(1 << 40), value: syntheticAnswerValue(1 << 20)},
	}}
	return solvedResults{
		schema:     syntheticRowSchema,
		store:      syntheticRowStore,
		generation: syntheticFirstStamp,
		axes:       []solvedAxis{query, observation},
	}
}

// syntheticRowKey encodes one row ordinal as the content identity a consumer
// addresses the row by.
func syntheticRowKey(ordinal uint64) identity.ContentID {
	var key identity.ContentID
	binary.BigEndian.PutUint64(key[:8], ordinal+1)
	return key
}

// syntheticAnswerValue publishes one scalar result value under a freezer whose
// fingerprint is a function of the value, so a changed answer is a changed
// content identity.
func syntheticAnswerValue(value uint64) solvedValue {
	same := func(held uint64) uint64 { return held }
	return &typedFrozenValue[uint64]{value: value, freeze: FrozenResult[uint64]{
		Semantic:    coldKey(970_101),
		Freeze:      same,
		Clone:       same,
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(held uint64) uint64 { return held*0x9e3779b97f4a7c15 + 1 },
	}}
}
