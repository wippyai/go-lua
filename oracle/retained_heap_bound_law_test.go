package oracle

import (
	"fmt"
	"runtime"
	"testing"
)

// This file is the acceptance path's retained-memory gate. Its subject is not
// how much a corpus run allocates; it is how much a corpus run still owns once
// every Plan it compiled has been closed.
//
// The distinction is the whole point. Go reports RSS and TotalAlloc as
// high-water marks, so a path that allocates gigabytes transiently and frees
// all of it reads identically to a path that retains gigabytes forever. Only
// live heap measured after a full close separates them, and only a series of
// such measurements over increasing fixture counts separates a bounded
// one-time cost from an unbounded one.
//
// The law: analyzing more distinct fixtures in one process, closing each one,
// must not make the process own more memory. Every product of a compile is
// owned by its Workspace and released at Plan.Close; nothing keyed by fixture
// content may outlive that boundary in a process-global. A cache that is
// process-global rather than Workspace-scoped fails here by construction,
// because its retention is proportional to the number of distinct fixtures it
// has seen.
//
// Sizing is chosen so the series has statistical shape rather than a single
// before/after pair: a leak of any size appears as a straight line through
// four points, while allocator noise does not.

const (
	// retainedHeapRounds is the number of measured disjoint fixture batches.
	// Four points make a leak legible as a slope instead of a single delta.
	retainedHeapRounds = 4
	// retainedHeapBatchFixtures is how many distinct fixtures each batch
	// analyzes and closes.
	retainedHeapBatchFixtures = 24
	// retainedHeapSettleTolerance is the absolute live-heap slack allowed
	// between the first and last measured batch. It covers allocator
	// bookkeeping, span fragmentation, and lazily grown runtime structures,
	// and is far below the per-batch cost of retaining even one compiled
	// topology per fixture.
	retainedHeapSettleTolerance = 24 << 20
)

// TestSequentialCorpusFixturesRetainNoHeapAfterClose is the standing gate on
// acceptance-path retention. It analyzes disjoint batches of frozen corpus
// fixtures sequentially in one process, closing every compiled Plan
// synchronously, and asserts that live heap measured after each batch does not
// grow with the number of fixtures analyzed.
//
// It fails, by design, the moment any owner starts retaining per-fixture
// products past Plan.Close: a process-global artifact or product cache, a
// finalizer-reliant close that never runs, a memo keyed by fixture content
// held outside a Workspace, or a registry that installs one row per analyzed
// program. Those all read as a positive slope here and as nothing at all in an
// RSS or TotalAlloc measurement.
//
// A batch is not required to analyze cleanly. A fixture that fails to compile
// or solve still exercises construction and close, and its retention is just
// as much a defect as a passing fixture's. Correctness of the corpus is the
// census lane's subject, not this one's; this gate only refuses to measure a
// corpus that produced no successful analysis at all, which would let it pass
// without having exercised the path it guards.
// The batches are sequential and cumulative by construction - later rounds
// are judged against the live heap earlier rounds left behind - so the whole
// series is one subtest: a pattern that does not name it runs no batch below.
func TestSequentialCorpusFixturesRetainNoHeapAfterClose(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		batches := retainedHeapBatches(t)
		// The measured boundary is the compile's own: a fixture's products must be
		// gone once its Plan and the Workspace that owns them are closed. Each
		// fixture therefore runs in a private Workspace it closes itself. The
		// shared Workspace of a long-lived process answers a different question -
		// it retains its products until Close by declared design - and would report
		// its own reuse cache as this gate's leak.
		mode := corpusHarnessMode{name: "retained-heap", execution: corpusHarnessDiagnosticSolve, options: corpusHarnessSolveOptions(), workspace: corpusHarnessWorkspacePerFixture}

		// The warmup batch pays every one-time cost the analyzer defers to first
		// use: the sealed standard-library target, engine templates, package-level
		// singletons, and the runtime's own lazily grown structures. Measuring it
		// would report bounded initialization as a leak.
		analyzed, warmupOK := retainedHeapRunBatch(t, batches[0], mode)
		if analyzed == 0 {
			t.Fatal("warmup batch analyzed no fixtures")
		}
		baseline := retainedHeapLiveBytes()
		t.Logf("warmup: %d fixtures, %d analyzed clean, live heap %s", analyzed, warmupOK, retainedHeapBytes(baseline))

		series := make([]uint64, 0, retainedHeapRounds)
		cumulative := 0
		cleanTotal := 0
		for round := 0; round < retainedHeapRounds; round++ {
			count, clean := retainedHeapRunBatch(t, batches[round+1], mode)
			cumulative += count
			cleanTotal += clean
			live := retainedHeapLiveBytes()
			series = append(series, live)
			t.Logf("batch %d: %d fixtures (%d cumulative, %d analyzed clean), live heap %s, delta from baseline %s",
				round+1, count, cumulative, clean, retainedHeapBytes(live), retainedHeapSignedBytes(int64(live)-int64(baseline)))
		}
		if cleanTotal == 0 {
			t.Fatalf("no measured fixture analyzed cleanly across %d fixtures; the gate cannot certify a path it never exercised", cumulative)
		}

		first, last := series[0], series[len(series)-1]
		if last <= first {
			return
		}
		growth := last - first
		if growth <= retainedHeapSettleTolerance {
			return
		}
		perFixture := growth / uint64(cumulative-len(batches[1]))
		t.Fatalf("acceptance path retains heap across sequential fixtures: live heap grew %s (%s after batch 1, %s after batch %d, about %s per additional fixture) "+
			"against a %s tolerance, with every compiled Plan closed. Series: %s. "+
			"An owner is holding per-fixture products past Plan.Close; find the owner and release at its lifecycle boundary rather than capping workers, evicting a cache, or reducing fixtures",
			retainedHeapBytes(growth), retainedHeapBytes(first), retainedHeapBytes(last), retainedHeapRounds, retainedHeapBytes(perFixture),
			retainedHeapBytes(retainedHeapSettleTolerance), retainedHeapSeries(series))
	})
}

