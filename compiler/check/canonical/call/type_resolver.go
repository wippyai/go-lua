package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// StaticTypeLookup supplies immutable callable-type facts. It intentionally
// contains callbacks, not program/driver pointers, so the resolution precedence
// can be tested as a pure canonical/call policy.
type StaticTypeLookup struct {
	FuncBySymbol   func(cfg.SymbolID) (typ.Type, bool)
	FieldFunc      func(cfg.SymbolID, fieldkey.Key) (typ.Type, bool)
	ImportedBase   func(cfg.SymbolID) (typ.Type, bool)
	GlobalBySymbol func(cfg.SymbolID) (typ.Type, bool)
	GlobalByName   func(string) (typ.Type, bool)
}

// TypeResolver resolves caller-visible callable types. Solved product-state
// expression types win; immutable facts are fallbacks.
type TypeResolver struct {
	Bindings *bind.BindingTable
	ExprType func(ast.Expr) typ.Type
	Ctx      *db.QueryContext
	Query    core.TypeOps
	Static   StaticTypeLookup
}

// ResolveCallee resolves a non-method call callee type.
func (r TypeResolver) ResolveCallee(expr ast.Expr) typ.Type {
	if expr == nil {
		return nil
	}
	if ident, ok := expr.(*ast.IdentExpr); ok && ident != nil {
		if t := r.exprType(expr); !typ.IsAbsentOrUnknown(t) {
			return t
		}
		if sig := r.funcByIdent(ident); sig != nil {
			return sig
		}
		if t := r.ResolveGlobalIdentType(ident); !typ.IsAbsentOrUnknown(t) {
			return t
		}
		return nil
	}
	if t := r.exprType(expr); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	return r.ResolveStaticFieldCallee(expr)
}

