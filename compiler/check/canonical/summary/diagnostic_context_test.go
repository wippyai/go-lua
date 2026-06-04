package summary_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDiagnosticContextFrontierCachesSolvedObserverStates(t *testing.T) {
	root := summary.FuncRef{GraphID: 1}
	callee := summary.FuncRef{GraphID: 2}
	rootKey := summary.NewKey(root, flow.CaptureCellsDomain.Bottom())
	calleeKey := summary.NewKey(callee, flow.CaptureCellsDomain.Bottom())
	solves := make(map[summary.Key]int)

	result := summary.DiagnosticContextFrontier{
		Root: root,
		Refs: []summary.FuncRef{root, callee},
		Solve: func(key summary.Key) state.FunctionState {
			solves[key]++
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref summary.FuncRef, _ state.FunctionState) []summary.Key {
			if ref != root {
				return nil
			}
			return []summary.Key{calleeKey, calleeKey}
		},
	}.Build()

	if solves[rootKey] != 1 {
		t.Fatalf("root solves = %d, want 1", solves[rootKey])
	}
	if solves[calleeKey] != 1 {
		t.Fatalf("callee solves = %d, want 1", solves[calleeKey])
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
	root := summary.FuncRef{GraphID: 1}
	called := summary.FuncRef{GraphID: 2}
	uncalledClosure := summary.FuncRef{GraphID: 3}
	uncalledDefault := summary.FuncRef{GraphID: 4}
	calledKey := summary.NewKey(called, flow.CaptureCellsDomain.Bottom())
	calledClosureKey := summary.NewKeyWithEntryContext(
		called,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 10, Value: product.FromType(typ.String)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
	)
	closureKey := summary.NewKeyWithEntryContext(
		uncalledClosure,
		flow.CaptureCellsOf([]flow.CaptureCell{{Symbol: 11, Value: product.FromType(typ.Number)}}),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		nil,
	)
	defaultKey := summary.NewKey(uncalledDefault, flow.CaptureCellsDomain.Bottom())

	result := summary.DiagnosticContextFrontier{
		Root: root,
		Refs: []summary.FuncRef{root, called, uncalledClosure, uncalledDefault},
		DefaultKey: func(ref summary.FuncRef) summary.Key {
			if ref == uncalledDefault {
				return defaultKey
			}
			return summary.NewKey(ref, flow.CaptureCellsDomain.Bottom())
		},
		Solve: func(summary.Key) state.FunctionState {
			return state.FunctionStateDomain.Bottom()
		},
		ProjectCalls: func(ref summary.FuncRef, _ state.FunctionState) []summary.Key {
			if ref == root {
				return []summary.Key{calledKey}
			}
			return nil
		},
		ProjectClosures: func(ref summary.FuncRef, _ state.FunctionState) []summary.Key {
			if ref == root {
				return []summary.Key{calledClosureKey, closureKey}
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
