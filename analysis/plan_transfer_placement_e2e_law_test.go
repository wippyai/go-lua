package analysis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/result"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementpublication "github.com/wippyai/go-lua/domain/placement/publication"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestCompiledPlanProcessSendTransferPublishesExactSharedHeapPlacement is the
// mounted composite cut for an actor/thread transfer. The source is one
// module, the Target Contract is the canonical process.send declaration, and
// the assertion reads only Placement's typed detached publication. The
// authored payload root is resolved from Heap's canonical allocation origin;
// a transfer route may widen that exact key to SharedHeap, but cannot add an
// unrelated root or manufacture Unknown.
func TestCompiledPlanProcessSendTransferPublishesExactSharedHeapPlacement(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("seal canonical Target: %v", err)
	}
	op, opOK := target.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"process"},
		Member:    []string{"send"},
	})
	if !opOK || target.Operations.TransferCount(op) != 1 {
		t.Fatalf("canonical process.send operation = %d/%t transfers=%d, want one transfer", op, opOK, target.Operations.TransferCount(op))
	}
	transferID, transferOK := target.Operations.TransferIDAt(op, 0)
	endpoint, payload, alias, identityValue, capabilities, declarationOK := target.Operations.TransferDeclaration(transferID)
	wantPayload := vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 0}
	if !transferOK || !declarationOK || endpoint != (vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}) ||
		payload != wantPayload || alias != wantPayload ||
		identityValue != vocabulary.TransferIdentityUnspecified || capabilities != vocabulary.TransferCapabilitiesUnspecified {
		t.Fatalf("canonical process.send transfer = %d/%t declaration=%t endpoint=%#v payload=%#v alias=%#v identity=%d capabilities=%d; want authenticated external ValuesVar transfer", transferID, transferOK, declarationOK, endpoint, payload, alias, identityValue, capabilities)
	}
	// send answers the module's value/error pair, so its sealed outcome set is
	// the throw arm followed by the two correlated normal arms: the arm
	// answering true delivers, the arm answering the error rejects, and a send
	// that raised delivered nothing.
	if count := target.Operations.TransferOutcomeCount(op, 0); count != 3 {
		t.Fatalf("canonical process.send transfer outcomes = %d, want three", count)
	}
	for outcome, want := range []vocabulary.TransferPossibility{vocabulary.TransferMayReject, vocabulary.TransferMayDeliver, vocabulary.TransferMayReject} {
		if _, possibility, outcomeOK := target.Operations.TransferOutcomeAt(op, 0, outcome); !outcomeOK || possibility != want {
			t.Fatalf("canonical process.send outcome %d = %d/%t, want %d", outcome, possibility, outcomeOK, want)
		}
	}

	linked, err := testfixture.SealSource(target, "process-send-transfer.lua", []byte(`
local process = require("process")
local payload = { value = 1 }
process.send("worker", "topic", payload)
return true
`))
	if err != nil {
		t.Fatalf("seal one-module process.send Link: %v", err)
	}
	plan, compileStatus, diagnostics := CompileWithDiagnostics(linked)
	if plan == nil {
		t.Fatalf("compile process.send Link = %v diagnostics=%+v", compileStatus, diagnostics)
	}
	t.Cleanup(func() {
		if !plan.Close() {
			t.Error("close process.send Plan")
		}
	})
	if compileStatus != CompileComplete {
		t.Fatalf("compile process.send Link = %v diagnostics=%+v", compileStatus, diagnostics)
	}
	placementSchema, schemaOK := plan.PlacementSchema()
	if !schemaOK || !placementSchema.Valid() || placementSchema.DenseKeyCount() == 0 {
		t.Fatalf("process.send Placement schema = %t/%t/%d, want owner-issued allocation roots", schemaOK, placementSchema.Valid(), placementSchema.DenseKeyCount())
	}
	var rootID identity.ContentID
	authoredRoots := 0
	for index := 0; index < placementSchema.DenseKeyCount(); index++ {
		candidate, candidateOK := placementSchema.KeyAt(index)
		if !candidateOK {
			t.Fatalf("process.send Placement schema root %d is unavailable", index)
		}
		_, _, _, _, _, originOK := placementSchema.Heap().AllocationOriginForKey(candidate)
		valueID, valueOK := placementSchema.Heap().AllocationRootValueID(candidate)
		if !originOK || !valueOK || !valueID.Available() {
			continue
		}
		candidateID, candidateIDOK := placementSchema.Heap().KeyID(candidate)
		if !candidateIDOK || !candidateID.Available() {
			t.Fatalf("process.send authored payload root %d has no canonical Heap key identity", index)
		}
		authoredRoots++
		rootID = candidateID
	}
	if authoredRoots != 1 || !rootID.Available() {
		t.Fatalf("process.send authored payload roots = %d, want one canonical Heap root", authoredRoots)
	}
	analysisResult, solveStatus := plan.Solve(context.Background())
	if solveStatus != AnalyzeComplete || analysisResult == nil {
		t.Fatalf("solve process.send Plan = %v/%v", solveStatus, analysisResult)
	}
	if analysisResult.BodyCount() != 1 {
		t.Fatalf("process.send detached bodies = %d, want one mounted body", analysisResult.BodyCount())
	}
	body, bodyOK := analysisResult.BodyAt(0)
	if !bodyOK || body.RootCount() == 0 {
		t.Fatalf("process.send detached roots = %t/%d, want payload roots", bodyOK, body.RootCount())
	}
	family, familyOK := placementpublication.Open(analysisResult)
	if !familyOK || family.QueryCount() == 0 {
		t.Fatalf("process.send typed Placement family = %t/%d, want published query rows", familyOK, family.QueryCount())
	}
	sendPoint := transferCallEffectPoint(t, plan)
	sendHits := 0
	for queryIndex := 0; queryIndex < family.QueryCount(); queryIndex++ {
		query, queryOK := family.QueryAt(queryIndex)
		if !queryOK {
			t.Fatalf("process.send Placement query %d is not addressable", queryIndex)
		}
		pointID, pointOK := query.PointID()
		if !pointOK {
			t.Fatalf("process.send Placement query %d has no point identity", queryIndex)
		}
		if pointID != sendPoint {
			continue
		}
		sendHits++
		switch query.Status() {
		case result.QueryHit:
			// continue below
		default:
			t.Fatalf("process.send Placement send point status = %v, want hit", query.Status())
		}
		summary, summaryOK := query.Placement(placementSchema)
		if !summaryOK || !summary.Available() || summary.AllocationCount() != placementSchema.DenseKeyCount() {
			t.Fatalf("process.send Placement query %d summary = %t/%t/%d, want exact allocation denominator %d", queryIndex, summaryOK, summary.Available(), summary.AllocationCount(), placementSchema.DenseKeyCount())
		}
		allocation, allocationOK := summary.Allocation(rootID)
		if !allocationOK || !allocation.Available() {
			t.Fatalf("process.send Placement query %d omitted canonical payload root %s", queryIndex, rootID)
		}
		if gotID := allocation.AllocationID(); gotID != rootID || !gotID.Available() {
			t.Fatalf("process.send Placement query %d allocation identity = %s, want %s", queryIndex, gotID, rootID)
		}
		fact, factOK := allocation.Fact()
		wantFact := placementdomain.Fact{Class: placementdomain.SharedHeap, RetainEscape: placementdomain.EvidenceProven}
		if !factOK || fact != wantFact {
			t.Fatalf("process.send Placement send point payload fact = %v/%t, want %v", fact, factOK, wantFact)
		}
	}
	if sendHits != 1 {
		t.Fatalf("process.send Placement send point query count = %d, want one", sendHits)
	}
}