// ResolveReceiver resolves a method-call receiver type.
func (r TypeResolver) ResolveReceiver(expr ast.Expr) typ.Type {
	if expr == nil {
		return nil
	}
	if t := r.exprType(expr); !typ.IsAbsentOrUnknown(t) {
		return t
	}
	if ident, ok := expr.(*ast.IdentExpr); ok && ident != nil {
		if sig := r.funcByIdent(ident); sig != nil {
			return sig
		}
		if t := r.ResolveGlobalIdentType(ident); !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	return nil
}

// ResolveStaticCallee resolves only immutable non-product fallback signatures.
func (r TypeResolver) ResolveStaticCallee(expr ast.Expr) typ.Type {
	if ident, ok := expr.(*ast.IdentExpr); ok && ident != nil {
		if sig := r.funcByIdent(ident); sig != nil {
			return sig
		}
		return r.ResolveGlobalIdentType(ident)
	}
	return r.ResolveStaticFieldCallee(expr)
}

// ResolveGlobalIdentType resolves a global identifier using symbol-specific facts
// before external name lookup.
func (r TypeResolver) ResolveGlobalIdentType(ident *ast.IdentExpr) typ.Type {
	if ident == nil {
		return nil
	}
	if sym, ok := r.globalSymbolOf(ident); ok && r.Static.GlobalBySymbol != nil {
		if t, ok := r.Static.GlobalBySymbol(sym); ok && t != nil {
			return t
		}
	}
	if r.Static.GlobalByName == nil {
		return nil
	}
	t, ok := r.Static.GlobalByName(ident.Value)
	if !ok {
		return nil
	}
	return t
}

// ResolveStaticFieldCallee resolves local field-function facts before imported
// module/global member types.
func (r TypeResolver) ResolveStaticFieldCallee(expr ast.Expr) typ.Type {
	sym, field, ok := r.directFieldPath(expr)
	if !ok {
		return nil
	}
	if r.Static.FieldFunc != nil {
		if sig, ok := r.Static.FieldFunc(sym, field); ok && sig != nil {
			return sig
		}
	}
	if t := r.staticMemberFromBase(sym, field, r.Static.GlobalBySymbol); t != nil {
		return t
	}
	return r.ResolveImportedFieldCallee(expr)
}

// ResolveImportedFieldCallee resolves a field-path callee through an immutable
// imported base type.
func (r TypeResolver) ResolveImportedFieldCallee(expr ast.Expr) typ.Type {
	sym, field, ok := r.directFieldPath(expr)
	if !ok || r.Static.ImportedBase == nil {
		return nil
	}
	base, ok := r.Static.ImportedBase(sym)
	if !ok || base == nil {
		return nil
	}
	ft, ok := r.staticMemberType(base, field)
	if !ok || typ.IsAbsentOrUnknown(ft) {
		return nil
	}
	return ft
}

func (r TypeResolver) staticMemberFromBase(sym cfg.SymbolID, field fieldkey.Key, lookup func(cfg.SymbolID) (typ.Type, bool)) typ.Type {
	if sym == 0 || lookup == nil {
		return nil
	}
	base, ok := lookup(sym)
	if !ok || base == nil {
		return nil
	}
	ft, ok := r.staticMemberType(base, field)
	if !ok || typ.IsAbsentOrUnknown(ft) {
		return nil
	}
	return ft
}

// StaticMemberType reads a static field/index member from base.
func StaticMemberType(base typ.Type, key fieldkey.Key) (typ.Type, bool) {
	switch key.Kind {
	case constraint.SegmentField:
		if key.Name == "" {
			return nil, false
		}
		return core.Field(base, key.Name)
	case constraint.SegmentIndexString:
		return core.Index(base, typ.LiteralString(key.Name))
	case constraint.SegmentIndexInt:
		return core.Index(base, typ.LiteralInt(int64(key.Index)))
	default:
		return nil, false
	}
}

func (r TypeResolver) staticMemberType(base typ.Type, key fieldkey.Key) (typ.Type, bool) {
	switch key.Kind {
	case constraint.SegmentField:
		if key.Name == "" {
			return nil, false
		}
		if r.Query != nil && r.Ctx != nil {
			return r.Query.Field(r.Ctx, base, key.Name)
		}
		return core.Field(base, key.Name)
	case constraint.SegmentIndexString:
		keyType := typ.LiteralString(key.Name)
		if r.Query != nil && r.Ctx != nil {
			return r.Query.Index(r.Ctx, base, keyType)
		}
		return core.Index(base, keyType)
	case constraint.SegmentIndexInt:
		keyType := typ.LiteralInt(int64(key.Index))
		if r.Query != nil && r.Ctx != nil {
			return r.Query.Index(r.Ctx, base, keyType)
		}
		return core.Index(base, keyType)
	default:
		return nil, false
	}
}

func (r TypeResolver) method(receiver typ.Type, name string) (typ.Type, bool) {
	if r.Query != nil && r.Ctx != nil {
		return r.Query.Method(r.Ctx, receiver, name)
	}
	return core.Method(receiver, name)
}

func (r TypeResolver) exprType(expr ast.Expr) typ.Type {
	if r.ExprType == nil {
		return nil
	}
	return r.ExprType(expr)
}

func (r TypeResolver) funcByIdent(ident *ast.IdentExpr) typ.Type {
	if r.Static.FuncBySymbol == nil {
		return nil
	}
	sym, ok := r.symbolOf(ident)
	if !ok || sym == 0 {
		return nil
	}
	sig, ok := r.Static.FuncBySymbol(sym)
	if !ok {
		return nil
	}
	return sig
}

func (r TypeResolver) directFieldPath(expr ast.Expr) (cfg.SymbolID, fieldkey.Key, bool) {
	path, ok := r.exprPath(expr)
	if !ok || path.Symbol == 0 || len(path.Segments) != 1 {
		return 0, fieldkey.Key{}, false
	}
	key, ok := fieldkey.FromSegment(path.Segments[0])
	return path.Symbol, key, ok
}

func (r TypeResolver) exprPath(expr ast.Expr) (constraint.Path, bool) {
	if r.Bindings == nil || expr == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(expr, nil, r.Bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

func (r TypeResolver) symbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	if r.Bindings == nil || ident == nil {
		return 0, false
	}
	return r.Bindings.SymbolOf(ident)
}

func (r TypeResolver) globalSymbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool) {
	sym, ok := r.symbolOf(ident)
	if !ok || sym == 0 || r.Bindings == nil {
		return 0, false
	}
	if k, ok := r.Bindings.Kind(sym); !ok || k != cfg.SymbolGlobal {
		return 0, false
	}
	return sym, true
}
