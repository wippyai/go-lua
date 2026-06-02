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

// IndexedSourceSymbol returns the symbol of the array source in an indexed
// iterator call. The provenance proof for that symbol is owned by canonical facts;
// transfer uses this helper only to identify which fact to read.
func IndexedSourceSymbol(iter *ast.FuncCallExpr, bindings *bind.BindingTable, resolveKind KindResolver) (cfg.SymbolID, bool) {
	source, _, ok := sourceArg(iter, effect.IterateIndexed, resolveKind)
	if !ok || bindings == nil {
		return 0, false
	}
	srcIdent, ok := source.(*ast.IdentExpr)
	if !ok {
		return 0, false
	}
	srcSym, ok := bindings.SymbolOf(srcIdent)
	if !ok || srcSym == 0 {
		return 0, false
	}
	return srcSym, true
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
