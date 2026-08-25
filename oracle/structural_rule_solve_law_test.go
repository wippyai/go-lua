package oracle

import "testing"

// This file states the consequence of a structural publication at the only
// altitude where it is observable: a program whose call sites activate reaches
// a complete analysis.
//
// A structural rule computes no fact. Its publication is the activation row
// set its candidate branches mount into the construct topology, so its graph
// members carry no write at all. Every other rule's members carry exactly one.
// The runtime binding catalog reads the write side of every graph member while
// it binds the Factor plane, and it is the only pass that does so: the compile
// path never builds it. A catalog that recognizes only the two writing
// dispositions therefore refuses a structural member's group, the plane never
// binds, and the whole solve stops at the program's input fence - while
// compilation of the same source reports complete.
//
// The fixtures below are the activation-heavy ones: a supervisor whose handlers
// recurse, and a callback whose send crosses a placement boundary. Both mount
// several activated call sites, so a structural member reaches the catalog on
// both. Keeping the law over the fixtures rather than over the predicate is
// deliberate - the predicate states the shape, this states the consequence a
// future catalog change would silently take away.
func TestAStructurallyPublishingRuleCompletesItsSolve(t *testing.T) {
	for _, fixture := range []string{
		"semantic/actor-supervisor-recursive-app",
		"semantic/deep-placement-callback-send",
	} {
		t.Run(fixture, func(t *testing.T) {
			run, class, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, fixture), corpusHarnessDiagnosticMode())
			if err != nil {
				t.Fatalf("fixture %s did not complete its solve (%s): %v", fixture, class, err)
			}
			if run == nil {
				t.Fatalf("fixture %s produced no run", fixture)
			}
		})
	}
}
