package oracle

import "testing"

// This file states what a rule may not do to the axis it publishes into, at
// the only altitude where the consequence is observable: a recursive program
// reaches a complete analysis.
//
// Call dispatch refines the Call cell of the application it is indexed by. For
// a while it did so as a routed publication that SELECTED the Call axis to
// find the cells it would write, which reads as harmless - the destination of
// every route was the candidate's own coordinate anyway. It is not harmless. A
// routed fact is staged under the region of the cell the row observed, so a
// rule reading the axis it writes derives its own authored region from its own
// output; as the fixpoint narrows that cell the region narrows with it, and an
// operator whose authored region shrinks between ascent steps is not monotone.
// The widen gate refuses it, correctly, and the solve stops at
// refresh/region-merge with a Call coordinate authored under a region the
// exact rebuild no longer covers.
//
// bench/fibonacci is where it showed: a self-recursive function is exactly the
// program whose dispatch cell is still moving while its own rule reads it. The
// declaration now reads Value's image of the callee and publishes exactly at
// the candidate's own coordinate, so the alternatives a callee reaches are the
// judgment's own business and no Call read feeds the rule that writes Call.
//
// Keeping this law over the fixture rather than over the declaration is
// deliberate. The declaration test next to the rule states the shape; this
// states the consequence, which is what a future declaration change would
// silently take away.
func TestARecursiveCallSiteCompletesItsSolve(t *testing.T) {
	const fixture = "bench/fibonacci"
	run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, fixture), corpusHarnessDiagnosticMode())
	if err != nil {
		t.Fatalf("fixture %s did not complete its solve (%s): %v", fixture, class, err)
	}
	if run == nil {
		t.Fatalf("fixture %s produced no run", fixture)
	}
}
