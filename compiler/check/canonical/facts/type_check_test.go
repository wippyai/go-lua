package facts

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/types/typ"
)

func TestGuardTypeCheckBindsExtractsValueAndArgumentNarrows(t *testing.T) {
	root := parseFactsTestChunk(t, `
local function f(x)
	local v, err = User:is(x)
	if err == nil then
		return v
	end
end
`)
	graphs := collectFactsTestGraphs(root, "User")
	g := factsTestGraphWithParam(t, graphs, "x")
	userType := typ.NewRecord().Field("id", typ.String).Build()

	binds := guard.TypeCheckBinds(g, func(name string) typ.Type {
		if name == "User" {
			return userType
		}
		return nil
	})
	if len(binds) != 1 {
		t.Fatalf("TypeCheckBinds len = %d, want 1: %+v", len(binds), binds)
	}
	if got := g.Bindings().Name(binds[0].ErrSym); got != "err" {
		t.Fatalf("ErrSym name = %q, want err", got)
	}
	if !typ.TypeEquals(binds[0].Type, userType) {
		t.Fatalf("bind type = %v, want %v", binds[0].Type, userType)
	}
	names := make([]string, 0, len(binds[0].NarrowSyms))
	for _, sym := range binds[0].NarrowSyms {
		names = append(names, g.Bindings().Name(sym))
	}
	slices.Sort(names)
	if len(names) != 2 || names[0] != "v" || names[1] != "x" {
		t.Fatalf("NarrowSyms names = %#v, want v/x", names)
	}

	if unresolved := guard.TypeCheckBinds(g, func(string) typ.Type { return nil }); len(unresolved) != 0 {
		t.Fatalf("unresolved type produced binds: %+v", unresolved)
	}
}

func TestBuildPreTransferStoresTypeCheckBindsByRef(t *testing.T) {
	root := parseFactsTestChunk(t, `
local function f(x)
	local v, err = User:is(x)
	if err == nil then
		return v
	end
end
`)
	graphs := collectFactsTestGraphs(root, "User")
	g := factsTestGraphWithParam(t, graphs, "x")
	fnRef := ref.FuncRef{GraphID: g.ID()}
	userType := typ.NewRecord().Field("id", typ.String).Build()

	m := BuildPreTransfer(Program{
		Refs: factsTestRefs(graphs),
		Graph: func(r ref.FuncRef) *cfg.Graph {
			return graphByRef(graphs, r)
		},
		TypeByName: func(name string) typ.Type {
			if name == "User" {
				return userType
			}
			return nil
		},
	})
	binds := m.TypeChecks(fnRef)
	if len(binds) != 1 {
		t.Fatalf("TypeChecks(ref) len = %d, want 1: %+v", len(binds), binds)
	}
	binds[0].NarrowSyms[0] = 0
	if again := m.TypeChecks(fnRef); len(again) != 1 || again[0].NarrowSyms[0] == 0 {
		t.Fatalf("TypeChecks exposed mutable backing store: %+v", again)
	}
}

func factsTestGraphWithParam(t *testing.T, graphs map[*ast.FunctionExpr]*cfg.Graph, name string) *cfg.Graph {
	t.Helper()
	for fn, g := range graphs {
		if fn == nil || fn.ParList == nil || g == nil {
			continue
		}
		for _, n := range fn.ParList.Names {
			if n == name {
				return g
			}
		}
	}
	t.Fatalf("no graph with param %q", name)
	return nil
}
