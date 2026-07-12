package relationcall

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCatalogRejectsAmbiguousAndInvalidRoutes(t *testing.T) {
	target := resolverTestTarget(9001)
	tests := []struct {
		name   string
		width  int
		routes []Route
		want   string
	}{
		{name: "negative width", width: -1, want: "negative"},
		{name: "outside width", width: 1, routes: []Route{{Point: 1, Target: target}}, want: "outside"},
		{name: "ambiguous", width: 2, routes: []Route{{Point: 1, Target: target}, {Point: 1, Target: resolverTestTarget(9002)}}, want: "ambiguous"},
		{name: "zero cell", width: 1, routes: []Route{{Target: Target{SummaryKey: target.SummaryKey}}}, want: "zero cell"},
		{name: "zero summary", width: 1, routes: []Route{{Target: Target{Cell: target.Cell}}}, want: "zero summary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCatalog(test.width, test.routes); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewCatalog error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestResolverHandledMatrix(t *testing.T) {
	reg := standard.Registry()
	exact := resolverTestEmptyRelation(t, reg)
	cell := transformer.CellRef{Function: 9101}
	target := Target{Cell: cell, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(9101))}
	snapshot := resolverTestFreeze(t, cell, exact)
	catalog, err := NewCatalog(4, []Route{{Point: 2, Target: target}})
	if err != nil {
		t.Fatal(err)
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: 2, HasPoint: true}).View()
	ctx := transfer.NodeContext{Registry: reg}
	bindings := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State, transformer.Shape) (transformer.BindingCursor, bool) {
		cursor, err := transformer.NewBindingCursor(transformer.Shape{}, nil, nil)
		return cursor, err == nil
	}
	base := Config{Relations: snapshot, Catalog: &catalog, Bindings: bindings}

	if out, handled := NewResolver(base)(ctx, site, state.State{}, nil); !handled || len(out.Results) != 0 || out.MaySuspend {
		t.Fatalf("exact no-op outcome = %#v, handled=%v; no-result relation must retain routing authority", out, handled)
	}

	tests := []struct {
		name   string
		config Config
		site   factflow.CallSiteView
	}{
		{name: "missing point identity", config: base, site: factflow.NewCallSite(factflow.CallSiteConfig{}).View()},
		{name: "missing target", config: base, site: factflow.NewCallSite(factflow.CallSiteConfig{Point: 1, HasPoint: true}).View()},
		{name: "missing relation", config: Config{Catalog: &catalog, Bindings: bindings}, site: site},
		{name: "missing bindings", config: Config{Relations: snapshot, Catalog: &catalog}, site: site},
		{name: "binding failure", config: Config{Relations: snapshot, Catalog: &catalog, Bindings: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State, transformer.Shape) (transformer.BindingCursor, bool) {
			return transformer.BindingCursor{}, false
		}}, site: site},
		{name: "specialization failure", config: Config{Relations: snapshot, Catalog: &catalog, Bindings: bindings, Specialization: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (transformer.SpecializationContext, bool) {
			return transformer.SpecializationContext{}, false
		}}, site: site},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, handled := NewResolver(test.config)(ctx, test.site, state.State{}, nil)
			if handled || !out.Empty() {
				t.Fatalf("outcome = %#v, handled=%v; want fail-closed miss", out, handled)
			}
		})
	}

	contextual := transformer.NewPlanCompiler().Compile(reg, nil, nil, transformer.Shape{})
	contextualSnapshot := resolverTestFreeze(t, cell, contextual)
	out, handled := NewResolver(Config{Relations: contextualSnapshot, Catalog: &catalog, Bindings: bindings})(ctx, site, state.State{}, nil)
	if handled || !out.Empty() {
		t.Fatalf("contextual/widened relation handled=%v outcome=%#v", handled, out)
	}
}

func TestResolverRejectsEffectfulRelationWithoutEffectResolver(t *testing.T) {
	reg := standard.Registry()
	relation := resolverTestAllocationRelation(t, reg)
	cell := transformer.CellRef{Function: 9201}
	target := Target{Cell: cell, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(9201))}
	catalog, err := NewCatalog(4, []Route{{Point: 1, Target: target}})
	if err != nil {
		t.Fatal(err)
	}
	bindings := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State, transformer.Shape) (transformer.BindingCursor, bool) {
		cursor, err := transformer.NewBindingCursor(transformer.Shape{}, nil, nil)
		return cursor, err == nil
	}
	resolver := NewResolver(Config{Relations: resolverTestFreeze(t, cell, relation), Catalog: &catalog, Bindings: bindings})
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: 1, HasPoint: true}).View()
	if out, handled := resolver(transfer.NodeContext{Registry: reg}, site, state.State{}, nil); handled || !out.Empty() {
		t.Fatalf("effectful relation without resolver handled=%v outcome=%#v", handled, out)
	}
}

