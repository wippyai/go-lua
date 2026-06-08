package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
)

// staticAccess is the call package's normalized view of a source-level access
// path. Target and type resolution share this so direct static fallback, live
// function refs, and closure refs do not re-lower AST paths independently.
type staticAccess struct {
	Bindings *bind.BindingTable
}

func (a staticAccess) exprPath(expr ast.Expr) (constraint.Path, bool) {
	if a.Bindings == nil || expr == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(expr, nil, a.Bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

func (a staticAccess) methodPath(call *ast.FuncCallExpr) (constraint.Path, bool) {
	if call == nil || call.Method == "" {
		return constraint.Path{}, false
	}
	path, ok := a.exprPath(call.Receiver)
	if !ok {
		return constraint.Path{}, false
	}
	path.Segments = append(append([]constraint.Segment(nil), path.Segments...), constraint.Segment{
		Kind: constraint.SegmentField,
		Name: call.Method,
	})
	return path, true
}

func (a staticAccess) directField(expr ast.Expr) (cfg.SymbolID, fieldkey.Key, bool) {
	path, ok := a.exprPath(expr)
	if !ok || len(path.Segments) != 1 {
		return 0, fieldkey.Key{}, false
	}
	key, ok := fieldkey.FromSegment(path.Segments[0])
	return path.Symbol, key, ok
}
