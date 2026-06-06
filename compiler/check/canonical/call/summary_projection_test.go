package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSummaryProjectionForTargetsUsesClosureEntryContext(t *testing.T) {
	t.Parallel()

	direct := summary.FuncRef{GraphID: 1}
	closureRef := summary.FuncRef{GraphID: 2}
	entryCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)},
	})
	entryRefs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(11), "fn").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 20}))
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: closureRef.GraphID, ParentHash: closureRef.ParentHash}, entryCells, entryRefs)
	targets := NewTargetSet([]summary.FuncRef{direct}, true, []flow.ClosureRef{closure}, true)

	calledDirect := false
	projection, selection := SummaryProjectionForTargets(
		targets,
		func(target SelectedTarget) EntryContext {
			if target.IsClosure() {
				closure, _ := target.Closure()
				return NewEntryContext(target.Ref(), closure.EntryCells(), closure.EntryFunctionRefs(), closure.EntryClosureRefs(), summary.EntryValues{
					0: product.FromType(typ.Boolean),
				}, flow.BoundaryFactsDomain.Top())
			}
			calledDirect = true
			return NewEntryContext(target.Ref(), flow.CaptureCellsDomain.Bottom(), nil, nil, nil, flow.BoundaryFactsDomain.Top())
		},
		func(ctx EntryContext) summary.Summary {
			if got := ctx.Ref(); got != closureRef {
				t.Fatalf("lookup ref = %#v, want closure ref %#v", got, closureRef)
			}
			if got := ctx.CaptureCells(); !flow.CaptureCellsDomain.Equal(got, entryCells) {
				t.Fatalf("lookup cells = %s, want %s", got.Format(), entryCells.Format())
			}
			if got := ctx.FunctionRefs(); !flow.FunctionRefsDomain.Equal(got, entryRefs) {
				t.Fatalf("lookup function refs = %#v, want %#v", got, entryRefs)
			}
			if got := ctx.EntryValues(); !product.Domain.Equal(got[0], product.FromType(typ.Boolean)) {
				t.Fatalf("lookup entry value slot 0 = %#v, want boolean", got[0])
			}
			return summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.Number)}}
		},
		SummaryTargetInfo{
			DeclaredReturns: func(target SelectedTarget) bool {
				return target.IsClosure()
			},
			SignatureRelations: func(target SelectedTarget) flow.ReturnRelations {
				if target.IsClosure() {
					return flow.ReturnRelationsDomain.Top()
				}
				return flow.ReturnRelationsOfErrorReturns([]flow.ReturnCorrelation{{ValueIndex: 0, ErrorIndex: 1}})
			},
		},
	)

	if calledDirect {
		t.Fatal("direct entry context used despite finite closure target")
	}
	if !selection.HasClosureTargets() {
		t.Fatal("selection did not preserve closure-target classification")
	}
	if len(projection.Targets) != 1 {
		t.Fatalf("projection targets = %d, want 1", len(projection.Targets))
	}
	if !projection.Targets[0].DeclaredReturns {
		t.Fatal("per-target declared-return metadata was not applied")
	}
	if !flow.ReturnRelationsDomain.Equal(projection.Targets[0].SignatureRelations, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("closure signature fallback = %#v, want top", projection.Targets[0].SignatureRelations)
	}
}

func TestSummaryProjectionForTargetsPreservesSelectionFallbackState(t *testing.T) {
	t.Parallel()

	direct := summary.FuncRef{GraphID: 7}
	targets := NewTargetSet([]summary.FuncRef{direct}, true, nil, true)
	projection, selection := SummaryProjectionForTargets(
		targets,
		func(target SelectedTarget) EntryContext {
			return NewEntryContext(target.Ref(), flow.CaptureCellsDomain.Bottom(), nil, nil, nil, flow.BoundaryFactsDomain.Top())
		},
		func(EntryContext) summary.Summary {
			return summary.Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}}
		},
		SummaryTargetInfo{},
	)

	if selection.BlocksTypeFallback() {
		t.Fatal("closure-top with finite direct targets should not block type fallback")
	}
	if !selection.AllowsCallbackFallback() {
		t.Fatal("finite direct targets with no closure target should allow callback fallback")
	}
	if len(projection.Targets) != 1 || projection.Targets[0].Ref != direct {
		t.Fatalf("projection targets = %#v, want direct ref %#v", projection.Targets, direct)
	}
}
