package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeIsIntercept handles runtime type checking via the Type:is(x) pattern.
//
// In the type system, type names (like Point, Number) are first-class Meta
// types with an :is method for runtime type checking. This intercept handles
// calls like:
//
//	local Point = ...
//	if Point:is(value) then ... end
//
// Detection is effect-based: the receiver's :is method must have the
// TypeValueMethod effect, indicating it's a type checking method rather
// than a regular method.
//
// Always returns boolean since :is performs a type check.
type TypeIsIntercept struct{}

func (t *TypeIsIntercept) InterceptMethodCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if ex.Method != "is" {
		return Result{}
	}

	if len(ex.Args) == 0 {
		return Result{}
	}

	ident, ok := ex.Receiver.(*ast.IdentExpr)
	if !ok {
		return Result{}
	}

	// Effect-based dispatch: resolve receiver as Meta type, check :is method's effect
	if ctx.Scope != nil {
		meta := ctx.Scope.MetaForName(ident.Value)
		if meta != nil {
			// Meta types have :is with TypeValueMethod effect
			methodType, found := core.Method(meta, "is")
			if found {
				fn := unwrap.Function(methodType)
				if fn != nil {
					if row, ok := fn.Effects.(effect.Row); ok && row.HasTypeValueMethod() {
						return Result{
							Types: []typ.Type{typ.Boolean},
							Skip:  true,
						}
					}
				}
			}
		}
	}

	return Result{}
}
