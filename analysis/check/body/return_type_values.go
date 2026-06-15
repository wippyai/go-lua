package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ReturnTypeValues materializes declared return-type evidence for summary
// projection at call boundaries.
func (r *Result) ReturnTypeValues() []product.Value {
	if r == nil || r.registry == nil || r.bindings == nil || r.Function() == nil {
		return nil
	}
	returnTypes := declaredReturnTypeExprs(r.Function().ReturnTypes)
	if len(returnTypes) == 0 {
		return nil
	}
	resolver := typeresolve.NewWithExternal(r.bindings, r.moduleTypes)
	out := make([]product.Value, 0, len(returnTypes))
	for _, expr := range returnTypes {
		t, ok := resolver.Type(expr)
		if !ok {
			out = append(out, product.Top())
			continue
		}
		out = append(out, typevalue.WithWitness(r.registry, typevalue.FromType(r.registry, t), t))
	}
	return out
}

func declaredReturnTypeExprs(types []ast.TypeExpr) []ast.TypeExpr {
	if len(types) == 1 {
		if tuple, ok := types[0].(*ast.TupleTypeExpr); ok {
			return append([]ast.TypeExpr(nil), tuple.Elements...)
		}
	}
	return append([]ast.TypeExpr(nil), types...)
}