// retainedHeapBatches selects retainedHeapRounds+1 disjoint fixture batches
// strided across the whole frozen corpus, so every batch carries comparable
// construction weight and no batch is a single fixture family.
func retainedHeapBatches(t *testing.T) [][]corpusHarnessProject {
	t.Helper()
	projects := corpusHarnessProjects(t)
	batchCount := retainedHeapRounds + 1
	needed := batchCount * retainedHeapBatchFixtures
	if len(projects) < needed {
		t.Fatalf("frozen corpus holds %d fixtures, need %d for %d batches of %d", len(projects), needed, batchCount, retainedHeapBatchFixtures)
	}
	stride := len(projects) / needed
	batches := make([][]corpusHarnessProject, batchCount)
	for index := 0; index < needed; index++ {
		batches[index%batchCount] = append(batches[index%batchCount], projects[index*stride])
	}
	return batches
}

// retainedHeapRunBatch analyzes one batch strictly sequentially, closing each
// Plan before the next fixture is sealed, and reports how many fixtures ran
// and how many reached AnalyzeComplete.
func retainedHeapRunBatch(t *testing.T, batch []corpusHarnessProject, mode corpusHarnessMode) (analyzed, clean int) {
	t.Helper()
	for _, project := range batch {
		run, _, err := corpusHarnessExecuteDetached(t, project, mode)
		analyzed++
		if err == nil {
			clean++
		}
		// Drop every reference this batch holds before the next fixture, so a
		// live local cannot be mistaken for retention by the owner under test.
		if run != nil {
			run.linked, run.result, run.report = nil, nil, nil
		}
	}
	return analyzed, clean
}

// retainedHeapLiveBytes reports bytes still reachable after a full collection.
// The second collection lets objects freed by finalizers scheduled during the
// first one be reclaimed, so a finalizer-reliant close is measured as the
// runtime actually performs it rather than as its author intended.
func retainedHeapLiveBytes() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func retainedHeapBytes(value uint64) string {
	return fmt.Sprintf("%.1f MiB", float64(value)/(1<<20))
}

func retainedHeapSignedBytes(value int64) string {
	return fmt.Sprintf("%+.1f MiB", float64(value)/(1<<20))
}

func retainedHeapSeries(series []uint64) string {
	rendered := ""
	for index, live := range series {
		if index != 0 {
			rendered += " -> "
		}
		rendered += retainedHeapBytes(live)
	}
	return rendered
}
