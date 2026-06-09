package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// StaticTargetLookup is the immutable topology projection used when solved
// FunctionRefs/ClosureRefs do not provide an authoritative call target.
type StaticTargetLookup struct {
	FuncBySymbol  func(cfg.SymbolID) (summary.FuncRef, bool)
	FieldFunc     func(cfg.SymbolID, fieldkey.Key) (summary.FuncRef, bool)
	SelfMethodRef func(self cfg.SymbolID, method fieldkey.Key) (summary.FuncRef, bool)
}

// TargetResolver resolves call targets from normalized product axes and immutable
// topology facts. It performs no summary reads and owns the callable precedence
// rule's data preparation; TargetSet/TargetSelection own the precedence itself.
type TargetResolver struct {
	Graph    *cfg.Graph
	Bindings *bind.BindingTable
	Static   StaticTargetLookup
}

// Resolve returns the normalized target set for call.
func (r TargetResolver) Resolve(
	call *ast.FuncCallExpr,
	references flow.ReferenceContext,
) TargetSet {
	if call == nil {
		return TargetSet{}
	}
	functionRefs := references.FunctionRefs()
	closureRefs := references.ClosureRefs()

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
		if sym, field, ok := (staticAccess{Bindings: r.Bindings}).directField(expr); ok {
			if ref, ok := r.Static.FieldFunc(sym, field); ok {
				return ref, true
			}
		}
	}
	return summary.FuncRef{}, false
}

// ResolveStaticExprOrSymbol resolves immutable callback/callee topology for an
// expression, expanding the CFG-provided raw symbol through the same direct-alias
// candidate order used by call-site evidence. This is intentionally function-ref
// only: field and method topology are ordinary callee resolution, not callback
// argument alias evidence.
func (r TargetResolver) ResolveStaticExprOrSymbol(expr ast.Expr, rawSym cfg.SymbolID) (summary.FuncRef, bool) {
	if r.Static.FuncBySymbol == nil {
		return summary.FuncRef{}, false
	}
	sym := callsite.CanonicalSymbolFromExprWithAliases(
		expr,
		rawSym,
		r.Graph,
		r.Bindings,
		r.Bindings,
		func(candidate cfg.SymbolID) bool {
			_, ok := r.Static.FuncBySymbol(candidate)
			return ok
		},
	)
	return r.Static.FuncBySymbol(sym)
}

// ResolveFunctionRefsAtExpr resolves the live product FunctionRefs axis for a
// function-valued expression. The bool reports whether the axis was present and
// therefore authoritative; a present Top set returns (nil, true) and must block
// immutable topology projection.
func (r TargetResolver) ResolveFunctionRefsAtExpr(expr ast.Expr, refs flow.FunctionRefs) ([]summary.FuncRef, bool) {
	return r.directExprRefsFromState(expr, refs)
}

// ResolveFunctionRefsAtExprOrSymbol resolves live FunctionRefs for expr, falling
// back to rawSym when the CFG call-site already computed a canonical argument
// symbol but the expression itself does not carry a static path.
func (r TargetResolver) ResolveFunctionRefsAtExprOrSymbol(expr ast.Expr, refs flow.FunctionRefs, rawSym cfg.SymbolID) ([]summary.FuncRef, bool) {
	if got, ok := r.ResolveFunctionRefsAtExpr(expr, refs); ok {
		return got, true
	}
	if rawSym == 0 {
		return nil, false
	}
	addr, ok := flow.StableAddressOfSymbol(rawSym, nil)
	if !ok {
		return nil, false
	}
	return ref.FromFlowAddress(refs, addr)
}

// ResolveClosureRefSetAtExpr resolves the live product ClosureRefs axis for a
// function-valued expression. The bool has the same authority meaning as
// ResolveFunctionRefsAtExpr: a present Top set returns (Top, true).
func (r TargetResolver) ResolveClosureRefSetAtExpr(expr ast.Expr, refs flow.ClosureRefs) (flow.ClosureRefSet, bool) {
	path, ok := (staticAccess{Bindings: r.Bindings}).exprPath(expr)
	if !ok {
		return flow.ClosureRefSet{}, false
	}
	return flow.ClosureRefAtPath(refs, path)
}

// ResolveCallbackArgRefs resolves a callback argument using the same precedence
// as call target resolution: direct function literal, live FunctionRefs axis,
// then immutable topology projection.
func (r TargetResolver) ResolveCallbackArgRefs(
	arg ast.Expr,
	references flow.ReferenceContext,
	functionLiteral func(*ast.FunctionExpr) (summary.FuncRef, bool),
) ([]summary.FuncRef, bool) {
	return r.ResolveCallbackArgRefsOrSymbol(arg, references, 0, functionLiteral)
}

// ResolveCallbackArgRefsOrSymbol resolves a callback argument through the same
// callback policy as ResolveCallbackArgRefs, additionally honoring a CFG raw
// symbol when the call-site already captured one. The raw symbol is only a
// lookup candidate: live FunctionRefs remain authoritative and block immutable
// topology projection when present but unknown.
func (r TargetResolver) ResolveCallbackArgRefsOrSymbol(
	arg ast.Expr,
	references flow.ReferenceContext,
	rawSym cfg.SymbolID,
	functionLiteral func(*ast.FunctionExpr) (summary.FuncRef, bool),
) ([]summary.FuncRef, bool) {
	if arg == nil {
		return nil, false
	}
	if fn, ok := arg.(*ast.FunctionExpr); ok && fn != nil && functionLiteral != nil {
		if ref, ok := functionLiteral(fn); ok {
			return []summary.FuncRef{ref}, true
		}
	}
	if got, ok := r.ResolveFunctionRefsAtExprOrSymbol(arg, references.FunctionRefs(), rawSym); ok {
		return ref.UniqueSortedFuncRefs(got), true
	}
	if got, ok := r.ResolveStaticExprOrSymbol(arg, rawSym); ok {
		return []summary.FuncRef{got}, true
	}
	return nil, false
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
	path, ok := (staticAccess{Bindings: r.Bindings}).methodPath(call)
	if !ok {
		return nil, false
	}
	return ref.FromFlowStructuredPath(refs, path)
}

func (r TargetResolver) directExprRefsFromState(expr ast.Expr, refs flow.FunctionRefs) ([]summary.FuncRef, bool) {
	path, ok := (staticAccess{Bindings: r.Bindings}).exprPath(expr)
	if !ok {
		return nil, false
	}
	return ref.FromFlowStructuredPath(refs, path)
}

func (r TargetResolver) closureMethodRefs(call *ast.FuncCallExpr, refs flow.ClosureRefs) ([]flow.ClosureRef, bool) {
	path, ok := (staticAccess{Bindings: r.Bindings}).methodPath(call)
	if !ok {
		return nil, false
	}
	return closureRefsAtPath(refs, path)
}

func (r TargetResolver) closureExprRefs(expr ast.Expr, refs flow.ClosureRefs) ([]flow.ClosureRef, bool) {
	path, ok := (staticAccess{Bindings: r.Bindings}).exprPath(expr)
	if !ok {
		return nil, false
	}
	return closureRefsAtPath(refs, path)
}

func (r TargetResolver) symbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	if r.Bindings == nil || ident == nil {
		return 0, false
	}
	return r.Bindings.SymbolOf(ident)
}

func closureRefsAtPath(refs flow.ClosureRefs, path constraint.Path) ([]flow.ClosureRef, bool) {
	set, ok := flow.ClosureRefAtPath(refs, path)
	if !ok {
		return nil, false
	}
	return set.Refs(), true
}
