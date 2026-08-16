package analysis

import (
	"fmt"
	"strings"
	"testing"
)

// The census is the honest status shard of the frozen corpus. It judges Link
// admission and the public detached Result contract of the one-shot Analyze
// entry; diagnostics and fixture expectations belong to the acceptance mode of
// the same harness. Every shard below is one harness walk, so census and
// acceptance can never run over different corpora.

// TestCanonicalFrozenCorpusNativeCensus is a bounded diagnostic shard of the
// same frozen-corpus contract. It is not a fixture exception: every selected
// project still requires AnalyzeComplete, and the full 911-row law remains
// authoritative below. Keeping the shard named lets architectural failures
// converge without repeatedly spending the full-corpus safety budget.
func TestCanonicalFrozenCorpusNativeCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "native/")
}

func TestCanonicalFrozenCorpusCoreCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "core/")
}

func TestCanonicalFrozenCorpusAdviceCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "advice/")
}

func TestCanonicalFrozenCorpusFunctionsCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "functions/")
}

func TestCanonicalFrozenCorpusSemanticCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "semantic/")
}

func TestCanonicalFrozenCorpusTypesCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "types/")
}

func TestCanonicalFrozenCorpusNarrowingCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "narrowing/")
}

func TestCanonicalFrozenCorpusRegressionCensus(t *testing.T) {
	testCanonicalFrozenCorpusPrefix(t, "regression/")
}

func testCanonicalFrozenCorpusPrefix(t *testing.T, prefix string) {
	t.Helper()
	outcomes := corpusHarnessWalk(t, corpusHarnessShard(t, prefix), corpusHarnessCensusMode())
	reportCanonicalFrozenCorpus(t, prefix, outcomes)
}

// TestCanonicalFrozenCorpusCensus is the whole-corpus status law. Each fixture
// reports as its own named subtest; the shard receipt below records the status
// census and the cost that produced it.
func TestCanonicalFrozenCorpusCensus(t *testing.T) {
	outcomes := corpusHarnessWalk(t, corpusHarnessProjects(t), corpusHarnessCensusMode())
	reportCanonicalFrozenCorpus(t, "census", outcomes)
}

// reportCanonicalFrozenCorpus emits the shard receipt. Failures already failed
// their own fixture subtest, so this grouped report is evidence rather than a
// second verdict.
func reportCanonicalFrozenCorpus(t *testing.T, shard string, outcomes []corpusHarnessOutcome) {
	t.Helper()
	t.Log(corpusHarnessShardReceipt(shard, outcomes))
	if failure := corpusHarnessFailureReport(outcomes, 12); failure != "" {
		t.Log(failure)
	}
}

func TestCanonicalCorpusFailureFormattingIsGroupedAndBounded(t *testing.T) {
	if workers := corpusHarnessWorkerCount(1000000); workers < 1 || workers > corpusHarnessMaxWorkers {
		t.Fatalf("corpus worker bound changed: %d", workers)
	}
	if corpusHarnessWorkerCount(0) != 0 || corpusHarnessWorkerCount(1) != 1 {
		t.Fatal("corpus worker bound lost empty/singleton totality")
	}
	outcomes := []corpusHarnessOutcome{
		{project: "a", class: "incomplete", err: fmt.Errorf("one")},
		{project: "b", class: "incomplete", err: fmt.Errorf("two")},
		{project: "c", class: "incomplete", err: fmt.Errorf("three")},
		{project: "d", class: "link", err: fmt.Errorf("four")},
	}
	report := corpusHarnessFailureReport(outcomes, 2)
	for _, want := range []string{"incomplete: 3", "a (one)", "b (two)", "... 1 more", "link: 1", "d (four)"} {
		if !strings.Contains(report, want) {
			t.Fatalf("grouped report omitted %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "c (three)") {
		t.Fatalf("grouped report exceeded its per-class detail budget:\n%s", report)
	}
}
