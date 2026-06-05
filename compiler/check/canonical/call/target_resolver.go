package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// StaticTargetLookup is the immutable topology fallback used after solved
// FunctionRefs/ClosureRefs fail to provide an authoritative call target.
type StaticTargetLookup struct {
	FuncBySymbol  func(cfg.SymbolID) (summary.FuncRef, bool)
	FieldFunc     func(cfg.SymbolID, fieldkey.Key) (summary.FuncRef, bool)
	SelfMethodRef func(self cfg.SymbolID, method fieldkey.Key) (summary.FuncRef, bool)
}

// TargetResolver resolves call targets from normalized product axes and immutable
// topology facts. It performs no summary reads and owns the callable precedence
// rule's data preparation; TargetSet/TargetSelection own the precedence itself.
type TargetResolver struct {
	Bindings *bind.BindingTable
	Static   StaticTargetLookup
}

// Resolve returns the normalized target set for call.
func (r TargetResolver) Resolve(
	call *ast.FuncCallExpr,
	functionRefs flow.FunctionRefs,
	closureRefs flow.ClosureRefs,
) TargetSet {
	if call == nil {
		return TargetSet{}
	}

	var directRefs []summary.FuncRef
	var directAuthoritative bool
	if call.Method != "" {
		directRefs, directAuthoritative = r.directMethodRefsFromState(call, functionRefs)
		if !directAuthoritative {
			if ref, ok := r.ResolveStaticMethod(call); ok {
				directRefs = []summary.FuncRef{ref}
			}
		}
	} else {
		directRefs, directAuthoritative = r.directExprRefsFromState(call.Func, functionRefs)
		if !directAuthoritative {
			if ref, ok := r.ResolveStaticExpr(call.Func); ok {
				directRefs = []summary.FuncRef{ref}
			}
		}
	}

	var (
		closures             []flow.ClosureRef
		closureAuthoritative bool
	)
	if call.Method != "" {
		closures, closureAuthoritative = r.closureMethodRefs(call, closureRefs)
	} else {
		closures, closureAuthoritative = r.closureExprRefs(call.Func, closureRefs)
	}

	return NewTargetSet(directRefs, directAuthoritative, closures, closureAuthoritative)
}

// ResolveStaticCall resolves call through immutable symbol/topology facts only.
func (r TargetResolver) ResolveStaticCall(call *ast.FuncCallExpr) (summary.FuncRef, bool) {
	if call == nil {
		return summary.FuncRef{}, false
	}
	if call.Method != "" {
		return r.ResolveStaticMethod(call)
	}
	return r.ResolveStaticExpr(call.Func)
}

// ResolveStaticExpr resolves a non-method callee expression through immutable
// symbol/topology facts only.
func (r TargetResolver) ResolveStaticExpr(expr ast.Expr) (summary.FuncRef, bool) {
	if r.Static.FuncBySymbol != nil {
		if ident, ok := expr.(*ast.IdentExpr); ok && ident != nil {
			if sym, ok := r.symbolOf(ident); ok && sym != 0 {
				if ref, ok := r.Static.FuncBySymbol(sym); ok {
					return ref, true
				}
			}
		}
	}
	if r.Static.FieldFunc != nil {
		if sym, field, ok := r.directFieldPath(expr); ok {
			if ref, ok := r.Static.FieldFunc(sym, field); ok {
				return ref, true
			}
		}
	}
	return summary.FuncRef{}, false
}

// ResolveFunctionRefsAtExpr resolves the live product FunctionRefs axis for a
// function-valued expression. The bool reports whether the axis was present and
// therefore authoritative; a present Top set returns (nil, true) and must block
// static fallback.
func (r TargetResolver) ResolveFunctionRefsAtExpr(expr ast.Expr, refs flow.FunctionRefs) ([]summary.FuncRef, bool) {
	return r.directExprRefsFromState(expr, refs)
}

