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
	calledClosureKey := NewKeyWithReferenceContext(
		called,
		flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	closureKey := NewKeyWithReferenceContext(
		uncalledClosure,
		flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 11, Value: product.FromType(typ.Number)}}),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
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

func TestDiagnosticContextFrontierDropsWeakerClosureContext(t *testing.T) {
	root := FuncRef{GraphID: 1}
	closure := FuncRef{GraphID: 2}
	weak := NewKeyWithReferenceContext(
		closure,
		flow.ReferenceContextOf(
			flow.CaptureCellsDomain.Bottom(),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	strong := NewKeyWithReferenceContext(
		closure,
		flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, closure},
		Solve: func(Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectClosures: func(ref FuncRef, _ state.FunctionState) []Key {
			if ref == root {
				return []Key{weak, strong}
			}
			return nil
		},
	}.Build()

	if got := result.Contexts[closure]; len(got) != 1 || got[0] != strong {
		t.Fatalf("closure contexts = %+v, want only stronger context", got)
	}
}

func TestDiagnosticContextFrontierPromotesFallbackDiscoveredCallContext(t *testing.T) {
	root := FuncRef{GraphID: 1}
	caller := FuncRef{GraphID: 2}
	callee := FuncRef{GraphID: 3}
	callerFallback := NewKeyWithReferenceContext(
		caller,
		flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
		nil,
		flow.BoundaryFactsDomain.Top(),
	)
	calleeFallback := NewKeyWithReferenceContext(
		callee,
		flow.ReferenceContextOf(
			flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 11, Value: product.FromType(typ.String)}}),
			flow.FunctionRefsDomain.Bottom(),
			flow.ClosureRefsDomain.Bottom(),
		),
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
	factful := NewKeyWithReferenceContext(
		callee,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
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

func TestDiagnosticContextFrontierDoesNotRefreshStaleContexts(t *testing.T) {
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

	if solves[rootKey] != 1 {
		t.Fatalf("root solves = %d, want one snapshot observation", solves[rootKey])
	}
	if got := result.Contexts[callee]; len(got) != 1 || got[0] != stale {
		t.Fatalf("callee contexts = %+v, want first snapshot-derived context", got)
	}
	if len(result.Contexts[current.Ref]) != 1 && result.Contexts[callee][0] == current {
		t.Fatal("diagnostic frontier repaired stale context through a refresh pass")
	}
}

func TestDiagnosticContextFrontierDoesNotRefreshUnobservableChurn(t *testing.T) {
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

	if solves[rootKey] != 1 {
		t.Fatalf("root solves = %d, want one snapshot observation", solves[rootKey])
	}
	if got := result.Contexts[root]; len(got) != 1 || got[0] != rootKey {
		t.Fatalf("root contexts = %+v, want only default root context", got)
	}
}

func TestDiagnosticContextFrontierDoesNotDeriveContextsFromRefresh(t *testing.T) {
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

	if solves[rootKey] != 1 {
		t.Fatalf("root solves = %d, want one snapshot observation", solves[rootKey])
	}
	if got := result.Contexts[callee]; len(got) != 0 {
		t.Fatalf("callee contexts = %+v, want none without snapshot-derived call", got)
	}
}

func TestDiagnosticContextFrontierDoesNotUseRefreshAsCallAuthority(t *testing.T) {
	root := FuncRef{GraphID: 1}
	callee := FuncRef{GraphID: 2}
	rootKey := NewDefaultKey(root, nil)
	calleeKey := NewDefaultKey(callee, EntryValues{0: product.FromType(typ.String)})
	solves := make(map[Key]int)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root, callee},
		Solve: func(key Key) state.FunctionState {
			solves[key]++
			fs := state.FunctionStateDomain.Bottom()
			if key == rootKey && solves[key] >= 2 {
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
		ProjectCalls: func(ref FuncRef, fs state.FunctionState) []Key {
			if ref == root && len(fs.InPoints) != 0 {
				return []Key{calleeKey}
			}
			return nil
		},
	}.Build()

	rootState, ok := result.State(rootKey)
	if !ok {
		t.Fatal("root exact observer state was not retained")
	}
	if len(rootState.InPoints) != 0 {
		t.Fatalf("root state was refreshed; solves=%d", solves[rootKey])
	}
	if got := result.Contexts[callee]; len(got) != 1 || got[0] != NewDefaultKey(callee, nil) {
		t.Fatalf("callee contexts = %+v, want default fallback only", got)
	}
}

func TestDiagnosticContextFrontierDoesNotPublishExactSummaries(t *testing.T) {
	root := FuncRef{GraphID: 1}
	rootKey := NewDefaultKey(root, nil)

	result := DiagnosticContextFrontier{
		Root: root,
		Refs: []FuncRef{root},
		Solve: func(Key) state.FunctionState {
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
	}.Build()

	if _, ok := result.State(rootKey); !ok {
		t.Fatal("exact diagnostic state was not retained")
	}
	if len(result.Contexts[root]) != 1 {
		t.Fatalf("root contexts = %+v, want one context", result.Contexts[root])
	}
}
