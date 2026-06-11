package cfgfacts

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLoopKind(t *testing.T) {
	tests := []struct {
		kind LoopKind
		name string
	}{
		{LoopKindUnknown, "LoopKindUnknown"},
		{LoopKindConditional, "LoopKindConditional"},
		{LoopKindNumericFor, "LoopKindNumericFor"},
		{LoopKindGenericFor, "LoopKindGenericFor"},
	}

	for i, tt := range tests {
		if int(tt.kind) != i {
			t.Errorf("%s: expected value %d, got %d", tt.name, i, tt.kind)
		}
	}
}

func TestLoopFactCopiesSlices(t *testing.T) {
	var meta Metadata
	vars := []symbol.ID{1}
	locals := []symbol.ID{2}
	modified := []symbol.ID{3}

	meta.SetLoop(3, LoopFact{
		Vars:                 vars,
		Locals:               locals,
		DirectModifiedOuters: modified,
		Preheader:            cfg.Point(1),
		HasPreheader:         true,
	})
	vars[0] = 10
	locals[0] = 20
	modified[0] = 30

	fact, ok := meta.Loop(3)
	if !ok {
		t.Fatal("missing loop fact")
	}
	if fact.Vars[0] != 1 || fact.Locals[0] != 2 || fact.DirectModifiedOuters[0] != 3 {
		t.Fatalf("loop fact was mutated through input slices: %+v", fact)
	}

	fact.Vars[0] = 30
	fact.DirectModifiedOuters[0] = 40
	again, _ := meta.Loop(3)
	if again.Vars[0] != 1 || again.DirectModifiedOuters[0] != 3 {
		t.Fatalf("loop fact was mutated through getter slice: %+v", again)
	}
}

func TestGenericForFactCopiesSlices(t *testing.T) {
	var meta Metadata
	names := []string{"k", "v"}
	iterExpr := &ast.IdentExpr{Value: "iter"}
	stateExpr := &ast.IdentExpr{Value: "state"}
	exprs := []ast.Expr{iterExpr, stateExpr}
	sources := []sourceprovenance.ASTSource{{Kind: factflow.ValueSourceExpression}, {Kind: factflow.ValueSourceNil}}
	symbols := []symbol.ID{1, 2}

	meta.SetGenericFor(7, GenericForFact{
		Stmt:          &ast.GenericForStmt{Names: names, Exprs: exprs},
		Role:          GenericForRoleVariable,
		Names:         names,
		Exprs:         exprs,
		Sources:       sources,
		Symbols:       symbols,
		HasSymbols:    true,
		VariableIndex: 1,
	})
	names[0] = "mutated"
	exprs[0] = &ast.IdentExpr{Value: "mutated"}
	sources[0].Kind = factflow.ValueSourceCall
	symbols[0] = 9

	fact, ok := meta.GenericFor(7)
	if !ok {
		t.Fatal("missing generic for fact")
	}
	if fact.Names[0] != "k" || fact.Exprs[0] != iterExpr || fact.Sources[0].Kind != factflow.ValueSourceExpression || fact.Symbols[0] != 1 {
		t.Fatalf("generic for fact was mutated through input slices: %+v", fact)
	}

	fact.Names[0] = "mutated"
	fact.Exprs[0] = &ast.IdentExpr{Value: "mutated"}
	fact.Sources[0].Kind = factflow.ValueSourceCall
	fact.Symbols[0] = 42
	again, _ := meta.GenericFor(7)
	if again.Names[0] != "k" || again.Sources[0].Kind != factflow.ValueSourceExpression || again.Symbols[0] != 1 {
		t.Fatalf("generic for fact was mutated through getter slice: %+v", again)
	}
}