func transferCallEffectPoint(t testing.TB, plan *Plan) identity.ContentID {
	t.Helper()
	if plan == nil || plan.state == nil || plan.state.artifacts == nil {
		t.Fatal("process.send transfer has no compiled artifact authority")
	}
	var point identity.ContentID
	for _, mount := range plan.state.artifacts.mounts {
		program := mount.Program.Program
		callCount, callPublished := program.CallCount()
		if !callPublished {
			t.Fatal("process.send artifact has no Call family")
		}
		for callIndex := 0; callIndex < callCount; callIndex++ {
			call, callOK := program.CallAt(callIndex)
			if !callOK || call.ArgumentCount() != 3 {
				continue
			}
			occurrenceOrdinal, ordinalOK := program.OccurrenceOrdinalForID(programschema.OccurrenceCall, call.ID())
			if !ordinalOK {
				t.Fatalf("process.send call %s has no Program occurrence ordinal", call.ID())
			}
			ruleCount, rulesPublished := program.RuleOccurrenceCount()
			if !rulesPublished {
				t.Fatal("process.send artifact has no RuleOccurrence family")
			}
			for ruleIndex := 0; ruleIndex < ruleCount; ruleIndex++ {
				row, rowOK := program.RuleOccurrenceAt(ruleIndex)
				rowOccurrence, occurrenceOK := row.Occurrence()
				if !rowOK || !occurrenceOK || int(rowOccurrence) != occurrenceOrdinal || row.Key() != "placement-transfer" || row.Stage() != programissuance.StageCallEffect {
					continue
				}
				candidate := row.PointID()
				if !candidate.Available() {
					t.Fatal("placement-transfer send row has no call-effect point")
				}
				if point.Available() {
					t.Fatalf("multiple three-actual calls have placement-transfer call-effect points (%s, %s)", point, candidate)
				}
				point = candidate
			}
		}
	}
	if !point.Available() {
		t.Fatal("process.send has no placement-transfer call-effect point")
	}
	return point
}
