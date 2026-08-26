package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// TestSuspensionYieldBoundaryRequiresASuspendingCallee is the boundary law of
// the suspension consumer. A subject that outlives a call crosses a yield
// boundary only when the call it is anchored at can actually suspend. Call
// owns the dynamic target set and Target owns each operation's sealed
// suspension denominator, so the answer exists only at solve time: static
// support is a may-envelope over every declared operation and admits the
// opaque alternative unconditionally.
//
// Both fixtures below allocate the same untouched witness table, keep it live
// across one call, and read it afterwards. The witness is never an actual of
// that call, so the formal rule leaves it at its allocation seed and the only
// judgment that can raise it is suspension.
//
// The provider operation declares exactly one outcome, and that outcome is
// normal. It therefore cannot suspend, the witness never has to survive a
// re-entry, and its Stack baseline stands. Raising it would be the
// suspension consumer treating every call as a yield.
func TestSuspensionYieldBoundaryRequiresASuspendingCallee(t *testing.T) {
	target := sealCompositeFormalTarget(t)
	synchronous := suspensionBoundaryWitnessClasses(t, target, "suspension-synchronous-boundary", `
local formal = require("formal-placement")
local first = { value = 1 }
local second = { value = 2 }
local third = { value = 3 }
local witness = { value = 4 }
formal.owned(first, second, third)
return witness.value
`)
	if len(synchronous) == 0 {
		t.Fatal("synchronous boundary fixture published no Placement summary for the witness allocation")
	}
	for point, fact := range synchronous {
		if fact.Class != placementdomain.Stack {
			t.Fatalf("witness across a provably synchronous call at point %s = %s, want its Stack baseline", point, fact)
		}
	}
}

// TestSuspensionYieldBoundaryHoldsForASuspendingCallee is the dual. The
// callee here declares a yield outcome and a re-entry, so Target seals a
// non-empty suspension denominator for it. The witness must survive that
// re-entry and the consumer must raise it: the gate narrows the boundary set
// to the calls that can actually suspend, it does not remove the judgment.
func TestSuspensionYieldBoundaryHoldsForASuspendingCallee(t *testing.T) {
	target := sealCompositeFormalTarget(t)
	suspending := suspensionBoundaryWitnessClasses(t, target, "suspension-suspending-boundary", `
local witness = { value = 4 }
coroutine.yield()
return witness.value
`)
	if len(suspending) == 0 {
		t.Fatal("suspending boundary fixture published no Placement summary for the witness allocation")
	}
	raised := 0
	for _, fact := range suspending {
		if fact.Class == placementdomain.OwnedHeap {
			raised++
		}
		if fact.Class == placementdomain.SharedHeap || fact.Class == placementdomain.Unknown {
			t.Fatalf("witness across a suspending callee = %s, want at most the OwnedHeap survival demand", fact)
		}
	}
	if raised == 0 {
		t.Fatal("witness across a declared suspending callee was never raised: the boundary gate refused a real yield")
	}
}

// suspensionBoundaryWitnessClasses solves one fixture and returns the typed
// Placement summary fact published for its witness allocation at every
// selected Placement query point. The witness is the last Program allocation
// root of the fixture: the source authors it last and the artifact preserves
// that order.
func suspensionBoundaryWitnessClasses(t testing.TB, target *contract.Contract, name, source string) map[identity.ContentID]placementdomain.Fact {
	t.Helper()
	record, failure, mounted := mountFormalTargetRecord(t, target, name, source)
	if !mounted {
		if failure.Stage() == artifactcompiler.CompileStageOccurrences && failure.Reason() == artifactcompiler.CompileReasonOccurrenceUnavailable {
			t.Skipf("suspension boundary law cannot execute: artifact occurrence-unavailable; failure=%s", failure.Error())
		}
		t.Fatalf("compile and mount suspension boundary fixture: %s", failure.Error())
	}
	roots := formalAllocationRoots(t, record)
	if len(roots) == 0 {
		t.Fatal("suspension boundary fixture sealed no Program allocation roots")
	}
	witness := roots[len(roots)-1].id

	bound := materializerBinding(t, record)
	committed, table := queryCanonicalProgram(t, record, bound)
	sealed, sealFailure, sealedOK := committed.Seal(nil)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal suspension boundary program: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve suspension boundary program: status=%v reason=%v failure=%v rule=%v", solveStatus, solveReport.Reason(), solveReport.Failure(), solveReport.Rule())
	}
	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("suspension boundary solve published no snapshot")
	}
	view := published.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	if !queryPlanOK {
		t.Fatal("open typed Placement query publication")
	}
	publications, publicationsOK := bound.QueryPublications(committed, table)
	if !publicationsOK {
		t.Fatal("resolve typed Placement query publications")
	}
	facts := make(map[identity.ContentID]placementdomain.Fact)
	for _, publication := range publications {
		if publication.Site.Family != QueryFamilyPlacementSummary {
			continue
		}
		answer, answerStatus := snapshot.Query(&view, queryPlan, publication.Key)
		if answerStatus == snapshot.ReadProvenAbsent {
			continue
		}
		if answerStatus != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("typed Placement publication status = %s, want a hit", answerStatus)
		}
		cell, cellOK := publication.CanonicalCell(answer)
		if !cellOK || !cell.Available() {
			t.Fatal("Placement answer did not close under its typed contract")
		}
		result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
		if !resultOK || !result.Available() {
			t.Fatal("typed Placement publication did not decode under the mounted schema")
		}
		rows := decodeFormalPlacementRows(t, result)
		fact, present := rows[witness]
		if !present {
			t.Fatalf("Placement point %s omitted the witness allocation root %s", publication.Site.Point, witness)
		}
		facts[publication.Site.Point] = fact
	}
	return facts
}
