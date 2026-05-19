package iteration

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
)

// KeyedPairValue identifies the value variable paired with a pairs() key.
type KeyedPairValue struct {
	TablePath constraint.Path
	KeyPath   constraint.Path
	ValuePath constraint.Path
}

// FindKeyedPairValue returns the table/key/value relation introduced by:
//
//	for key, value in pairs(table) do
//
// The relation is pure provenance. Callers decide how much type evidence to
// take from the paired value; this query only proves that the key symbol was
// introduced by the same iterator as the value symbol.
func FindKeyedPairValue(graph *cfg.Graph, table ast.Expr, key *ast.IdentExpr) (KeyedPairValue, bool) {
	if graph == nil || table == nil || key == nil {
		return KeyedPairValue{}, false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return KeyedPairValue{}, false
	}

	keySym, ok := bindings.SymbolOf(key)
	if !ok || keySym == 0 {
		return KeyedPairValue{}, false
	}
	tablePath := path.FromExprWithBindings(table, nil, bindings)
	if tablePath.IsEmpty() {
		return KeyedPairValue{}, false
	}

	var result KeyedPairValue
	found := false
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if found || info == nil || len(info.IterExprs) == 0 || len(info.Targets) < 2 {
			return
		}
		keyTarget := info.Targets[0]
		valueTarget := info.Targets[1]
		if keyTarget.Kind != cfg.TargetIdent || keyTarget.Symbol != keySym ||
			valueTarget.Kind != cfg.TargetIdent || valueTarget.Symbol == 0 {
			return
		}
		source, ok := builtinPairsSource(info.IterExprs[0], bindings)
		if !ok {
			return
		}
		sourcePath := path.FromExprWithBindings(source, nil, bindings)
		if !sourcePath.Equal(tablePath) {
			return
		}
		result = KeyedPairValue{
			TablePath: tablePath,
			KeyPath: constraint.Path{
				Root:   key.Value,
				Symbol: keySym,
			},
			ValuePath: constraint.Path{
				Root:   valueTarget.Name,
				Symbol: valueTarget.Symbol,
			},
		}
		found = true
	})

	return result, found
}

func builtinPairsSource(expr ast.Expr, bindings interface {
	SymbolOf(*ast.IdentExpr) (cfg.SymbolID, bool)
	Kind(cfg.SymbolID) (cfg.SymbolKind, bool)
}) (ast.Expr, bool) {
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) == 0 || call.Method != "" || call.Receiver != nil {
		return nil, false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	if !ok || ident == nil || ident.Value != "pairs" {
		return nil, false
	}
	if bindings != nil {
		if sym, ok := bindings.SymbolOf(ident); ok && sym != 0 {
			if kind, ok := bindings.Kind(sym); ok && kind != cfg.SymbolGlobal {
				return nil, false
			}
		}
	}
	return call.Args[0], true
}
