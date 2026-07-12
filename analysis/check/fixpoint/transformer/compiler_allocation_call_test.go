package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPlanCompilerAllocationCallSharesReturnAndHeapTransaction(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	ref := factflow.ExprRef(1)
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	source, _ := factflow.NewCallValueSource(ref, 0, 0, 0, callPoint, shape)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true,
		ExprRef: ref, HasExpr: true, Final: true, Adjusted: true,
		ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, path.Path{})},
	})
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	callOp, _ := operationplan.NewSignatureCallOperation(sig)
	template, _ := effectlowering.StaticSignatureAllocationTemplate(sig)
	allocationOp, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 41, Template: template.Root, Ordinal: uint32(callPoint),
	}, template)
	plan := operationplan.New(graph, factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{callPoint: site},
		Returns:   map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})},
	}).WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: callOp}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{callPoint: allocationOp})
	relation := NewPlanCompiler().Compile(reg, graph, plan, Shape{})
	if reason := relation.ContextualReason(); reason != "" {
		t.Fatalf("allocation relation contextual: %s", reason)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	got, exact := relation.SpecializeWithEffects(cursor, nil, SpecializationContext{}, func(effects []ResolvedEffect) (summary.Summary, bool) {
		if len(effects) != 1 || effects[0].Kind != EffectAllocationTemplate {
			return summary.Summary{}, false
		}
		allocation := effects[0].Allocation
		ks := keyspace.New()
		materialized, ok := effectlowering.MaterializeStaticAllocation(reg, nil, ks, cfg.Point(allocation.Site.Ordinal), allocation.Template.Template())
		if !ok {
			return summary.Summary{}, false
		}
		fresh := make([]summary.FreshHeapAllocation, 0, len(materialized.Placements))
		for id, placement := range materialized.Placements {
			fresh = append(fresh, summary.FreshHeapAllocation{ID: id, Placement: placement})
		}
		return summary.Summary{HeapTableObjects: materialized.Objects, FreshHeapAllocations: fresh, HeapKeySpace: ks}, true
	})
	if !exact || len(got.Returns) != 1 || len(got.HeapTableObjects) != 1 || len(got.FreshHeapAllocations) != 1 {
		t.Fatalf("allocation specialization exact=%v summary=%#v", exact, got)
	}
	returnID, _ := product.Get(reg, got.Returns[0], identity.Key).ID()
	if returnID == (identity.ID{}) || got.FreshHeapAllocations[0].ID != returnID {
		t.Fatalf("return/fresh allocation identity diverged: %v/%v", returnID, got.FreshHeapAllocations)
	}
}
