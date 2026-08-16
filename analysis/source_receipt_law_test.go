package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	targetdomain "github.com/wippyai/go-lua/program/target"
	"github.com/wippyai/go-lua/program/target/profile"
)

func TestProgramReceiptCursorTraversesWithoutPublicationSliceAllocation(t *testing.T) {
	linked := directFieldHostileLink(t, `return 1`)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	receipt, receiptOK := program.SemanticSourceReceipt()
	if !shardOK || !programOK || !receiptOK || !receipt.Valid() {
		t.Fatal("Program receipt")
	}
	want := receipt.Count()
	observed := 0
	allocations := testing.AllocsPerRun(100, func() {
		cursor := receipt.Cursor()
		count := 0
		for {
			_, ok := cursor.Next()
			if !ok {
				break
			}
			count++
		}
		observed = count
	})
	if observed != want {
		t.Fatalf("receipt cursor count = %d, want %d", observed, want)
	}
	if allocations != 0 {
		t.Fatalf("receipt cursor allocated %f times; detached Publications materialized", allocations)
	}
}

func TestTypedOwnerViewsFenceForeignReceiptsAndTraverseWithoutPublicationAllocation(t *testing.T) {
	linked := directFieldHostileLink(t, `return 1`)
	boundary := linked.Boundary()
	localTarget, targetOK := boundary.Target()
	localTargetReceipt, targetReceiptOK := localTarget.SemanticSourceReceipt()
	localTargetViews, targetViewsOK := localTargetReceipt.Views()
	module := linked.Module()
	localModuleReceipt, moduleReceiptOK := module.SemanticSourceReceipt()
	localModuleViews, moduleViewsOK := localModuleReceipt.Views()
	if !targetOK || !targetReceiptOK || !targetViewsOK || !moduleReceiptOK || !moduleViewsOK {
		t.Fatal("typed owner receipts unavailable")
	}
	if localTargetReceipt.OwnerID() != localTarget.ContentID() || localTargetViews.OwnerID() != localTarget.ContentID() {
		t.Fatal("Target view escaped its Contract owner")
	}
	if localModuleReceipt.OwnerID() != module.ContentID() || localModuleViews.OwnerID() != module.ContentID() {
		t.Fatal("LinkModule view escaped its Component owner")
	}

	program, err := lower.Lower(lower.Source{Name: "foreign-owner.lua", Text: []byte(`return 1`)})
	if err != nil {
		t.Fatal(err)
	}
	foreignTarget, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	foreignLinked, err := link.Seal(&link.Spec{
		Target:  foreignTarget,
		Modules: []linkproject.Module{{Name: "main", Program: program}},
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignTargetContract, foreignTargetOK := foreignLinked.Boundary().Target()
	foreignTargetReceipt, foreignTargetReceiptOK := foreignTargetContract.SemanticSourceReceipt()
	foreignModuleReceipt, foreignModuleReceiptOK := foreignLinked.Module().SemanticSourceReceipt()
	if !foreignTargetOK || !foreignTargetReceiptOK || !foreignModuleReceiptOK {
		t.Fatal("foreign typed owner receipts unavailable")
	}
	if foreignTargetReceipt.OwnerID() == localTargetReceipt.OwnerID() || foreignModuleReceipt.OwnerID() == localModuleReceipt.OwnerID() {
		t.Fatal("foreign typed owner receipt crossed an owner identity fence")
	}
	foreignTargetViews, foreignTargetViewsOK := foreignTargetReceipt.Views()
	if !foreignTargetViewsOK || foreignTargetViews.OwnerID() == localTargetViews.OwnerID() {
		t.Fatal("foreign Target view crossed an owner identity fence")
	}

	allocations := testing.AllocsPerRun(100, func() {
		for _, view := range []targetdomain.SemanticSourceView{
			localTargetViews.Contract(), localTargetViews.Operation(), localTargetViews.ABI(), localTargetViews.Subedge(),
			localTargetViews.Callback(), localTargetViews.Binding(), localTargetViews.Resume(), localTargetViews.Spawn(),
			localTargetViews.Opaque(), localTargetViews.OperationEffect(), localTargetViews.CallbackEffect(), localTargetViews.CallbackRelease(),
			localTargetViews.Outcome(), localTargetViews.Transfer(), localTargetViews.TransferOutcome(), localTargetViews.Suspension(),
			localTargetViews.ResumeOutcome(), localTargetViews.SpawnSibling(), localTargetViews.SubedgeArgumentOrigin(), localTargetViews.CallbackResult(),
			localTargetViews.ResultAlias(), localTargetViews.Produced(), localTargetViews.ProducedCapture(), localTargetViews.FreshResult(),
			localTargetViews.Protocol(), localTargetViews.ProtocolState(), localTargetViews.ProtocolAcquisition(), localTargetViews.ProtocolTransition(),
			localTargetViews.ProtocolTransitionOutcome(), localTargetViews.ProtocolEscape(), localTargetViews.ProtocolCallbackHolder(),
			localTargetViews.Boot(), localTargetViews.BootEntry(), localTargetViews.BootMetatableAttachment(), localTargetViews.BootBinding(), localTargetViews.Gsub(),
		} {
			_ = walkTargetReceiptView(view)
		}
		for _, view := range []linkmodule.SemanticSourceView{
			localModuleViews.Module(), localModuleViews.Cache(), localModuleViews.Representative(), localModuleViews.Transport(),
			localModuleViews.AnalysisRoot(), localModuleViews.InitGeneration(), localModuleViews.InitOutcome(), localModuleViews.InitTerminal(),
		} {
			_ = walkModuleReceiptView(view)
		}
	})
	if allocations != 0 {
		t.Fatalf("typed Target/LinkModule traversal allocated %f times; Publications materialized", allocations)
	}
}

func walkTargetReceiptView(view targetdomain.SemanticSourceView) bool {
	for index := 0; index < view.Count(); index++ {
		id, ok := view.DigestAt(index)
		if !ok || !id.Available() {
			return false
		}
	}
	_, beyond := view.DigestAt(view.Count())
	return !beyond
}

func walkModuleReceiptView(view linkmodule.SemanticSourceView) bool {
	for index := 0; index < view.Count(); index++ {
		id, ok := view.DigestAt(index)
		if !ok || !id.Available() {
			return false
		}
	}
	_, beyond := view.DigestAt(view.Count())
	return !beyond
}
