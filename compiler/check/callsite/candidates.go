package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

type symbolSet struct {
	order []cfg.SymbolID
	seen  map[cfg.SymbolID]struct{}
}

func newSymbolSet(capacity int) *symbolSet {
	return &symbolSet{
		order: make([]cfg.SymbolID, 0, capacity),
		seen:  make(map[cfg.SymbolID]struct{}, capacity),
	}
}

func (s *symbolSet) Add(sym cfg.SymbolID) {
	if sym == 0 {
		return
	}
	if _, ok := s.seen[sym]; ok {
		return
	}
	s.seen[sym] = struct{}{}
	s.order = append(s.order, sym)
}

func (s *symbolSet) Slice() []cfg.SymbolID {
	return s.order
}

func selectPreferredSymbol(candidates []cfg.SymbolID, prefer func(cfg.SymbolID) bool) cfg.SymbolID {
	selected := cfg.SymbolID(0)
	for _, sym := range candidates {
		if selected == 0 {
			selected = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return selected
}

func addAliasExpansion(set *symbolSet, graph *cfg.Graph, sym cfg.SymbolID) {
	if set == nil || graph == nil || sym == 0 {
		return
	}
	graph.EachAliasSymbol(sym, func(candidate cfg.SymbolID) bool {
		set.Add(candidate)
		return false
	})
}

func exprSymbolCandidates(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
) []cfg.SymbolID {
	set := newSymbolSet(3)
	set.Add(raw)
	set.Add(SymbolFromExpr(expr, primary))
	if fallback != primary {
		set.Add(SymbolFromExpr(expr, fallback))
	}
	return set.Slice()
}
