package oracle

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
)

// This file is the corpus spine's Workspace law. Its subject is the analyzer
// lifetime the fixture spine compiles through, and the two properties that
// lifetime has to hold at once.
//
// The first is compute-once. A Workspace seals one analyzer composition and
// owns one content-addressed compiler product directory for its whole life, so
// a spine that opens a Workspace per fixture pays for the entire analyzer
// schema, and for every product of every shared library, once per fixture. The
// production lint and LSP process does the opposite: one long-lived Workspace,
// many compiles.
//
// The second is that sharing changes nothing an analysis says. A cache may only
// answer with the exact product a cold compile would have sealed, so a fixture
// compiled after other fixtures must publish the byte-identical Result and the
// same verdict it publishes alone. If a warm Workspace ever moved a fixture's
// answer, that would be a soundness defect in the product cache's identity, not
// a harness inconvenience.

// corpusHarnessWorkspaceLawFixtures are four fixtures from four families. They
// are compiled in both orders, so a product cache that let one fixture's
// compile inform another's answer is separated from one that only avoids
// repeating work.
var corpusHarnessWorkspaceLawFixtures = []string{
	"frame-local/pure-scratch-table",
	"functions/callback-closure-counter",
	"narrowing/and-or-truthiness-narrow",
	"transitive-libs/shared-lib-divergent-consumers",
}

// corpusHarnessWorkspaceLawSignature is one fixture's complete observable
// answer: the public status, the classified verdict, the detached Result's
// content identity, and the engine evidence of the solve that produced it.
// Result content identity is derived from the whole detached publication, so
// two equal signatures are two byte-identical analyses.
func corpusHarnessWorkspaceLawSignature(run *corpusHarnessRun, class string, err error) string {
	status := corpusHarnessStatusName(run.status)
	content := "unavailable"
	bodies := 0
	if run.result != nil {
		content = run.result.ContentID().String()
		bodies = run.result.BodyCount()
	}
	failure := "none"
	if err != nil {
		failure = err.Error()
	}
	engine := run.solveDiagnostics.Engine
	return fmt.Sprintf("status=%s class=%q result=%s bodies=%d epochs=%d passes=%d evaluates=%d failures=%d folds=%d restarts=%d activations=%d error=%q",
		status, class, content, bodies,
		engine.Epochs, engine.EpochPasses, engine.Evaluates, engine.EvaluateFailures,
		engine.Folds, engine.Restarts, engine.Activations, failure)
}

func corpusHarnessWorkspaceLawMode(lifetime corpusHarnessWorkspaceLifetime) corpusHarnessMode {
	mode := corpusHarnessDiagnosticMode()
	mode.workspace = lifetime
	return mode
}

// TestCorpusFixtureAnalysisIsIndependentOfCompileOrder is the shared-Workspace
// soundness law. Each fixture is first analyzed alone in a private Workspace,
// which is the coldest compile the analyzer has. The same fixtures are then
// analyzed through the shared Workspace in declaration order and again in
// reverse order, so every one of them is compiled behind a cache warmed by
// different neighbours. All three answers must be identical.
//
// The three orders share one private baseline and are not a per-fixture
// property, so the whole comparison runs as one subtest: a pattern that does
// not name it selects none of the compiles below.
func TestCorpusFixtureAnalysisIsIndependentOfCompileOrder(t *testing.T) {
	t.Run("law", func(t *testing.T) {
		cold := make(map[string]string, len(corpusHarnessWorkspaceLawFixtures))
		for _, name := range corpusHarnessWorkspaceLawFixtures {
			project := corpusHarnessFixture(t, name)
			run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessWorkspaceLawMode(corpusHarnessWorkspacePerFixture))
			cold[name] = corpusHarnessWorkspaceLawSignature(run, class, err)
		}
		orders := map[string][]string{
			"forward": corpusHarnessWorkspaceLawFixtures,
			"reverse": nil,
		}
		for index := len(corpusHarnessWorkspaceLawFixtures) - 1; index >= 0; index-- {
			orders["reverse"] = append(orders["reverse"], corpusHarnessWorkspaceLawFixtures[index])
		}
		for _, order := range []string{"forward", "reverse"} {
			for _, name := range orders[order] {
				project := corpusHarnessFixture(t, name)
				run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessWorkspaceLawMode(corpusHarnessWorkspaceShared))
				shared := corpusHarnessWorkspaceLawSignature(run, class, err)
				if shared != cold[name] {
					t.Errorf("fixture %s analyzed differently through the shared Workspace in %s order\n  private: %s\n  shared:  %s",
						name, order, cold[name], shared)
				}
			}
		}
	})
}

// TestCorpusSpineSealsOneCompositionForTheWholeRun is the compute-once law. The
// analyzer schema is compiled from a compile-time constant declaration
// inventory, so a fixture spine has no reason to seal a second one. Every run
// the spine produces must therefore carry the shared Workspace's own
// compilation instance, which is the same authority its Plans were bound to.
func TestCorpusSpineSealsOneCompositionForTheWholeRun(t *testing.T) {
	workspace := corpusHarnessSharedWorkspace(t)
	shared := workspace.Compilation()
	if !shared.Available() || shared.Schema() == nil {
		t.Fatal("shared corpus Workspace sealed no analyzer composition")
	}
	for _, name := range corpusHarnessWorkspaceLawFixtures {
		name := name
		t.Run(name, func(t *testing.T) {
			corpusSpineSealsOneCompositionFixture(t, name, shared)
		})
	}
}

func corpusSpineSealsOneCompositionFixture(t *testing.T, name string, shared composite.Compilation) {
	project := corpusHarnessFixture(t, name)
	run, _, _ := corpusHarnessExecuteDetached(t, project, corpusHarnessWorkspaceLawMode(corpusHarnessWorkspaceShared))
	if !run.compilation.Available() {
		t.Fatalf("fixture %s ran with no sealed composition", name)
	}
	if run.compilation.Schema() != shared.Schema() {
		t.Errorf("fixture %s sealed its own analyzer composition instead of reading its Workspace's", name)
	}
}

// TestCorpusPrivateWorkspaceKeepsOneSemanticIdentity is the other half of the
// compute-once law. A private Workspace is an independent environment and holds
// its own engine schema instance, exactly as the composition law requires, and
// that independence is never a semantic difference: the two environments seal
// one execution schema identity, so a fixture judged in either is judged
// against the same analyzer.
func TestCorpusPrivateWorkspaceKeepsOneSemanticIdentity(t *testing.T) {
	shared := corpusHarnessSharedWorkspace(t).Compilation()
	project := corpusHarnessFixture(t, corpusHarnessWorkspaceLawFixtures[0])
	run, class, err := corpusHarnessExecuteDetached(t, project, corpusHarnessWorkspaceLawMode(corpusHarnessWorkspacePerFixture))
	if err != nil {
		t.Fatalf("private Workspace fixture %s: %s: %v", project.name, class, err)
	}
	private := run.compilation
	if !private.Available() || private.Schema() == nil {
		t.Fatal("private Workspace run carried no sealed composition")
	}
	if private.Schema() == shared.Schema() {
		t.Fatal("a private Workspace shared the shared Workspace's engine schema instance")
	}
	if private.Digest() != shared.Digest() || private.ExecutionSchemaID() != shared.ExecutionSchemaID() {
		t.Fatal("independent Workspaces sealed different analyzer semantic identities")
	}
}
