package oracle

import (
	"testing"

	"github.com/wippyai/go-lua/analysis"
)

// The ascent boundary law.
//
// Only a region head is admitted as a Region; every point the region encloses
// exact-replaces its complete RHS on each pass. A head's one-step image is
// therefore the image of a vector whose interior components are assigned rather
// than accumulated, and that image is free to move between passes: a loop whose
// body strongly updates an accumulator publishes the literal it wrote this pass,
// not the join of every pass. The ascent itself is carried by the head
// publication, where Widen bounds the new row above the current point, and by
// the candidate boundary, which admits each interior replacement on its own
// upper bound.
//
// A head law that additionally demands the image include the previous pass's
// image contradicts both of those admissions and leaves the region no way
// forward: the interior replacement is admitted, the head refuses it, and the
// solve stops with a refusal rather than a verdict. These fixtures are the
// minimal programs of that shape, so each states the law by converging.
//
// accumulator-bounded is the smallest: one counter incremented under a guard
// inside ipairs, whose loop-carried coordinate moves from one authored literal
// to the next on consecutive passes.
func TestRegionAscentAdmitsAMovedHeadImage(t *testing.T) {
	for _, name := range []string{
		"narrowing-recovery/accumulator-bounded",
		"placement/list-inbox-clean",
		"regression/deadlock-dataflow-node",
		"realworld/transactional-saga-orchestrator",
	} {
		t.Run(name, func(t *testing.T) {
			run := corpusHarnessFixtureRun(t, name, corpusHarnessDiagnosticMode())
			if run.status != analysis.AnalyzeComplete {
				t.Fatalf("%s did not converge: status=%s engine=%s", name, corpusHarnessStatusName(run.status), corpusHarnessEngineFailure(run.solveDiagnostics))
			}
		})
	}
}

// TestRegionAscentAdmitsAMovedAuthoredCoverage is the same boundary on the
// authorship axis. An interior producer whose read loses presence over part of
// its guard region authors less than it did on the previous pass. Inclusion is
// the premise of the lifted order, not of Kleene progress, so the candidate
// boundary must judge that movement against a defined upper bound instead of
// refusing it as an operand it cannot answer for.
func TestRegionAscentAdmitsAMovedAuthoredCoverage(t *testing.T) {
	for _, name := range []string{
		"realworld/tenant-policy-runtime",
		"realworld/tenant-policy-runtime-soundness",
	} {
		t.Run(name, func(t *testing.T) {
			run := corpusHarnessFixtureRun(t, name, corpusHarnessDiagnosticMode())
			if run.status != analysis.AnalyzeComplete {
				t.Fatalf("%s did not converge: status=%s engine=%s", name, corpusHarnessStatusName(run.status), corpusHarnessEngineFailure(run.solveDiagnostics))
			}
		})
	}
}
