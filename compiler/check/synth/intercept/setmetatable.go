package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/typ"
)

// SetMetatableIntercept models Lua's setmetatable(table, metatable) primitive.
//
// The normal stdlib signature can express the value identity of the first
// argument, but the abstract state also needs the metatable edge on the returned
// table value so method/field queries see the prototype chain.
type SetMetatableIntercept struct{}

func (s *SetMetatableIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if !metatable.IsSetMetatableCall(ex, ctx.Bindings) || ctx.Recurse == nil {
		return Result{}
	}

	tableType := ctx.Recurse(ex.Args[0])
	metaType := ctx.Recurse(ex.Args[1])
	if ctx.StableType != nil {
		metaType = ctx.StableType(ex.Args[1], metaType)
	}
	if ctx.CanonicalMetatable != nil {
		metaType = ctx.CanonicalMetatable(ex.Args[1], metaType)
	}
	if tableType == nil {
		return Result{Skip: true, Types: []typ.Type{typ.Unknown}}
	}

	return Result{Skip: true, Types: []typ.Type{metatable.With(tableType, metaType)}}
}
