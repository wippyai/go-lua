package summary

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDiagnosticContextFrontierCachesSolvedObserverStates(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	rootKey := NewDefaultKey(root, nil)
	calleeKey := NewDefaultKey(callee, nil)
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, callee},
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref != root {
				return nil
			}
			return []Key{calleeKey, calleeKey}
		},
	}.Build()

	if solves[rootKey] == 0 {
		t.Fatal("root was not solved")
	}
	if solves[calleeKey] == 0 {
		t.Fatal("callee was not solved")
	}
	if _, ok := result.State(rootKey); !ok {
		t.Fatal("root state was not cached")
	}
	if _, ok := result.State(calleeKey); !ok {
		t.Fatal("callee state was not cached")
	}
	if got := len(result.Contexts[callee]); got != 1 {
		t.Fatalf("callee contexts = %d, want 1", got)
	}
}

func TestDiagnosticContextFrontierUsesFallbackOnlyForUncalledFunctions(t *testing.T) {
	root := FuncRef{GraphID: 1}
	called := FuncRef{GraphID: 2}
	uncalledClosure := FuncRef{GraphID: 3}
	uncalledDefault := FuncRef{GraphID: 4}
	calledKey := NewDefaultKey(called, nil)
	calledClosureKey := NewKeyWithEntryContextFacts(
		called,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	closureKey := NewKeyWithEntryContextFacts(
		uncalledClosure,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 11, Value: product.FromType(typ.Number)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	defaultKey := NewDefaultKey(uncalledDefault, nil)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, called, uncalledClosure, uncalledDefault},
		DefaultKey: func(ref FuncRef) Key {
			if ref == uncalledDefault {
				return defaultKey
			}
			return NewDefaultKey(ref, nil)
		},
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{calledKey}
			}
			return nil
		},
		ProjectClosures: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{calledClosureKey, closureKey}
			}
			return nil
		},
	}.Build()

	if got := result.Contexts[called]; len(got) != 1 || got[0] != calledKey {
		t.Fatalf("called contexts = %+v, want only primary call key", got)
	}
	if got := result.Contexts[uncalledClosure]; len(got) != 1 || got[0] != closureKey {
		t.Fatalf("uncalled closure contexts = %+v, want closure fallback", got)
	}
	if got := result.Contexts[uncalledDefault]; len(got) != 1 || got[0] != defaultKey {
		t.Fatalf("uncalled default contexts = %+v, want default fallback", got)
	}
}

func TestDiagnosticContextFrontierPromotesFallbackDiscoveredCallContext(t *testing.T) {
	root := FuncRef{GraphID: 1}
	caller := FuncRef{GraphID: 2}
	callee := FuncRef{GraphID: 3}
	callerFallback := NewKeyWithEntryContextFacts(
		caller,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	calleeFallback := NewKeyWithEntryContextFacts(
		callee,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 11, Value: product.FromType(typ.String)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	calleeCall := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.Number)})

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, caller, callee},
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == caller {
				return []Key{calleeCall}
			}
			return nil
		},
		ProjectClosures: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{callerFallback, calleeFallback}
			}
			return nil
		},
	}.Build()

	if got := result.Contexts[callee]; len(got) != 1 || got[0] != calleeCall {
		t.Fatalf("callee contexts = %+v, want promoted call context only", got)
	}
	if got := result.Contexts[caller]; len(got) != 1 || got[0] != callerFallback {
		t.Fatalf("caller contexts = %+v, want fallback caller", got)
	}
}

func TestDiagnosticContextFrontierReducesDominatedEntryFactContexts(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	base := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.String)})
	facts := flow.BoundaryFactsOf(nil, []flow.BoundaryKeyArrayFact{{
		Array: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0},
		Table: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{
			Kind: constraint.SegmentField,
			Name: "nodes",
		}}},
	}}, nil, nil, nil, nil)
	factful := NewKeyWithEntryContextFacts(
		callee,
		flow.CaptureCellsDomain.Bottom(),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		EntryValues{0: product.FromType(typ.String)},
		facts,
	)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, callee},
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{base, factful}
			}
			return nil
		},
	}.Build()

	if got := result.Contexts[callee]; len(got) != 1 || got[0] != factful {
		t.Fatalf("callee contexts = %+v, want only factful context", got)
	}
}

