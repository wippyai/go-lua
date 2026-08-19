package oracle

import "testing"

// The real-fixture lane names individual corpus fixtures whose analysis cost
// or termination is worth reporting on its own, outside a shard walk. It runs
// one compile and one diagnostic solve through the corpus spine and reports
// the fixture's measured cost; a work cutoff is a failed analysis here, never
// a passed sample, so the lane carries no work budget and the bounded runner
// remains the only resource authority.
func analyzeCanonicalRealFixture(t *testing.T, name string) {
	t.Helper()
	run := corpusHarnessFixtureRun(t, name, corpusHarnessDiagnosticMode())
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
