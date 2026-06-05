package iteration

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/effect"
)

// KindResolver resolves a generic-for iterator call into its abstract iterator
// kind and source runtime argument index.
type KindResolver func(iter *ast.FuncCallExpr) (effect.IteratorKind, int, bool)

// KeyedSource returns the iterated source expression for a keyed iterator.
func KeyedSource(iter *ast.FuncCallExpr, resolveKind KindResolver) (ast.Expr, bool) {
	source, _, ok := sourceArg(iter, effect.IterateKeyed, resolveKind)
	return source, ok
}

// IndexedSourcePath returns the static array source path in an indexed iterator
// call. The provenance proof for that path is owned by canonical facts; transfer
// uses this helper only to identify which fact to read.
func IndexedSourcePath(iter *ast.FuncCallExpr, bindings *bind.BindingTable, resolveKind KindResolver) (constraint.Path, bool) {
	source, _, ok := sourceArg(iter, effect.IterateIndexed, resolveKind)
	if !ok || bindings == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(source, nil, bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

// IndexedSourceSymbol returns the root symbol of the array source in an indexed
// iterator call. Prefer IndexedSourcePath for new flow facts so static field paths
// are preserved.
func IndexedSourceSymbol(iter *ast.FuncCallExpr, bindings *bind.BindingTable, resolveKind KindResolver) (cfg.SymbolID, bool) {
	path, ok := IndexedSourcePath(iter, bindings, resolveKind)
	if !ok || path.Symbol == 0 {
		return 0, false
	}
	return path.Symbol, true
}

// ContainerPath builds a static container path from a keys-collector actual
// argument. Non-static or unresolved arguments have no provenance.
func ContainerPath(arg ast.Expr, bindings *bind.BindingTable) (constraint.Path, bool) {
	if bindings == nil || arg == nil {
		return constraint.Path{}, false
	}
	path := flowpath.FromExprWithBindings(arg, nil, bindings)
	if path.IsEmpty() || path.Symbol == 0 {
		return constraint.Path{}, false
	}
	return path, true
}

func sourceArg(iter *ast.FuncCallExpr, want effect.IteratorKind, resolveKind KindResolver) (ast.Expr, int, bool) {
	if iter == nil || iter.Method != "" || resolveKind == nil {
		return nil, 0, false
	}
	kind, srcIdx, ok := resolveKind(iter)
	if !ok || kind != want || srcIdx < 0 || srcIdx >= len(iter.Args) {
		return nil, 0, false
	}
	return iter.Args[srcIdx], srcIdx, true
}