// ResolveCallbackArgRefs resolves a callback argument using the same precedence
// as call target resolution: direct function literal, live FunctionRefs axis,
// then immutable static expression fallback.
func (r TargetResolver) ResolveCallbackArgRefs(
	arg ast.Expr,
	refs flow.FunctionRefs,
	functionLiteral func(*ast.FunctionExpr) (summary.FuncRef, bool),
) ([]summary.FuncRef, bool) {
	return ResolveCallbackArgRefs(CallbackArgInput{
		Arg:             arg,
		FunctionLiteral: functionLiteral,
		FunctionRefs: func(expr ast.Expr) ([]summary.FuncRef, bool) {
			return r.ResolveFunctionRefsAtExpr(expr, refs)
		},
		StaticExpr: func(expr ast.Expr) (summary.FuncRef, bool) {
			return r.ResolveStaticExpr(expr)
		},
	})
}

// ResolveStaticMethod resolves a method call through immutable topology facts
// only: receiver field function first, then current-self prototype method.
func (r TargetResolver) ResolveStaticMethod(call *ast.FuncCallExpr) (summary.FuncRef, bool) {
	if call == nil || call.Method == "" {
		return summary.FuncRef{}, false
	}
	receiver, ok := call.Receiver.(*ast.IdentExpr)
	if !ok || receiver == nil {
		return summary.FuncRef{}, false
	}
	sym, ok := r.symbolOf(receiver)
	if !ok || sym == 0 {
		return summary.FuncRef{}, false
	}
	method, ok := fieldkey.FromName(call.Method)
	if !ok {
		return summary.FuncRef{}, false
	}
	if r.Static.FieldFunc != nil {
		if ref, ok := r.Static.FieldFunc(sym, method); ok {
			return ref, true
		}
	}
	if r.Static.SelfMethodRef != nil {
		if ref, ok := r.Static.SelfMethodRef(sym, method); ok {
			return ref, true
		}
	}
	return summary.FuncRef{}, false
}

func (r TargetResolver) directMethodRefsFromState(call *ast.FuncCallExpr, refs flow.FunctionRefs) ([]summary.FuncRef, bool) {
	path, ok := r.methodPath(call)
	if !ok {
		return nil, false
	}
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return nil, false
	}
	return ref.FromFlowAddress(refs, addr)
}

func (r TargetResolver) directExprRefsFromState(expr ast.Expr, refs flow.FunctionRefs) ([]summary.FuncRef, bool) {
	path, ok := r.exprPath(expr)
	if !ok {
		return nil, false
	}
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return nil, false
	}
	return ref.FromFlowAddress(refs, addr)
}

func (r TargetResolver) closureMethodRefs(call *ast.FuncCallExpr, refs flow.ClosureRefs) ([]flow.ClosureRef, bool) {
	path, ok := r.methodPath(call)
	if !ok {
		return nil, false
	}
	return closureRefsAtPath(refs, path)
}

func (r TargetResolver) closureExprRefs(expr ast.Expr, refs flow.ClosureRefs) ([]flow.ClosureRef, bool) {
	path, ok := r.exprPath(expr)
	if !ok {
		return nil, false
	}
	return closureRefsAtPath(refs, path)
}

func (r TargetResolver) methodPath(call *ast.FuncCallExpr) (constraint.Path, bool) {
	if call == nil || call.Method == "" {
		return constraint.Path{}, false
	}
	path, ok := r.exprPath(call.Receiver)
	if !ok {
		return constraint.Path{}, false
	}
	path.Segments = append(append([]constraint.Segment(nil), path.Segments...), constraint.Segment{
		Kind: constraint.SegmentField,
		Name: call.Method,
	})
	return path, true
}

func (r TargetResolver) directFieldPath(expr ast.Expr) (cfg.SymbolID, fieldkey.Key, bool) {
	path, ok := r.exprPath(expr)
	if !ok || path.Symbol == 0 || len(path.Segments) != 1 {
		return 0, fieldkey.Key{}, false
	}
	key, ok := fieldkey.FromSegment(path.Segments[0])
	return path.Symbol, key, ok
}

func (r TargetResolver) exprPath(expr ast.Expr) (constraint.Path, bool) {
	if r.Bindings == nil || expr == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(expr, nil, r.Bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

func (r TargetResolver) symbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	if r.Bindings == nil || ident == nil {
		return 0, false
	}
	return r.Bindings.SymbolOf(ident)
}

func closureRefsAtPath(refs flow.ClosureRefs, path constraint.Path) ([]flow.ClosureRef, bool) {
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return nil, false
	}
	set, ok := flow.ClosureRefAtAddress(refs, addr)
	if !ok {
		return nil, false
	}
	return set.Refs(), true
}
