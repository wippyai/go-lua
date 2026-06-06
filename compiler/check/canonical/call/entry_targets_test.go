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

func TestSummaryEntryTargetsWithLiveContextUsesClosureTargetsWhenFinite(t *testing.T) {
	t.Parallel()

	directRef := summary.FuncRef{GraphID: 1}
	liveCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(13), Value: product.FromType(typ.Boolean)},
	})
	cells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)},
	})
	refs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(11), "fn").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 20}))
	closures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(12), "cl").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 30}, cells, refs),
	))
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 2}, cells, refs, closures)
	targets := NewTargetSet([]summary.FuncRef{directRef}, true, []flow.ClosureRef{closure}, true)

	calledDirect := false
	out := SummaryEntryTargetsWithLiveContext(targets, func(ref summary.FuncRef) EntryContext {
		if ref == directRef {
			calledDirect = true
		}
		return NewEntryContext(ref, liveCells, nil, nil, nil)
	})

	if calledDirect {
		t.Fatal("direct context builder called despite finite closure targets")
	}
	if len(out) != 1 {
		t.Fatalf("SummaryEntryTargetsWithLiveContext len = %d, want 1", len(out))
	}
	if got := out[0].Ref; got.GraphID != 2 {
		t.Fatalf("closure context ref = %#v, want graph 2", got)
	}
	if got := out[0].EntryCells; !flow.CaptureCellsDomain.Equal(got, flow.OverlayCaptureCells(cells, liveCells)) {
		t.Fatalf("closure context cells = %s, want captured/live overlay", got.Format())
	}
	if got := out[0].EntryFunctionRefs; !flow.FunctionRefsDomain.Equal(got, refs) {
		t.Fatalf("closure context function refs = %#v, want %#v", got, refs)
	}
	if got := out[0].EntryClosureRefs; !flow.ClosureRefsDomain.Equal(got, closures) {
		t.Fatalf("closure context closure refs = %#v, want %#v", got, closures)
	}
}

func TestSummaryEntryTargetsUseDirectWhenClosureTopHasNoFiniteTargets(t *testing.T) {
	t.Parallel()

	directRef := summary.FuncRef{GraphID: 7, ParentHash: 9}
	cells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(1), Value: product.FromType(typ.Number)},
	})
	refs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(2), "fn").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 8}))
	closures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(3), "cl").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 9}, cells, refs),
	))
	targets := NewTargetSet([]summary.FuncRef{directRef}, true, nil, true)

	out := SummaryEntryTargetsWithLiveContext(targets, func(ref summary.FuncRef) EntryContext {
		return NewEntryContext(ref, cells, refs, closures, nil)
	})

	if len(out) != 1 {
		t.Fatalf("SummaryEntryTargetsWithLiveContext len = %d, want 1", len(out))
	}
	if out[0].Ref != directRef {
		t.Fatalf("target ref = %#v, want %#v", out[0].Ref, directRef)
	}
	if got := out[0].EntryCells; !flow.CaptureCellsDomain.Equal(got, cells) {
		t.Fatalf("target cells = %s, want %s", got.Format(), cells.Format())
	}
	if got := out[0].EntryFunctionRefs; !flow.FunctionRefsDomain.Equal(got, refs) {
		t.Fatalf("target function refs = %#v, want %#v", got, refs)
	}
	if got := out[0].EntryClosureRefs; !flow.ClosureRefsDomain.Equal(got, closures) {
		t.Fatalf("target closure refs = %#v, want %#v", got, closures)
	}
}

func TestSummaryEntryTargetsWithLiveContextOverlaysClosureTargets(t *testing.T) {
	t.Parallel()

	callee := summary.FuncRef{GraphID: 2}
	capturedCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.String)},
	})
	liveCells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: cfg.SymbolID(10), Value: product.FromType(typ.Number)},
		{Symbol: cfg.SymbolID(11), Value: product.FromType(typ.Boolean)},
	})
	capturedRefs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(20), "captured").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 30}))
	liveRefs := flow.WithFunctionRef(nil, constraint.NewPath(cfg.SymbolID(20), "captured").Key(), flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 31}))
	capturedClosures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(21), "captured").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 40}, nil, nil),
	))
	liveClosures := flow.WithClosureRef(nil, constraint.NewPath(cfg.SymbolID(22), "live").Key(), flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 41}, nil, nil),
	))
	facts := flow.BoundaryFactsOf(nil, nil, nil, nil, []flow.BoundaryLengthLowerBound{{
		Target: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0},
		Lower:  1,
	}}, nil)
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: callee.GraphID}, capturedCells, capturedRefs, capturedClosures)
	targets := NewTargetSet(nil, false, []flow.ClosureRef{closure}, true)

	out := SummaryEntryTargetsWithLiveContext(targets, func(ref summary.FuncRef) EntryContext {
		return NewEntryContextWithFacts(ref, liveCells, liveRefs, liveClosures, nil, facts)
	})

	if len(out) != 1 {
		t.Fatalf("SummaryEntryTargetsWithLiveContext len = %d, want 1", len(out))
	}
	if got := out[0].EntryCells; !flow.CaptureCellsDomain.Equal(got, flow.OverlayCaptureCells(capturedCells, liveCells)) {
		t.Fatalf("target cells = %s, want live overlay", got.Format())
	}
	if got := out[0].EntryFunctionRefs; !flow.FunctionRefsDomain.Equal(got, flow.OverlayFunctionRefs(capturedRefs, liveRefs)) {
		t.Fatalf("target function refs = %#v, want live overlay", got)
	}
	if got := out[0].EntryClosureRefs; !flow.ClosureRefsDomain.Equal(got, flow.OverlayClosureRefs(capturedClosures, liveClosures)) {
		t.Fatalf("target closure refs = %#v, want live overlay", got)
	}
	if got := out[0].EntryFacts; !got.HasProof() {
		t.Fatal("target entry facts dropped live proof")
	}
}
