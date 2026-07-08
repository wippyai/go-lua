package cfgfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestGenericForFactCopiesSlices(t *testing.T) {
	var meta Metadata
	names := []string{"k", "v"}
	iterExpr := &ast.IdentExpr{Value: "iter"}
	stateExpr := &ast.IdentExpr{Value: "state"}
	exprs := []ast.Expr{iterExpr, stateExpr}
	sources := []sourceprovenance.ASTSource{{Kind: sourceprovenance.SourceExpression}, {Kind: sourceprovenance.SourceNil}}
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
	sources[0].Kind = sourceprovenance.SourceCall
	symbols[0] = 9

	fact, ok := meta.GenericFor(7)
	if !ok {
		t.Fatal("missing generic for fact")
	}
	if fact.Names[0] != "k" || fact.Exprs[0] != iterExpr || fact.Sources[0].Kind != sourceprovenance.SourceExpression || fact.Symbols[0] != 1 {
		t.Fatalf("generic for fact was mutated through input slices: %+v", fact)
	}

	fact.Names[0] = "mutated"
	fact.Exprs[0] = &ast.IdentExpr{Value: "mutated"}
	fact.Sources[0].Kind = sourceprovenance.SourceCall
	fact.Symbols[0] = 42
	again, _ := meta.GenericFor(7)
	if again.Names[0] != "k" || again.Sources[0].Kind != sourceprovenance.SourceExpression || again.Symbols[0] != 1 {
		t.Fatalf("generic for fact was mutated through getter slice: %+v", again)
	}
}