func TestDiagnosticContextFrontierDropsStaleContextsAfterRefresh(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	rootKey := NewDefaultKey(root, nil)
	stale := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.NewRecord().Build())})
	current := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.NewRecord().Field("nodes", typ.NewMap(typ.String, typ.Number)).Build())})
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, callee},
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			if key != rootKey || solves[key] == 1 {
				return state.FunctionStateDomain.Bottom()
			}
			fs := state.FunctionStateDomain.Bottom()
			fs.InPoints = map[cfg.Point]flow.PointState{
				1: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(10): product.FromType(typ.String),
					},
				},
			}
			return fs
		},
		ProjectCalls: func(ref FuncRef, fs state.FunctionState) []Key {
			if ref != root {
				return nil
			}
			if len(fs.InPoints) == 0 {
				return []Key{stale}
			}
			return []Key{current}
		},
	}.Build()

	if solves[rootKey] < 2 {
		t.Fatalf("root solves = %d, want refresh", solves[rootKey])
	}
	if got := result.Contexts[callee]; len(got) != 1 || got[0] != current {
		t.Fatalf("callee contexts = %+v, want only current refreshed context", got)
	}
}

func TestDiagnosticContextFrontierIgnoresUnobservableRefreshChurn(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root},
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			fs := state.FunctionStateDomain.Bottom()
			fs.InPoints = map[cfg.Point]flow.PointState{
				1: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(10): product.FromType(typ.LiteralInt(int64(solves[key]))),
					},
				},
			}
			return fs
		},
	}.Build()

	if solves[rootKey] != 2 {
		t.Fatalf("root solves = %d, want initial solve plus one unobservable refresh", solves[rootKey])
	}
	if got := result.Contexts[root]; len(got) != 1 || got[0] != rootKey {
		t.Fatalf("root contexts = %+v, want only default root context", got)
	}
}

func TestDiagnosticContextFrontierRefreshesDerivedInPointContexts(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	rootKey := NewDefaultKey(root, nil)
	calleeKey := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.String)})
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root},
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			if key != rootKey || solves[key] == 1 {
				return state.FunctionStateDomain.Bottom()
			}
			fs := state.FunctionStateDomain.Bottom()
			fs.InPoints = map[cfg.Point]flow.PointState{
				1: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(10): product.FromType(typ.String),
					},
				},
			}
			return fs
		},
		ProjectCalls: func(ref FuncRef, fs state.FunctionState) []Key {
			if ref != root || len(fs.InPoints) == 0 {
				return nil
			}
			return []Key{calleeKey}
		},
	}.Build()

	if solves[rootKey] < 2 {
		t.Fatalf("root solves = %d, want refresh after initial observer state", solves[rootKey])
	}
	if got := result.Contexts[callee]; len(got) != 1 || got[0] != calleeKey {
		t.Fatalf("callee contexts = %+v, want refreshed call context", got)
	}
}

func TestDiagnosticContextFrontierRefreshesCallersAfterExactSummaryOverlay(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	rootKey := NewDefaultKey(root, nil)
	calleeKey := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.String)})
	overlay := make(map[Key]Summary)
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root, callee},
		SummaryOverlay: overlay,
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			fs := state.FunctionStateDomain.Bottom()
			if key == rootKey {
				sum, ok := overlay[calleeKey]
				if !ok || len(sum.Returns) == 0 || !typ.TypeEquals(sum.Returns[0].ProjectValue(), typ.String) {
					return fs
				}
				fs.InPoints = map[cfg.Point]flow.PointState{
					1: {
						Env: map[flow.ValueKey]product.AbstractValue{
							flow.SymbolValueKey(10): product.FromType(typ.Boolean),
						},
					},
				}
			}
			return fs
		},
		ProjectSummary: func(key Key, _ state.FunctionState) Summary {
			if key == calleeKey {
				return Summary{
					Returns: []product.AbstractValue{product.FromType(typ.String)},
				}
			}
			return SummaryDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{calleeKey}
			}
			return nil
		},
	}.Build()

	rootState, ok := result.State(rootKey)
	if !ok {
		t.Fatal("root state was not cached")
	}
	if len(rootState.InPoints) == 0 {
		t.Fatalf("root was not refreshed after callee overlay summary; solves=%d", solves[rootKey])
	}
	if solves[rootKey] < 2 {
		t.Fatalf("root solves = %d, want at least two solves", solves[rootKey])
	}
	if _, ok := result.Summaries[calleeKey]; !ok {
		t.Fatal("callee overlay summary was not projected")
	}
}

