package analysis

import (
	"testing"

	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// The Link composition publication is the one writer of the engine-published
// prefix the sealed catalog declares for the composition. Snapshot slots are
// dense: a declared column no publisher fills is a hole in that range, and a
// publication with a hole seals nothing, so the whole analysis refuses at the
// composition step carrying no column to name.
//
// The coverage is therefore stated here, against the catalog's own slot
// assignment, so a composition axis declared without a publisher names itself
// instead of erasing the compile.
func TestLinkCompositionPublishesEveryDeclaredCompositionColumn(t *testing.T) {
	plan, status, diagnostics := CompileWithDiagnostics(mustLink(t, `return 1`, fixtureContract(t)))
	if plan != nil {
		defer plan.Close()
	}
	if status != CompileComplete || plan == nil || plan.state == nil {
		t.Fatalf("compiling one mounted module refused: phase %s composition %s axis %q",
			diagnostics.Phase, diagnostics.Composition, diagnostics.CompositionAxis)
	}
	publication, publicationOK := plan.state.compilation.Publication()
	if !publicationOK {
		t.Fatal("the sealed compilation carries no publication plan")
	}
	sealed := plan.state.composition.Columns()
	for _, output := range []schema.Key{
		selectapply.OutputKey,
		programmount.OutputKey,
		modulecomposition.ImportOutputKey,
		modulecomposition.CacheOutputKey,
		modulecomposition.ModuleCallTransitionOutputKey,
		modulecomposition.GenerationOutputKey,
		modulecomposition.OutcomeOutputKey,
		modulecomposition.ModuleReturnStateEdgeOutputKey,
		modulecomposition.TerminalOutputKey,
		modulecomposition.ModuleExportCallableOriginOutputKey,
		modulecomposition.ModuleExportCallableIngressOutputKey,
	} {
		address, projected := analysiscatalog.ProjectAxis[identity.ContentID, struct{}](publication, output)
		if !projected {
			t.Fatalf("the sealed catalog declares no column for %s", output)
		}
		if int(address.Slot) >= sealed {
			t.Fatalf("the composition publication never wrote %s: slot %d of %d sealed columns", output, address.Slot, sealed)
		}
	}
}
