package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildPreTransferSeedsReturnedFunctionLiteralExpectedParams(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "direct",
			src: `
local function build()
	return function(state, at)
	end
end
`,
		},
		{
			name: "identifier",
			src: `
local function build()
	local project = function(state, at)
	end
	return project
end
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseFactsTestChunk(t, tc.src)
			graphs := collectFactsTestGraphs(root)
			rootGraph := graphs[root]
			funcRefsBySymbol := factsTestFuncSymbolRefs(graphs)
			buildRef, ok := funcRefsBySymbol[symbolNamed(t, rootGraph, "build")]
			if !ok {
				t.Fatal("missing build function ref")
			}
			callbackRef := refByParamNames(t, graphs, "state", "at")
			stateType := typ.NewRecord().Field("id", typ.String).Build()
			timeType := typ.NewInterface("time.Time", nil)
			expected := typ.Func().
				Param("state", stateType).
				Param("at", timeType).
				Build()

			m := BuildPreTransfer(Program{
				Refs: factsTestRefs(graphs),
				Graph: func(r ref.FuncRef) *cfg.Graph {
					return graphByRef(graphs, r)
				},
				Evidence: func(g *cfg.Graph) api.FlowEvidence {
					return trace.GraphEvidence(g, g.Bindings())
				},
				RefForFuncSymbol: func(sym cfg.SymbolID) (ref.FuncRef, bool) {
					r, ok := funcRefsBySymbol[sym]
					return r, ok
				},
				DeclaredReturnTypes: func(r ref.FuncRef) []typ.Type {
					if r == buildRef {
						return []typ.Type{expected}
					}
					return nil
				},
			})

			seeds := m.FunctionEntrySeeds(callbackRef)
			assertEntrySeed(t, seeds, 0, stateType)
			assertEntrySeed(t, seeds, 1, timeType)
		})
	}
}

func refByParamNames(t *testing.T, graphs map[*ast.FunctionExpr]*cfg.Graph, names ...string) ref.FuncRef {
	t.Helper()
	for fn, g := range graphs {
		if fn == nil || fn.ParList == nil || g == nil {
			continue
		}
		if len(fn.ParList.Names) != len(names) {
			continue
		}
		matches := true
		for i, name := range names {
			if fn.ParList.Names[i] != name {
				matches = false
				break
			}
		}
		if matches {
			return ref.FuncRef{GraphID: g.ID()}
		}
	}
	t.Fatalf("missing function ref with params %v", names)
	return ref.FuncRef{}
}

func assertEntrySeed(t *testing.T, seeds []FunctionEntrySeed, slot int, want typ.Type) {
	t.Helper()
	for _, seed := range seeds {
		if seed.Slot == slot && typ.TypeEquals(seed.Type, want) {
			return
		}
	}
	t.Fatalf("missing entry seed slot %d type %v in %+v", slot, want, seeds)
}
