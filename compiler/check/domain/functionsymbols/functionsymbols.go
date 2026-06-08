// Package functionsymbols owns normalized symbol sets that describe how
// function boundaries expose lexical state.
package functionsymbols

import (
	"slices"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// Set is a deterministic root-symbol set for function-boundary evidence.
type Set struct {
	symbols map[cfg.SymbolID]struct{}
}

// Add records sym when it is a valid root symbol.
func (s *Set) Add(sym cfg.SymbolID) {
	if sym == 0 {
		return
	}
	if s.symbols == nil {
		s.symbols = make(map[cfg.SymbolID]struct{})
	}
	s.symbols[sym] = struct{}{}
}

// Contains reports whether sym is present in the boundary set.
func (s Set) Contains(sym cfg.SymbolID) bool {
	if sym == 0 || s.symbols == nil {
		return false
	}
	_, ok := s.symbols[sym]
	return ok
}

// IsEmpty reports whether the set has no valid symbols.
func (s Set) IsEmpty() bool {
	return len(s.symbols) == 0
}

// Len returns the number of unique valid symbols in the set.
func (s Set) Len() int {
	return len(s.symbols)
}

// Slice returns the set in deterministic symbol order.
func (s Set) Slice() []cfg.SymbolID {
	if len(s.symbols) == 0 {
		return nil
	}
	out := make([]cfg.SymbolID, 0, len(s.symbols))
	for sym := range s.symbols {
		out = append(out, sym)
	}
	slices.Sort(out)
	return out
}

// CapturedFree returns the symbols fn captures from an enclosing function.
func CapturedFree(g *cfg.Graph, fn *ast.FunctionExpr) Set {
	var set Set
	if g == nil || g.Bindings() == nil || fn == nil {
		return set
	}
	for _, sym := range g.Bindings().CapturedSymbols(fn) {
		if g.IsFreeSymbol(sym) {
			set.Add(sym)
		}
	}
	return set
}

// Captured returns every symbol captured by fn according to bindings.
func Captured(bindings *bind.BindingTable, fn *ast.FunctionExpr) Set {
	var set Set
	if bindings == nil || fn == nil {
		return set
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		set.Add(sym)
	}
	return set
}

// Parameters returns the formal parameter root symbols owned by g.
func Parameters(g *cfg.Graph) Set {
	var set Set
	if g == nil {
		return set
	}
	for _, slot := range g.ParamSlotsReadOnly() {
		set.Add(slot.Symbol)
	}
	return set
}

// CurrentFunction returns symbols that name fn at its own function boundary.
func CurrentFunction(g *cfg.Graph, fn *ast.FunctionExpr) Set {
	var set Set
	if g == nil || fn == nil {
		return set
	}
	if bindings := g.Bindings(); bindings != nil {
		if sym, ok := bindings.FuncLitSymbol(fn); ok {
			set.Add(sym)
		}
	}
	for _, localFn := range g.LocalFunctionAssignments() {
		if localFn.Func == fn {
			set.Add(localFn.Symbol)
		}
	}
	for _, nested := range g.NestedFunctions() {
		if nested.Func != fn {
			continue
		}
		if funcDef := g.FuncDef(nested.Point); funcDef != nil {
			set.Add(funcDef.Symbol)
		}
		if assign := g.Assign(nested.Point); assign != nil && assign.IsLocal {
			if len(assign.Targets) == 1 && assign.Targets[0].Kind == cfg.TargetIdent {
				if len(assign.Sources) == 1 && assign.Sources[0] == nested.Func {
					set.Add(assign.Targets[0].Symbol)
				}
			}
		}
		set.Add(nested.Symbol)
	}
	return set
}

// NonGlobalCaptures returns captured symbols excluding globals.
func NonGlobalCaptures(bindings *bind.BindingTable, fn *ast.FunctionExpr) Set {
	var set Set
	if bindings == nil || fn == nil {
		return set
	}
	for _, sym := range bindings.CapturedSymbols(fn) {
		if sym == 0 {
			continue
		}
		kind, ok := bindings.Kind(sym)
		if ok && kind == cfg.SymbolGlobal {
			continue
		}
		set.Add(sym)
	}
	return set
}

// Returned returns root symbols directly present in return slots.
func Returned(g *cfg.Graph) Set {
	var set Set
	if g == nil || g.Bindings() == nil {
		return set
	}
	g.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		n := len(info.Exprs)
		if len(info.Symbols) > n {
			n = len(info.Symbols)
		}
		for i := 0; i < n; i++ {
			var sym cfg.SymbolID
			if i < len(info.Symbols) {
				sym = info.Symbols[i]
			}
			if sym == 0 && i < len(info.Exprs) {
				if ident, ok := info.Exprs[i].(*ast.IdentExpr); ok {
					sym, _ = g.Bindings().SymbolOf(ident)
				}
			}
			set.Add(sym)
		}
	})
	return set
}

// OwnedCapturedByNested returns symbols owned by g and captured by any nested
// function in g.
func OwnedCapturedByNested(g *cfg.Graph) Set {
	var set Set
	if g == nil || g.Bindings() == nil {
		return set
	}
	for _, nested := range g.NestedFunctions() {
		if nested.Func == nil {
			continue
		}
		for _, sym := range g.Bindings().CapturedSymbols(nested.Func) {
			if g.OwnsSymbol(sym) {
				set.Add(sym)
			}
		}
	}
	return set
}
