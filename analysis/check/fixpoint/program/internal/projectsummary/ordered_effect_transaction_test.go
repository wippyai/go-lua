package projectsummary

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestConcreteStaticAllocationMutationRequiresStaticMemberEffect(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `
function build()
	local result = table.create()
	result["k"] = "v"
	return result
end`)
	result, err := body.CheckFunction(fn, body.Config{
		Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	got := FromResult(result)
	if len(got.Returns) != 1 || len(got.HeapTableObjects) != 1 {
		t.Fatalf("concrete summary omitted returned allocation transaction: %#v", got)
	}
	id, ok := product.Get(reg, got.Returns[0], identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		t.Fatalf("return/fresh identity diverged: return=%v/%v fresh=%#v", id, ok, got.FreshHeapAllocations)
	}
	object, ok := got.HeapTableObjects[id]
	if !ok || len(object.StaticMembers()) == 0 {
		t.Fatalf("concrete solve omitted static-key heap mutation: id=%v object=%#v", id, object)
	}
	if !product.Equal(reg, object.Root(), got.Returns[0]) {
		rootType, _ := typevalue.TypeOf(reg, object.Root())
		returnType, _ := typevalue.TypeOf(reg, got.Returns[0])
		t.Logf("oracle root differs from return: root=%v return=%v", rootType, returnType)
	}
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	shape := transformer.Shape{Globals: uint32(len(plan.BoundaryGlobals()))}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	reason := relation.ContextualReason()
	if !strings.Contains(reason, "PathAssignments") || !strings.Contains(reason, "PathStaticMemberWrites") {
		t.Fatalf("static mutation contextual reason = %q, want distinct PathAssignment + PathStaticMemberWrite effect vocabulary", reason)
	}
}

func TestAllocationOnlyTransformerMatchesAuthenticConcreteSummaryAndDigest(t *testing.T) {
	reg := standard.Registry()
	fn := parseBranchTransformerFunction(t, `function build()
		local result = table.create()
		return result
	end`)
	prepared, err := body.PrepareFunction(fn, body.Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatal(err)
	}
	stats := &body.Stats{}
	plan := prepared.OperationPlan()
	shape := transformer.Shape{Globals: uint32(len(plan.BoundaryGlobals()))}
	relation := transformer.NewPlanCompiler().Compile(reg, prepared.Graph(), plan, shape)
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("allocation-only relation contextual: %s", reason)
	}
	if stats.BodySolves != 0 {
		t.Fatalf("relation build ran %d body solves", stats.BodySolves)
	}
	cursor, _ := transformer.NewBindingCursor(shape, make([]product.Value, len(plan.BoundaryGlobals())), nil)
	lowered, exact := relation.SpecializeWithEffects(cursor, nil, transformer.SpecializationContext{}, ResolvedEffectSummaryResolver(reg))
	if !exact {
		t.Fatal("allocation-only relation failed specialization")
	}
	if stats.BodySolves != 0 {
		t.Fatalf("relation specialization ran %d body solves", stats.BodySolves)
	}
	concrete, err := body.SolvePrepared(prepared, body.SolveConfig{Stats: stats})
	if err != nil {
		t.Fatalf("SolvePrepared concrete oracle: %v", err)
	}
	if stats.BodySolves != 1 {
		t.Fatalf("canonical oracle body solves = %d, want 1", stats.BodySolves)
	}
	got := FromResult(concrete)
	if !summary.Equal(reg, lowered, got) {
		t.Fatalf("allocation-only relation differs from concrete summary:\n got=%#v\nwant=%#v", lowered, got)
	}
	if loweredDigest, concreteDigest := summary.NormalizedPayloadDigest(reg, lowered), summary.NormalizedPayloadDigest(reg, got); loweredDigest != concreteDigest {
		t.Fatalf("allocation-only payload digest = %v, want %v", loweredDigest, concreteDigest)
	}

	caller := parseBranchTransformerFunction(t, `function caller()
		return build()
	end`)
	callerPrepared, err := body.PrepareFunction(caller, body.Config{Registry: reg, Globals: []string{"build"}})
	if err != nil {
		t.Fatalf("PrepareFunction caller: %v", err)
	}
	gotResult := solveDynamicIndexCaller(t, callerPrepared, state.State{}, lowered)
	wantResult := solveDynamicIndexCaller(t, callerPrepared, state.State{}, got)
	point := dynamicIndexTransformerCallPoint(t, gotResult)
	wantPoint := dynamicIndexTransformerCallPoint(t, wantResult)
	gotOutcome, gotOK := gotResult.CallOutcomeAt(point)
	wantOutcome, wantOK := wantResult.CallOutcomeAt(wantPoint)
	if gotOK != wantOK || !reflect.DeepEqual(gotOutcome, wantOutcome) {
		t.Fatalf("allocation-only production CallOutcome differs:\n got=%#v\nwant=%#v", gotOutcome, wantOutcome)
	}
	gotState, gotOK := gotResult.StateAtBoundary(point)
	wantState, wantOK := wantResult.StateAtBoundary(wantPoint)
	if gotOK != wantOK {
		t.Fatalf("allocation-only post-call state presence = %v/%v", gotOK, wantOK)
	}
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("state lane catalog = %d, want 17", len(lanes))
	}
	for _, lane := range lanes {
		domain, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		if gotOK && !domain.Equal(gotState, wantState) {
			t.Fatalf("allocation-only caller state differs on lane %q", lane)
		}
	}
	gotDiagnostics, err := json.Marshal(diagnostics.Produce(gotResult))
	if err != nil {
		t.Fatal(err)
	}
	wantDiagnostics, err := json.Marshal(diagnostics.Produce(wantResult))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotDiagnostics, wantDiagnostics) {
		t.Fatalf("allocation-only diagnostics differ:\n got=%s\nwant=%s", gotDiagnostics, wantDiagnostics)
	}
}
