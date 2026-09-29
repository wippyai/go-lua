package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// ReceiverComplete reports whether the receivers of class table classSym's
// methods hold only the fields the class and its constructors give them: the
// module sets a metatable that resolves to the class on at least one table, and
// every such table starts as an empty table literal, directly or through a
// local that never receives a dynamic-key write. Its fields then arrive only
// through the writes the class type collects. A literal with its own fields, a
// table copied through dynamic keys, or instances the module never builds may
// hold fields the class type does not list.
func ReceiverComplete(graphs []*cfg.Graph, classSym cfg.SymbolID) bool {
	if classSym == 0 || len(graphs) == 0 {
		return false
	}
	literals := literalLocals(graphs)
	dynamic := dynamicKeyTargets(graphs)
	found := false
	complete := true
	for _, graph := range graphs {
		bindings := graph.Bindings()
		if bindings == nil {
			continue
		}
		graph.EachCallSite(func(_ cfg.Point, info *cfg.CallInfo) {
			if !complete || info == nil || info.CalleeName != "setmetatable" || len(info.Args) < 2 {
				return
			}
			if !metatableResolvesTo(info.Args[1], classSym, bindings, literals) {
				return
			}
			found = true
			if !completeInstanceRoot(info.Args[0], bindings, literals, dynamic) {
				complete = false
			}
		})
	}
	return found && complete
}

// literalLocals maps each local initialized from a table literal to it.
func literalLocals(graphs []*cfg.Graph) map[cfg.SymbolID]*ast.TableExpr {
	out := make(map[cfg.SymbolID]*ast.TableExpr)
	for _, graph := range graphs {
		graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil || !info.IsLocal {
				return
			}
			info.EachTargetSource(func(_ int, target cfg.AssignTarget, source ast.Expr) {
				if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
					return
				}
				if tbl, ok := source.(*ast.TableExpr); ok {
					out[target.Symbol] = tbl
				}
			})
		})
	}
	return out
}

// dynamicKeyTargets lists the symbols written through an index whose key is
// not a string literal, which may name any field.
func dynamicKeyTargets(graphs []*cfg.Graph) map[cfg.SymbolID]bool {
	out := make(map[cfg.SymbolID]bool)
	for _, graph := range graphs {
		graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
			if info == nil {
				return
			}
			for _, target := range info.Targets {
				if target.Kind != cfg.TargetIndex || target.BaseSymbol == 0 {
					continue
				}
				if _, static := target.Key.(*ast.StringExpr); !static {
					out[target.BaseSymbol] = true
				}
			}
		})
	}
	return out
}

// metatableResolvesTo reports whether expr, the metatable argument of
// setmetatable, resolves fields to classSym: the class itself, or a table
// whose __index is the class.
func metatableResolvesTo(expr ast.Expr, classSym cfg.SymbolID, bindings *bind.BindingTable, literals map[cfg.SymbolID]*ast.TableExpr) bool {
	switch mt := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(mt)
		if !ok || sym == 0 {
			return false
		}
		if sym == classSym {
			return true
		}
		if tbl := literals[sym]; tbl != nil {
			return indexFieldIs(tbl, classSym, bindings)
		}
	case *ast.TableExpr:
		return indexFieldIs(mt, classSym, bindings)
	}
	return false
}

func indexFieldIs(tbl *ast.TableExpr, classSym cfg.SymbolID, bindings *bind.BindingTable) bool {
	for _, field := range tbl.Fields {
		if field == nil || ast.KeyName(field.Key) != "__index" {
			continue
		}
		ident, ok := field.Value.(*ast.IdentExpr)
		if !ok {
			return false
		}
		sym, ok := bindings.SymbolOf(ident)
		return ok && sym == classSym
	}
	return false
}

// completeInstanceRoot reports whether expr, the table argument of
// setmetatable, is an empty table literal or a local initialized from one
// without dynamic-key writes.
func completeInstanceRoot(expr ast.Expr, bindings *bind.BindingTable, literals map[cfg.SymbolID]*ast.TableExpr, dynamic map[cfg.SymbolID]bool) bool {
	switch root := expr.(type) {
	case *ast.TableExpr:
		return len(root.Fields) == 0
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(root)
		if !ok || sym == 0 || dynamic[sym] {
			return false
		}
		tbl := literals[sym]
		return tbl != nil && len(tbl.Fields) == 0
	}
	return false
}