func TestResolverFrozenCatalogIsRaceSafe(t *testing.T) {
	reg := standard.Registry()
	cell := transformer.CellRef{Function: 9301}
	target := Target{Cell: cell, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(9301))}
	catalog, err := NewCatalog(3, []Route{{Point: 1, Target: target}})
	if err != nil {
		t.Fatal(err)
	}
	bindings := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State, transformer.Shape) (transformer.BindingCursor, bool) {
		cursor, err := transformer.NewBindingCursor(transformer.Shape{}, nil, nil)
		return cursor, err == nil
	}
	resolver := NewResolver(Config{Relations: resolverTestFreeze(t, cell, resolverTestEmptyRelation(t, reg)), Catalog: &catalog, Bindings: bindings})
	ctx := transfer.NodeContext{Registry: reg}
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: 1, HasPoint: true}).View()
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if out, handled := resolver(ctx, site, state.State{}, nil); !handled || len(out.Results) != 0 || out.MaySuspend {
					t.Errorf("concurrent resolve handled=%v outcome=%#v", handled, out)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestExclusiveResolverOwnsHandledEmptyAndFallsBackExactlyOnce(t *testing.T) {
	ctx := transfer.NodeContext{}
	site := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
	legacyCalls := 0
	legacy := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		legacyCalls++
		return callpayload.CallOutcome{MaySuspend: true}
	}
	handled := Exclusive(func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (callpayload.CallOutcome, bool) {
		return callpayload.CallOutcome{}, true
	}, legacy)
	if out := handled(ctx, site, state.State{}, nil); !out.Empty() || legacyCalls != 0 {
		t.Fatalf("handled empty outcome = %#v, legacy calls=%d", out, legacyCalls)
	}

	miss := Exclusive(func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (callpayload.CallOutcome, bool) {
		return callpayload.CallOutcome{}, false
	}, legacy)
	if out := miss(ctx, site, state.State{}, nil); !out.MaySuspend || legacyCalls != 1 {
		t.Fatalf("miss outcome = %#v, legacy calls=%d; want one", out, legacyCalls)
	}
}

func resolverTestTarget(id symbol.ID) Target {
	return Target{Cell: transformer.CellRef{Function: uint64(id)}, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(id))}
}

func resolverTestEmptyRelation(t *testing.T, reg *axis.Registry) transformer.Relation {
	t.Helper()
	graph := cfg.New()
	graph.AddEdge(graph.Entry(), graph.Exit(), false)
	return transformer.NewPlanCompiler().Compile(reg, graph, operationplan.New(graph, factflow.FactsInput{}), transformer.Shape{})
}

func resolverTestFreeze(t *testing.T, cell transformer.CellRef, relation transformer.Relation) transformer.RelationSnapshot {
	t.Helper()
	snapshot, err := transformer.FreezeAcyclicRelation(context.Background(), cell, relation)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func resolverTestAllocationRelation(t *testing.T, reg *axis.Registry) transformer.Relation {
	t.Helper()
	graph := cfg.New()
	callPoint := graph.AddNode(cfg.NodeCall)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), callPoint, false)
	graph.AddEdge(callPoint, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	refID := factflow.ExprRef(1)
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	source, _ := factflow.NewCallValueSource(refID, 0, 0, 0, callPoint, shape)
	site := factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextReturnSource, Point: callPoint, HasPoint: true, ExprRef: refID, HasExpr: true, Final: true, Adjusted: true, ResultTargets: []factflow.CallResultTarget{factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 0, 0, 0, pathdom.Path{})}})
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	callOp, _ := operationplan.NewSignatureCallOperation(sig)
	template, _ := effectlowering.StaticSignatureAllocationTemplate(sig)
	allocationOp, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{Owner: 41, Template: template.Root, Ordinal: uint32(callPoint)}, template)
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{callPoint: site}, Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})}}).
		WithSignatureCalls(map[cfg.Point]operationplan.SignatureCallOperation{callPoint: callOp}).
		WithSignatureAllocations(map[cfg.Point]operationplan.SignatureAllocationOperation{callPoint: allocationOp})
	return transformer.NewPlanCompiler().Compile(reg, graph, plan, transformer.Shape{})
}
