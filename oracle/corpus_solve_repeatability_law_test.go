package oracle

import "testing"

// This file is the corpus spine's repeatability law. Its subject is what a
// solve is a function of: the sealed Program and the sealed analyzer schema,
// and nothing else. Nothing a Go process varies between one execution of a
// range statement and the next belongs in that function - map iteration order,
// allocation addresses, pool residue, garbage-collection timing, goroutine
// interleaving. A solver whose counters move under any of those is reading a
// coordinate it was never given, and every measurement taken from it, every
// before-and-after comparison of a solver change, is noise around an unknown
// mean.
//
// The compile-order law next to this one states that a warm Workspace answers
// with the product a cold compile would have sealed. This law states the other
// half: the same fixture, solved again under the same conditions, answers
// identically. Together they close the space in which a solve counter can move
// without the Program or the schema moving.
//
// The signature compared here is the whole observable answer - public status,
// classified verdict, detached Result content identity, and every engine
// counter - so a repetition that differs anywhere is reported, not only one
// that differs in the counter a reader happened to be watching.

// corpusSolveRepeatabilityLawFixtures are five fixtures chosen for the two
// shapes a repetition defect hides in. Three of them - the discriminant,
// dispatch, and composed-chain narrowings - complete their solve at an engine
// refusal, which is the path where the boundary a fixture stops at is decided
// while the work queue is still draining, so an order-sensitive fold would move
// the counters by one. The other two run their fixpoint to convergence, where
// an order-sensitive merge would move the pass count instead.
var corpusSolveRepeatabilityLawFixtures = []string{
	"bench/fibonacci",
	"narrowing/accumulated-array-local-surface",
	"narrowing/inherited-field-discriminant",
	"narrowing/keyed-component-composed-chain",
	"narrowing/type-eq-multivariant-dispatch",
}

// corpusSolveRepeatabilityLawRounds is how many times one fixture is solved.
// Go re-randomizes map iteration order on every range statement, so each round
// is an independent sample of every order a map-fed queue, activation, or merge
// could be given.
const corpusSolveRepeatabilityLawRounds = 3

// TestCorpusFixtureSolveIsRepeatable states the property for both Workspace
// lifetimes. "warm" solves each fixture repeatedly through the one shared
// Workspace: every round after the first reads the same sealed compiler
// products the round before it sealed, so the rounds differ in nothing the
// analyzer is allowed to see, and their answers must be one answer. "cold"
// states the same property for a Workspace of its own per round, which seals
// its own analyzer composition and its own compiler products and reaches the
// solver through allocations no earlier round touched: a cold answer that
// moves between rounds is reading its own heap, not its Program.
//
// Each fixture is its own subtest under each lifetime, so a pattern naming one
// fixture solves only that fixture, in only the lifetime it names.
func TestCorpusFixtureSolveIsRepeatable(t *testing.T) {
	t.Run("warm", func(t *testing.T) { corpusSolveRepeatabilityLawRun(t, corpusHarnessWorkspaceShared) })
	t.Run("cold", func(t *testing.T) { corpusSolveRepeatabilityLawRun(t, corpusHarnessWorkspacePerFixture) })
}

func corpusSolveRepeatabilityLawRun(t *testing.T, lifetime corpusHarnessWorkspaceLifetime) {
	for _, name := range corpusSolveRepeatabilityLawFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			project := corpusHarnessFixture(t, name)
			first := ""
			for round := 0; round < corpusSolveRepeatabilityLawRounds; round++ {
				run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessWorkspaceLawMode(lifetime))
				signature := corpusHarnessWorkspaceLawSignature(run, class, err)
				if round == 0 {
					first = signature
					continue
				}
				if signature != first {
					t.Errorf("fixture %s answered differently on solve %d of %d\n  first: %s\n  again: %s",
						name, round+1, corpusSolveRepeatabilityLawRounds, first, signature)
				}
			}
		})
	}
}
