package targetfixture

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestDebugTmp(t *testing.T) {
	fixture := newLawSpec(t)
	world := Build(t, fixture.spec)
	t.Logf("mounted=%v view=%v base=%v manager=%p", world.Mounted().Available(), world.View().Available(), world.Base().Available(), world.View().Manager())
	t.Logf("scopes=%d", len(world.Mounted().Scopes()))
	initial := fixture.spec.Initials[0]
	scope, ok := world.Mounted().Scope(initial.Scope)
	if !ok {
		t.Fatal("scope")
	}
	scopeToken, ok := world.Mounted().ScopeToken(scope)
	if !ok {
		t.Fatal("scope token")
	}
	provenance, ok := targetInitialProvenance(world.Mounted(), initial.Operation)
	if !ok {
		t.Fatal("provenance")
	}
	executor, ok := apply.Prepare(world.Mounted(), initial.Operation.Identity(), scope)
	if !ok {
		t.Fatal("executor")
	}
	frame, ok := binding.NewFrame(scopeToken)
	if !ok {
		t.Fatal("frame")
	}
	application, ok := executor.Invoke(frame, provenance, binding.NewOwnerNamedDestination(initial.Operation.Outputs()[0].Relation))
	if !ok {
		t.Fatal("invoke")
	}
	batch, ok := transaction.NewSubmissionBatch(application, witness.WideningPermit{}, nil)
	if !ok {
		t.Fatal("batch")
	}
	proposals, ok := application.Proposals()
	if !ok {
		t.Fatal("application proposals")
	}
	t.Logf("app=%v outcome=%v proposals=%v len=%d batch=%v batchlen=%d", application.Available(), application.Outcome(), proposals.Available(), proposals.Len(), batch.Available(), batch.Len())
	for index := 0; index < proposals.Len(); index++ {
		proposal, ok := proposals.At(index)
		if !ok {
			t.Fatal("proposal at")
		}
		coord, coordOK := world.View().Resolve(proposal.Destination())
		version, versionOK := world.Base().Store().Column(proposal.Destination().Column())
		fmt.Printf("dest=%v coord=%v coordOK=%v manager=%p valid=%v version=%v guards=%p same=%v\n", proposal.Destination().Available(), coord.Available(), coordOK, coord.Mask().Manager(), coord.Mask().Valid(), versionOK, version.Guards(), versionOK && coord.Mask().Manager() == version.Guards())
		readScratch := store.NewReadScratch(world.View().Manager())
		completed, valid := world.Base().Store().Read(proposal.Destination().Column(), coord.Dense(), coord.Mask(), readScratch, func(part store.ReadPart) bool {
			fmt.Printf("part key=%v regionValid=%v empty=%v presence=%v value=%v lineage=%v type=%v\n", part.Key(), part.Region().Valid(), support.Empty(part.Region()), part.Presence(), part.Value().Available(), part.Lineage().Available(), part.Type().Available())
			return true
		})
		fmt.Printf("read completed=%v valid=%v\n", completed, valid)
	}
	scratch := store.NewReadScratch(world.View().Manager())
	prepared, preparedOK := transaction.Prepare(world.Base(), world.View(), scratch, batch)
	t.Logf("prepared=%v preparedOK=%v", prepared.Available(), preparedOK)
}
