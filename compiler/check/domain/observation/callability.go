package observation

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/effect"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// HasCallableTypeEffect reports whether expr resolves to a type-constructor
// callable. This is a solved program-model projection; diagnostics consume the
// result to avoid reporting ordinary function-call errors for type values.
func (p Projector) HasCallableTypeEffect(expr ast.Expr, point cfg.Point) bool {
	if expr == nil {
		return false
	}
	if functionHasCallableTypeEffect(p.TypeOf(expr, point)) {
		return true
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || len(p.cfg.Scopes) == 0 {
		return false
	}
	sc := p.cfg.Scopes[point]
	if sc == nil {
		return false
	}
	meta := sc.MetaForName(ident.Value)
	if meta == nil {
		return false
	}
	fn := typ.Func().
		Param("value", typ.Any).
		Returns(meta.Of).
		Effects(effect.WithCallableType()).
		Build()
	return functionHasCallableTypeEffect(fn)
}

// HasTypeValueMethodEffect reports whether receiver:method is a type-value
// operation rather than an ordinary runtime method call.
func (p Projector) HasTypeValueMethodEffect(receiver ast.Expr, point cfg.Point, method string) bool {
	if receiver == nil || method == "" {
		return false
	}
	receiverType := p.TypeOf(receiver, point)
	if receiverType == nil {
		return false
	}
	var methodType typ.Type
	if p.cfg.TypeOps != nil {
		methodType, _ = p.cfg.TypeOps.Method(p.cfg.Ctx, receiverType, method)
	} else {
		methodType, _ = querycore.Method(receiverType, method)
	}
	fn := unwrap.Function(methodType)
	if fn == nil {
		return false
	}
	row, ok := fn.Effects.(effect.Row)
	return ok && row.HasTypeValueMethod()
}

func functionHasCallableTypeEffect(t typ.Type) bool {
	fn := unwrap.Function(t)
	if fn == nil {
		return false
	}
	row, ok := fn.Effects.(effect.Row)
	return ok && row.HasCallableType()
}
