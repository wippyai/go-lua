package oracle

import (
	"testing"

	engineprobe "github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/domain/value"
)

// The real-fixture lane names individual corpus fixtures whose analysis cost
// or termination is worth reporting on its own, outside a shard walk. It runs
// one compile and one diagnostic solve through the corpus spine and reports
// the fixture's measured cost; a work cutoff is a failed analysis here, never
// a passed sample, so the lane carries no work budget and the bounded runner
// remains the only resource authority.
func analyzeCanonicalRealFixture(t *testing.T, name string) {
	t.Helper()
	value.DbgValueReset()
	engineprobe.DbgEngineReset()
	engineprobe.DbgMergeReset()
	run := corpusHarnessFixtureRun(t, name, corpusHarnessDiagnosticMode())
	t.Logf("canonical real fixture %s value: owns=%d valid=%d validRows=%d maxRows=%d leq=%d join=%d joinBuild=%d equal=%d",
		name, value.DbgValue().Owns, value.DbgValue().Valid, value.DbgValue().ValidRows, value.DbgValue().MaxRows,
		value.DbgValue().LessOrEq, value.DbgValue().Join, value.DbgValue().JoinBuild, value.DbgValue().Equal)
	t.Logf("canonical real fixture %s fold: folds=%d terms=%d maxTerms=%d reuseAdmit=%d reuseRefuse=%d reuseTerms=%d rebuildTerms=%d",
		name, engineprobe.DbgEngine().Folds, engineprobe.DbgEngine().FoldTerms, engineprobe.DbgEngine().FoldMaxTerms,
		engineprobe.DbgEngine().ReuseAdmit, engineprobe.DbgEngine().ReuseRefuse, engineprobe.DbgEngine().ReuseTerms, engineprobe.DbgEngine().RebuildTerms)
	t.Logf("canonical real fixture %s reuse refusal: notAscent=%d noAccumulator=%d pendingUnknown=%d pendingDescend=%d notOwned=%d changedRow=%d reasons=%v direction=%v",
		name, engineprobe.DbgEngine().RefuseNotAscent, engineprobe.DbgEngine().RefuseNoAccumulator,
		engineprobe.DbgEngine().RefusePendingUnknown, engineprobe.DbgEngine().RefusePendingDescend,
		engineprobe.DbgEngine().RefuseNotOwned, engineprobe.DbgEngine().RefuseChangedRow,
		engineprobe.DbgEngine().RefuseReasons, engineprobe.DbgEngine().RefuseDirection)
	mergeMany, cells, cellPairs, cellWidth, maxOperand := engineprobe.DbgMerge()
	t.Logf("canonical real fixture %s merge: mergeMany=%d cells=%d cellPairs=%d cellWidth=%d maxOperand=%d",
		name, mergeMany, cells, cellPairs, cellWidth, maxOperand)
	engine := run.solveDiagnostics.Engine
	t.Logf("canonical real fixture %s: seal=%s compile=%s solve=%s total=%s", name, run.cost.seal, run.cost.compile, run.cost.solve, run.cost.total())
	t.Logf("canonical real fixture %s engine: epochs=%d revisions=%d passes=%d refreshes=%d evaluates=%d evalFails=%d folds=%d rhs=%d restarts=%d activations=%d maxQueue=%d maxEpisode=%d pubs=%d semanticPubs=%d rawPubs=%d rawOnly=%d bumps=%d wakes=%d wakesByReason=%v ifaceRefresh=%d ifaceDone=%d ifaceFallback=%d leq=%d geq=%d eq=%d incomp=%d unknown=%d",
		name, engine.Epochs, engine.Revisions, engine.EpochPasses, engine.Refreshes, engine.Evaluates, engine.EvaluateFailures,
		engine.Folds, engine.RegionRHS, engine.Restarts, engine.Activations, engine.MaxQueue, engine.MaxEpisode,
		engine.Publications, engine.SemanticPublications, engine.RawPublications, engine.RawOnlyPublications, engine.VersionBumps,
		engine.Wakes, engine.WakesByReason, engine.InterfaceRefreshes, engine.InterfaceRefreshCompleted, engine.InterfaceRefreshFallbacks,
		engine.InterfaceRefreshOldLessEqNew, engine.InterfaceRefreshNewLessEqOld, engine.InterfaceRefreshEqual,
		engine.InterfaceRefreshIncomparable, engine.InterfaceRefreshUnknown)
}

func TestDiagnosticCanonicalDeadlockCompilerLua(t *testing.T) {
	// Measured 2026-08-17: one epoch, 940 WTO passes, 12743 point refreshes,
	// 2093 region-interface refreshes of which 2057 are OldLessEqNew, 8926
	// version bumps, ~40s solve. The solve terminates; it does not cycle
	// (rawOnly=31, restarts=4, maxEpisode=2). Time is a Kleene climb: the
	// region interface grows by one lawful inclusion step per refresh instead
	// of widening to a stable bound, so each new heap/table face re-enqueues
	// the head and walks the whole WTO schedule again. A few-ms solve needs
	// that interface to widen, not increment.
	analyzeCanonicalRealFixture(t, "regression/deadlock-compiler-lua")
}

func TestDiagnosticCanonicalDeadlockDataflowNode(t *testing.T) {
	analyzeCanonicalRealFixture(t, "regression/deadlock-dataflow-node")
}

// TestDiagnosticCanonicalOpaqueCalleeArrayRevocation pins the RawGet staged
// semantic-source route. The read is declared over rawSourceTag; a route
// emitted under a different tag type is refused by the staged sink, which
// fails the whole solve at execution/preflight rather than at a named domain
// boundary.
func TestDiagnosticCanonicalOpaqueCalleeArrayRevocation(t *testing.T) {
	analyzeCanonicalRealFixture(t, "soundness/opaque-callee-array-revocation")
}

func TestDiagnosticCanonicalAdviceShapePolymorphic(t *testing.T) {
	analyzeCanonicalRealFixture(t, "advice/shape-polymorphic")
}

func TestDiagnosticCanonicalControlForLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "core/control-for-loop")
}

func TestDiagnosticCanonicalBreakOutsideLoop(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/break-outside-loop")
}

func TestDiagnosticCanonicalBackwardGoto(t *testing.T) {
	analyzeCanonicalRealFixture(t, "functions/goto-backward")
}
