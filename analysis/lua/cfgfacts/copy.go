package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func copyGenericForFact(fact GenericForFact) GenericForFact {
	fact.Names = append([]string(nil), fact.Names...)
	fact.Exprs = append([]ast.Expr(nil), fact.Exprs...)
	fact.Sources = append([]sourceprovenance.ASTSource(nil), fact.Sources...)
	fact.Symbols = append([]symbol.ID(nil), fact.Symbols...)
	return fact
}
