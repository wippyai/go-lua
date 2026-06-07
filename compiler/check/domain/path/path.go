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
	"github.com/wippyai/go-lua/types/typ"
)

type versionedGraph interface {
	VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version
}

// SymbolLookupAt resolves a visible source name to its graph symbol at a CFG
// point.
type SymbolLookupAt func(cfg.Point, string) (cfg.SymbolID, bool)

// ArrayLenBoundKeyWithOffset projects a numeric `i <= #arr + offset` proof from
// symbol space to the versioned path key expected by indexed-read refiners.
func ArrayLenBoundKeyWithOffset(
	point cfg.Point,
	varName string,
	graph versionedGraph,
	numeric flow.NumericFacts,
	symbolAt SymbolLookupAt,
) (string, int64, bool) {
	if numeric == nil || symbolAt == nil {
		return "", 0, false
	}
	sym, ok := symbolAt(point, varName)
	if !ok {
		return "", 0, false
	}
	arrSym, offset, ok := numeric.ArrayLenRefAt(point, sym)
	if !ok {
		return "", 0, false
	}
	arrPath := WithVersion(constraint.Path{Symbol: arrSym}, graph, point)
	return string(arrPath.Key()), offset, true
}

// StaticKeySegment converts a syntactically static table-constructor key
// expression into a path segment using the hand-built-AST heuristic.
//
// Table-constructor semantics:
//   - identifier field: {foo = v}  -> SegmentField("foo")
//   - unknown string key "foo"     -> SegmentField("foo")
//   - string key: {["x-y"] = v}    -> SegmentIndexString("x-y")
//   - number key: {[1] = v}        -> SegmentIndexInt(1)
//
// Returns false for unsupported or empty keys.
func StaticKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return pathseg.StaticTableFieldKeySegment(key)
}

// StaticFieldSegment converts a full table-constructor field into a path
// segment, preserving parser-produced name-vs-bracket syntax when available.
func StaticFieldSegment(field *ast.Field) (constraint.Segment, bool) {
	return pathseg.StaticTableFieldSegment(field)
}

// StaticFieldSegmentWithConst converts a full table-constructor field into a
// path segment, resolving compile-time constant bracket keys without collapsing
// bracket strings into dot fields.
func StaticFieldSegmentWithConst(field *ast.Field, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	return pathseg.StaticTableFieldSegmentWithConst(field, constResolver)
}

// TableFieldMatchesSegment reports whether a table-constructor field lowers
// exactly to segment under parser-preserved table-key syntax.
func TableFieldMatchesSegment(field *ast.Field, segment constraint.Segment) bool {
	return pathseg.TableFieldMatchesSegment(field, segment)
}

// TableFieldMatchesSegmentWithConst is TableFieldMatchesSegment plus
// compile-time constant resolution for bracket-syntax dynamic identifiers.
func TableFieldMatchesSegmentWithConst(field *ast.Field, segment constraint.Segment, constResolver func(string) *flow.ConstValue) bool {
	return pathseg.TableFieldMatchesSegmentWithConst(field, segment, constResolver)
}

// StaticAttrKeySegment converts a syntactically static attribute/index key into
// a path segment using the hand-built-AST heuristic.
//
// Attribute semantics:
//   - dot field: obj.foo            -> SegmentField("foo")
//   - unknown string key "foo"      -> SegmentField("foo")
//   - string key: obj["x-y"]        -> SegmentIndexString("x-y")
//   - number key: obj[1]            -> SegmentIndexInt(1)
//   - dynamic identifier: obj[key]  -> rejected
func StaticAttrKeySegment(key ast.Expr) (constraint.Segment, bool) {
	return pathseg.StaticAttrKeySegment(key)
}

// StaticAttrSegment converts a full attribute/index expression into a path
// segment using the parser's dot-vs-bracket syntax bit when available.
func StaticAttrSegment(attr *ast.AttrGetExpr) (constraint.Segment, bool) {
	return pathseg.StaticAttrSegment(attr)
}

// StaticAttrKeySegmentWithConst resolves compile-time constant attribute/index
// keys before applying StaticAttrKeySegment. Non-constant identifier keys remain
// dynamic and are rejected.
func StaticAttrKeySegmentWithConst(key ast.Expr, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	if seg, ok := StaticAttrKeySegment(key); ok {
		return seg, true
	}
	ident, ok := key.(*ast.IdentExpr)
	if !ok || constResolver == nil {
		return constraint.Segment{}, false
	}
	val := constResolver(ident.Value)
	if val == nil {
		return constraint.Segment{}, false
	}
	switch val.Kind {
	case flow.ConstString:
		return StaticAttrKeySegment(&ast.StringExpr{Value: val.Str})
	case flow.ConstInt:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)}, true
	case flow.ConstFloat:
		if idx, ok := pathkey.FloatToSafeInt(val.Float); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
	}
	return constraint.Segment{}, false
}

// StaticAttrSegmentWithConst resolves compile-time constant attribute/index keys
// using the AttrGetExpr syntax bit. Parsed x["foo"] remains a string-index path,
// while parsed x.foo remains a dot-field path. AttrKeyUnknown preserves the
// compatibility behavior for manually constructed ASTs.
func StaticAttrSegmentWithConst(attr *ast.AttrGetExpr, constResolver func(string) *flow.ConstValue) (constraint.Segment, bool) {
	if attr == nil {
		return constraint.Segment{}, false
	}
	if seg, ok := StaticAttrSegment(attr); ok {
		return seg, true
	}
	ident, ok := attr.Key.(*ast.IdentExpr)
	if !ok || constResolver == nil {
		return constraint.Segment{}, false
	}
	val := constResolver(ident.Value)
	if val == nil {
		return constraint.Segment{}, false
	}
	switch val.Kind {
	case flow.ConstString:
		return pathseg.StaticAttrKeySegmentWithSyntax(&ast.StringExpr{Value: val.Str}, attr.KeySyntax)
	case flow.ConstInt:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(val.Int)}, true
	case flow.ConstFloat:
		if idx, ok := pathkey.FloatToSafeInt(val.Float); ok {
			return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
		}
	}
	return constraint.Segment{}, false
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
		base := FromExprWithBindings(e.Object, constResolver, bindings)
		if base.IsEmpty() {
			return constraint.Path{}
		}
		if seg, ok := StaticAttrSegmentWithConst(e, constResolver); ok {
			return base.Append(seg)
		}
	}
	return constraint.Path{}
}

// FromExprWithBindingsAt extracts a flow path using bindings and binds it to the SSA version at point p.
func FromExprWithBindingsAt(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable, graph versionedGraph, p cfg.Point) constraint.Path {
	path := FromExprWithBindings(expr, constResolver, bindings)
	return WithVersion(path, graph, p)
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
