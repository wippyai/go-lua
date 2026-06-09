package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProgramEntrySymbolValuesSeedsPredeclaredGlobals(t *testing.T) {
	stmts, err := parse.ParseString(`return print`, "global-entry.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{Stmts: stmts}
	in := input.BuildFromFunction(fn, nil, nil, "print", "unused")
	if in.Graph == nil {
		t.Fatal("input builder produced no graph")
	}
	sym, ok := in.Graph.GlobalSymbol("print")
	if !ok || sym == 0 {
		t.Fatal("print global symbol not found")
	}
	unusedSym, ok := in.Graph.GlobalSymbol("unused")
	if !ok || unusedSym == 0 {
		t.Fatal("unused global symbol not found")
	}

	ref := summary.FuncRef{GraphID: 1}
	prog := &program{
		funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
			{Ref: ref, Graph: in.Graph},
		}),
		observationContexts: map[summary.FuncRef]functionObservationContext{
			ref: {
				declared: map[cfg.SymbolID]typ.Type{
					sym:       typ.String,
					unusedSym: typ.Number,
				},
			},
		},
	}

	values := prog.EntrySymbolValues(ref)
	got := values[sym].ProjectValue()
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("entry symbol value for print = %v, want string", got)
	}
	if _, seeded := values[unusedSym]; seeded {
		t.Fatal("unused predeclared global should not be seeded into entry state")
	}
}