func TestDiagnosticContextFrontierRefreshesOnlyExactOverlayDependents(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	unrelated := FuncRef{GraphID: 3}
	rootKey := NewDefaultKey(root, nil)
	calleeKey := NewDefaultKey(callee, nil)
	unrelatedKey := NewDefaultKey(unrelated, nil)
	overlay := make(map[Key]Summary)
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root, callee, unrelated},
		SummaryOverlay: overlay,
		SolveWithDependencies: func(key Key) (state.FunctionState, []Key) {
			solves[key]++
			fs := state.FunctionStateDomain.Bottom()
			if key == rootKey {
				sum, ok := overlay[calleeKey]
				if ok && len(sum.Returns) != 0 && typ.TypeEquals(sum.Returns[0].ProjectValue(), typ.String) {
					fs.InPoints = map[cfg.Point]flow.PointState{
						1: {
							Env: map[flow.ValueKey]product.AbstractValue{
								flow.SymbolValueKey(10): product.FromType(typ.Boolean),
							},
						},
					}
				}
				return fs, []Key{calleeKey}
			}
			return fs, nil
		},
		ProjectSummary: func(key Key, _ state.FunctionState) Summary {
			if key == calleeKey && solves[key] >= 2 {
				return Summary{
					Returns: []product.AbstractValue{product.FromType(typ.String)},
				}
			}
			return SummaryDomain.Bottom()
		},
		ProjectCalls: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{calleeKey, unrelatedKey}
			}
			return nil
		},
	}.Build()

	rootState, ok := result.State(rootKey)
	if !ok {
		t.Fatal("root state was not cached")
	}
	if len(rootState.InPoints) == 0 {
		t.Fatalf("root was not refreshed after dependent callee overlay; solves=%d", solves[rootKey])
	}
	if solves[rootKey] < 3 {
		t.Fatalf("root solves = %d, want dependency-triggered refresh", solves[rootKey])
	}
	if solves[unrelatedKey] != 2 {
		t.Fatalf("unrelated solves = %d, want only initial solve plus normal refresh", solves[unrelatedKey])
	}
}

func TestDiagnosticContextFrontierWidensExactSummaryOverlay(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	overlay := map[Key]Summary{
		rootKey: {
			Returns: []product.AbstractValue{product.FromType(typ.LiteralString("a"))},
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				Returns: []product.AbstractValue{product.FromType(typ.LiteralString("b"))},
			}
		},
	}.Build()

	got := result.Summaries[rootKey].Returns
	if len(got) != 1 {
		t.Fatalf("overlay returns = %#v, want one widened return", got)
	}
	if typ.TypeEquals(got[0].ProjectValue(), typ.LiteralString("b")) {
		t.Fatalf("overlay summary was replaced instead of widened: %v", got[0].ProjectValue())
	}
	want := typ.NewUnion(typ.LiteralString("a"), typ.LiteralString("b"))
	if !typ.TypeEquals(got[0].ProjectValue(), want) {
		t.Fatalf("overlay return = %v, want widened %v", got[0].ProjectValue(), want)
	}
}

func TestDiagnosticContextFrontierLetsExactReturnOverlayRefineWidenedValue(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	overlay := map[Key]Summary{
		rootKey: {
			Returns: []product.AbstractValue{product.FromType(typ.Number)},
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				Returns: []product.AbstractValue{product.FromType(typ.Integer)},
			}
		},
	}.Build()

	got := result.Summaries[rootKey].Returns
	if len(got) != 1 || !typ.TypeEquals(got[0].ProjectValue(), typ.Integer) {
		t.Fatalf("overlay return = %#v, want exact integer refinement", got)
	}
}

func TestDiagnosticContextFrontierDoesNotEraseExactReturnOverlayWithBroadRefresh(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	broad := product.FromType(typ.NewRecord().Build())
	refined := product.FromType(typ.NewRecord().Field("render", typ.Func().Returns(typ.String).Build()).Build())
	overlay := map[Key]Summary{
		rootKey: {
			Returns: []product.AbstractValue{refined},
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				Returns: []product.AbstractValue{broad},
			}
		},
	}.Build()

	got := result.Summaries[rootKey].Returns
	if len(got) != 1 {
		t.Fatalf("overlay returns = %#v, want one slot", got)
	}
	if gotValue := got[0].ProjectValue(); !typ.TypeEquals(gotValue, refined.ProjectValue()) {
		t.Fatalf("overlay return erased exact member: %v, want %v", gotValue, refined.ProjectValue())
	}
}

func TestDiagnosticContextFrontierDoesNotEraseExactReturnOverlayWithRecursiveRefresh(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	refinedType := typ.NewRecord().Field("node_order", typ.NewArray(typ.String)).Build()
	refined := product.FromType(refinedType)
	recursive := product.FromType(typ.NewRecursive("Inferred", func(typ.Type) typ.Type {
		return typ.NewRecord().Field("node_order", typ.NewArray(typ.Never)).Build()
	}))
	direct := mergeExactOverlaySummary(
		Summary{Returns: []product.AbstractValue{refined}},
		Summary{Returns: []product.AbstractValue{recursive}},
	)
	if len(direct.Returns) != 1 || !typ.TypeEquals(direct.Returns[0].ProjectValue(), refinedType) {
		t.Fatalf("direct exact overlay merge = %#v, want %v", direct.Returns, refinedType)
	}
	overlay := map[Key]Summary{
		rootKey: {
			Returns: []product.AbstractValue{refined},
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				Returns: []product.AbstractValue{recursive},
			}
		},
	}.Build()

	got := result.Summaries[rootKey].Returns
	if len(got) != 1 {
		t.Fatalf("overlay returns = %#v, want one slot", got)
	}
	if gotValue := got[0].ProjectValue(); !typ.TypeEquals(gotValue, refinedType) {
		t.Fatalf("overlay return erased exact recursive member: %v, want %v", gotValue, refinedType)
	}
}

