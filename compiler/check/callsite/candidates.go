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

const symbolSetMapThreshold = 8

func newSymbolSet(capacity int) *symbolSet {
	if capacity < 0 {
		capacity = 0
	}

	return &symbolSet{
		order: make([]cfg.SymbolID, 0, capacity),
	}
}

func (s *symbolSet) ensureSeen() {
	if s.seen != nil {
		return
	}

	s.seen = make(map[cfg.SymbolID]struct{}, len(s.order)+1)
	for _, existing := range s.order {
		s.seen[existing] = struct{}{}
	}
}

func (s *symbolSet) Add(sym cfg.SymbolID) {
	if sym == 0 {
		return
	}

	if s.seen != nil {
		if _, ok := s.seen[sym]; ok {
			return
		}
		s.seen[sym] = struct{}{}
		s.order = append(s.order, sym)
		return
	}

	for _, existing := range s.order {
		if existing == sym {
			return
		}
	}

	if len(s.order) >= symbolSetMapThreshold {
		s.ensureSeen()
		s.seen[sym] = struct{}{}
	}

	s.order = append(s.order, sym)
}

func (s *symbolSet) Slice() []cfg.SymbolID {
	return s.order
}

type symbolDeduper struct {
	small [symbolSetMapThreshold]cfg.SymbolID
	count int
	seen  map[cfg.SymbolID]struct{}
}

func (d *symbolDeduper) Add(sym cfg.SymbolID) bool {
	if sym == 0 {
		return false
	}
	if d.seen != nil {
		if _, ok := d.seen[sym]; ok {
			return false
		}
		d.seen[sym] = struct{}{}
		return true
	}
	for i := 0; i < d.count; i++ {
		if d.small[i] == sym {
			return false
		}
	}
	if d.count < len(d.small) {
		d.small[d.count] = sym
		d.count++
		return true
	}
	d.seen = make(map[cfg.SymbolID]struct{}, len(d.small)+1)
	for i := 0; i < d.count; i++ {
		d.seen[d.small[i]] = struct{}{}
	}
	d.seen[sym] = struct{}{}
	return true
}

type preferredSymbolSelector struct {
	prefer   func(cfg.SymbolID) bool
	selected cfg.SymbolID
	seen     symbolDeduper
}

func (s *preferredSymbolSelector) Add(sym cfg.SymbolID) bool {
	if !s.seen.Add(sym) {
		return false
	}
	if s.selected == 0 {
		s.selected = sym
	}
	if s.prefer != nil && s.prefer(sym) {
		s.selected = sym
		return true
	}
	return false
}

// SelectPreferredSymbol returns the first candidate and, if prefer is non-nil, returns
// the first candidate that satisfies the predicate.
func SelectPreferredSymbol(candidates []cfg.SymbolID, prefer func(cfg.SymbolID) bool) cfg.SymbolID {
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

func visitExprSymbolCandidates(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	secondary *bind.BindingTable,
	visit func(cfg.SymbolID) bool,
) bool {
	if visit == nil {
		return false
	}
	if visit(raw) {
		return true
	}
	if visit(SymbolFromExpr(expr, primary)) {
		return true
	}
	if secondary != primary {
		return visit(SymbolFromExpr(expr, secondary))
	}
	return false
}

func addExprSymbolCandidates(
	set *symbolSet,
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	secondary *bind.BindingTable,
) {
	if set == nil {
		return
	}
	visitExprSymbolCandidates(expr, raw, primary, secondary, func(sym cfg.SymbolID) bool {
		set.Add(sym)
		return false
	})
}

func addAliasExpansion(set *symbolSet, graph *cfg.Graph, sym cfg.SymbolID) {
	if set == nil || graph == nil || sym == 0 {
		return
	}
	visitAliasExpansion(graph, sym, func(candidate cfg.SymbolID) bool {
		set.Add(candidate)
		return false
	})
}

func visitAliasExpansion(graph *cfg.Graph, sym cfg.SymbolID, visit func(cfg.SymbolID) bool) bool {
	if sym == 0 || visit == nil {
		return false
	}
	if graph == nil {
		return visit(sym)
	}
	var chain symbolDeduper
	current := sym
	for current != 0 {
		if !chain.Add(current) {
			return false
		}
		if visit(current) {
			return true
		}
		next := graph.DirectAliasSymbol(current)
		if next == 0 || next == current {
			return false
		}
		current = next
	}
	return false
}

func exprSymbolCandidates(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	secondary *bind.BindingTable,
) []cfg.SymbolID {
	set := newSymbolSet(3)
	addExprSymbolCandidates(set, expr, raw, primary, secondary)
	return set.Slice()
}
