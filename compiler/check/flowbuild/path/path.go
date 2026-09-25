// Path extraction from AST expressions.
//
// IDENTITY MODEL:
// FromExprWithBindings resolves symbols from AST nodes via bindings, not by name lookup.
// This ensures paths have stable symbol identity across function boundaries.
//
// For constraint extraction, always use FromExprWithBindings.
package path

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type versionedGraph interface {
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}

// StaticKeySegment converts a syntactically static key expression into a path segment.
//
// Supported static keys:
//   - identifier key: foo        -> SegmentField("foo")
//   - string key: "foo"          -> SegmentField("foo")
//   - string key: "x-y"          -> SegmentIndexString("x-y")
//   - number key: 1              -> SegmentIndexInt(1)
//
// Returns false for unsupported or empty keys.
func StaticKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return pathseg.StaticTableFieldKeySegment(key)
}

// KeyTypeSegment converts a key whose type is exactly one string literal into
// a path segment: t[k] with k: "foo" addresses the same slot as t.foo.
func KeyTypeSegment(keyType typ.Type) (constraint.Segment, bool) {
	key, ok := querycore.ExactStringKey(keyType)
	if !ok {
		return constraint.Segment{}, false
	}
	return StaticKeySegment(&ast.StringExpr{Value: key})
}

// WithVersion binds a path to the SSA version visible at point p.
// If the path is unversioned or the version is unavailable, it is returned unchanged.
func WithVersion(path constraint.Path, graph versionedGraph, p cfg.Point) constraint.Path {
	if path.IsEmpty() || path.Symbol == 0 || graph == nil {
		return path
	}
	ver := graph.VisibleVersion(p, path.Symbol)
	if ver.IsZero() {
		return path
	}
	path.Version = ver.ID
	return path
}

// FromExprWithBindings extracts a flow path using bindings for symbol resolution.
// Resolves symbols from AST nodes directly.
func FromExprWithBindings(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable) constraint.Path {
	return FromExprWithKeyTypes(expr, constResolver, bindings, nil)
}

// FromExprWithKeyTypes extracts a flow path like FromExprWithBindings, and
// also resolves index keys through keyType: t[k] with k typed exactly as one
// string literal addresses a static field of t.
func FromExprWithKeyTypes(
	expr ast.Expr,
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
	keyType func(ast.Expr) typ.Type,
) constraint.Path {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		var sym cfg.SymbolID
		if bindings != nil {
			sym, _ = bindings.SymbolOf(e)
		}
		if sym != 0 {
			root := e.Value
			if bindings != nil {
				if name := bindings.Name(sym); name != "" {
					root = name
				}
			}
			return constraint.Path{Root: root, Symbol: sym}
		}
		return constraint.Path{Root: e.Value}
	case *ast.AttrGetExpr:
		base := FromExprWithKeyTypes(e.Object, constResolver, bindings, keyType)
		if base.IsEmpty() {
			return constraint.Path{}
		}
		seg, ok := IndexKeySegment(e.Key, constResolver, keyType)
		if !ok {
			return constraint.Path{}
		}
		return base.Append(seg)
	}
	return constraint.Path{}
}

// FromExprWithKeyTypesThroughCasts extracts the identity of a value used by a
// guard or a narrowed read. Casts do not change the Lua value they contain.
// Assignment and mutation tracking use FromExprWithKeyTypes instead, since a
// cast's declared type must remain authoritative for those operations.
func FromExprWithKeyTypesThroughCasts(
	expr ast.Expr,
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
	keyType func(ast.Expr) typ.Type,
) constraint.Path {
	switch e := expr.(type) {
	case *ast.CastExpr:
		return FromExprWithKeyTypesThroughCasts(e.Expr, constResolver, bindings, keyType)
	case *ast.AttrGetExpr:
		base := FromExprWithKeyTypesThroughCasts(e.Object, constResolver, bindings, keyType)
		if base.IsEmpty() {
			return constraint.Path{}
		}
		seg, ok := IndexKeySegment(e.Key, constResolver, keyType)
		if !ok {
			return constraint.Path{}
		}
		return base.Append(seg)
	default:
		return FromExprWithKeyTypes(expr, constResolver, bindings, keyType)
	}
}

// IndexKeySegment classifies the key of an index expression t[key] as a static
// path segment. A key is static when it is a string or integer literal, an
// identifier bound to a string or integral constant, or, given keyType, an
// expression whose type is exactly one string literal.
func IndexKeySegment(
	key ast.Expr,
	constResolver func(string) *flow.ConstValue,
	keyType func(ast.Expr) typ.Type,
) (constraint.Segment, bool) {
	switch k := key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return pathseg.StaticAttrKeySegment(k)
	case *ast.IdentExpr:
		if constResolver != nil {
			if val := constResolver(k.Value); val != nil {
				switch val.Kind {
				case flow.ConstString:
					return StaticKeySegment(&ast.StringExpr{Value: val.Str})
				case flow.ConstInt:
					return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)}, true
				case flow.ConstFloat:
					if idx, ok := pathkey.FloatToSafeInt(val.Float); ok {
						return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
					}
				}
				return constraint.Segment{}, false
			}
		}
	}
	if keyType == nil || key == nil {
		return constraint.Segment{}, false
	}
	return KeyTypeSegment(keyType(key))
}

// FromExprWithBindingsAt extracts a flow path using bindings and binds it to the SSA version at point p.
func FromExprWithBindingsAt(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable, graph versionedGraph, p cfg.Point) constraint.Path {
	return FromExprWithKeyTypesAt(expr, constResolver, bindings, nil, graph, p)
}

// FromExprWithKeyTypesAt extracts a flow path like FromExprWithKeyTypes and
// binds it to the SSA version at point p.
func FromExprWithKeyTypesAt(
	expr ast.Expr,
	constResolver func(string) *flow.ConstValue,
	bindings *bind.BindingTable,
	keyType func(ast.Expr) typ.Type,
	graph versionedGraph,
	p cfg.Point,
) constraint.Path {
	return WithVersion(FromExprWithKeyTypes(expr, constResolver, bindings, keyType), graph, p)
}

// SplitIndexPath splits a path into base and index key.
func SplitIndexPath(path constraint.Path) (constraint.Path, typ.Type, bool) {
	if path.IsEmpty() || len(path.Segments) == 0 {
		return constraint.Path{}, nil, false
	}
	last := path.Segments[len(path.Segments)-1]
	var key typ.Type
	switch last.Kind {
	case constraint.SegmentIndexString:
		key = typ.LiteralString(last.Name)
	case constraint.SegmentIndexInt:
		key = typ.LiteralInt(int64(last.Index))
	default:
		return constraint.Path{}, nil, false
	}
	base := constraint.Path{Root: path.Root, Symbol: path.Symbol, Version: path.Version}
	if len(path.Segments) > 1 {
		base.Segments = append(base.Segments, path.Segments[:len(path.Segments)-1]...)
	}
	return base, key, true
}

// TypeOfCallPathWithBindings extracts the path argument from a type() call using bindings.
func TypeOfCallPathWithBindings(expr ast.Expr, bindings *bind.BindingTable) (constraint.Path, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return constraint.Path{}, false
	}
	if callsite.IsMethodLikeExpr(call) {
		return constraint.Path{}, false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident.Value != "type" {
		return constraint.Path{}, false
	}
	if len(call.Args) != 1 {
		return constraint.Path{}, false
	}
	path := FromExprWithBindings(call.Args[0], nil, bindings)
	if path.IsEmpty() {
		return constraint.Path{}, false
	}
	return path, true
}