func TestDiagnosticContextFrontierDoesNotEraseExactReturnOverlayWithEmptyContainerRefresh(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	refinedType := typ.NewRecord().Field("node_order", typ.NewArray(typ.String)).Build()
	refined := product.FromType(refinedType)
	empty := product.FromType(typ.NewRecord().Field("node_order", typ.NewArray(typ.Never)).Build())
	overlay := map[Key]Summary{
		rootKey: {
			Returns: []product.AbstractValue{refined},
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				Returns: []product.AbstractValue{empty},
			}
		},
	}.Build()

	got := result.Summaries[rootKey].Returns
	if len(got) != 1 {
		t.Fatalf("overlay returns = %#v, want one slot", got)
	}
	if gotValue := got[0].ProjectValue(); !typ.TypeEquals(gotValue, refinedType) {
		t.Fatalf("overlay return erased exact empty-container member: %v, want %v", gotValue, refinedType)
	}
}

func TestDiagnosticContextFrontierLetsExactProofOverlayRefineFromTop(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	overlay := map[Key]Summary{
		rootKey: {
			Relations:     flow.ReturnRelationsDomain.Top(),
			BoundaryFacts: flow.BoundaryFactsDomain.Top(),
		},
	}
	facts := flow.BoundaryFactsOf(nil, nil, nil, nil, []flow.BoundaryLengthLowerBound{{
		Target: flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0},
		Lower:  1,
	}}, nil)
	relations := flow.ReturnRelationsOfLengthParams([]flow.ReturnLengthParamRelation{{
		ReturnIndex: 0,
		ParamIndex:  0,
	}})

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				BoundaryFacts: facts,
				Relations:     relations,
			}
		},
	}.Build()

	got := result.Summaries[rootKey]
	if !got.BoundaryFacts.HasProof() || !flow.BoundaryFactsDomain.Equal(got.BoundaryFacts, facts) {
		t.Fatalf("overlay boundary facts = %#v, want latest exact proof %#v", got.BoundaryFacts, facts)
	}
	if !got.Relations.HasProof() || !flow.ReturnRelationsDomain.Equal(got.Relations, relations) {
		t.Fatalf("overlay return relations = %#v, want latest exact proof %#v", got.Relations, relations)
	}
}

func TestDiagnosticContextFrontierLetsExactEffectOverlayRefineFromIdentity(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	sym := cfg.SymbolID(42)
	must := flow.CaptureMustWrite(sym, product.FromType(typ.String))
	overlay := map[Key]Summary{
		rootKey: {
			CellEffects:     flow.CaptureEffectsIdentity(),
			ReceiverEffects: flow.ReceiverEffectsIdentity(),
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				CellEffects: must,
			}
		},
	}.Build()

	got := result.Summaries[rootKey].CellEffects
	if !flow.CaptureEffectsDomain.Equal(got, must) {
		t.Fatalf("overlay cell effects = %s, want latest exact effect %s", got.Format(), must.Format())
	}
	entries := got.Entries()
	if len(entries) != 1 || !entries[0].MustWrite {
		t.Fatalf("overlay cell effect kept historical identity path: %s", got.Format())
	}
}

func TestDiagnosticContextFrontierDoesNotEraseExactEffectWithIdentityRefresh(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)
	sym := cfg.SymbolID(42)
	must := flow.CaptureMustWrite(sym, product.FromType(typ.String))
	overlay := map[Key]Summary{
		rootKey: {
			CellEffects: must,
		},
	}

	result := DiagnosticContextFrontier{
		Root:           root,
		Refs:           []FuncRef{root},
		SummaryOverlay: overlay,
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectSummary: func(Key, state.FunctionState) Summary {
			return Summary{
				CellEffects: flow.CaptureEffectsIdentity(),
			}
		},
	}.Build()

	got := result.Summaries[rootKey].CellEffects
	entries := got.Entries()
	if len(entries) != 1 || entries[0].Symbol != sym || entries[0].Value.IsZero() {
		t.Fatalf("overlay cell effect erased by identity refresh: %s", got.Format())
	}
}
