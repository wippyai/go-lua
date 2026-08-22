package oracle

import "testing"

// typestateLifecycleAcceptanceFixtures are the fixture projects whose whole
// contract is the typestate/lifecycle/channel diagnostic family. They are named
// here so the family can be judged on its own without walking the full corpus;
// the full acceptance walk still judges them along with every other fixture.
//
// Each diagnostic code in the family is carried by a positive fixture that
// states the exact finding and by a negative fixture whose closest-correct
// program states the zero-diagnostic contract for the same code.
var typestateLifecycleAcceptanceFixtures = []string{
	// channel.send.closed, channel.close.closed
	"semantic/channel-lifecycle-typestate",
	"semantic/channel-lifecycle-clean-path",
	// typestate.invalid_requirement, typestate.invalid_transition,
	// typestate.unproven_requirement, effect.lifecycle.unreleased
	"semantic/declared-resource-lifecycle",
	"semantic/resource-lifecycle-escape",
	"semantic/resource-lifecycle-clean-path",
	"semantic/resource-cross-protocol-coupling",
}

// TestTypestateLifecycleAcceptance runs the canonical acceptance judgment over
// the typestate/lifecycle fixtures alone. It shares the acceptance mode with
// TestCanonicalCorpusSemanticAcceptance, so a fixture that passes here passes
// there for the same reason.
func TestTypestateLifecycleAcceptance(t *testing.T) {
	projects := make([]corpusHarnessProject, 0, len(typestateLifecycleAcceptanceFixtures))
	for _, name := range typestateLifecycleAcceptanceFixtures {
		projects = append(projects, corpusHarnessFixture(t, name))
	}
	corpusHarnessWalk(t, projects, corpusSemanticAcceptanceMode())
}
