package cfg

import "github.com/wippyai/go-lua/compiler/ast"

// ParamSlot is the canonical parameter layout entry for a function graph.
//
// SourceIndex maps this slot back to fn.ParList. A value of -1 means this is
// an implicit parameter introduced by binder/CFG (for example implicit method
// receiver `self` on `function T:m(...)`).
type ParamSlot struct {
	Name           string
	Symbol         SymbolID
	DeclPoint      Point
	TypeAnnotation ast.TypeExpr
	SourceIndex    int
	IsImplicitSelf bool
}

// HasSourceParam reports whether this slot maps to a source parameter in fn.ParList.
func (s ParamSlot) HasSourceParam() bool {
	return s.SourceIndex >= 0
}

// SourceParamIndex returns the source parameter index when present.
func (s ParamSlot) SourceParamIndex() (int, bool) {
	if !s.HasSourceParam() {
		return 0, false
	}
	return s.SourceIndex, true
}

// ParamSlots returns the canonical parameter layout for this graph.
//
// All downstream phases should consume this API rather than re-deriving
// ParList-to-symbol mapping, which can drift for implicit method receivers.
func (g *Graph) ParamSlots() []ParamSlot {
	if g == nil || len(g.paramSymbols) == 0 {
		return nil
	}

	slots := make([]ParamSlot, 0, len(g.paramSymbols))

	var parNames []string
	var parTypes []ast.TypeExpr
	if g.fn != nil && g.fn.ParList != nil {
		parNames = g.fn.ParList.Names
		parTypes = g.fn.ParList.Types
	}

	hasImplicitSelf := len(g.paramSymbols) == len(parNames)+1 &&
		len(g.paramNames) == len(g.paramSymbols) &&
		len(g.paramNames) > 0 &&
		g.paramNames[0] == "self" &&
		(len(parNames) == 0 || parNames[0] != "self")

	for i, sym := range g.paramSymbols {
		slot := ParamSlot{
			Symbol:      sym,
			SourceIndex: i,
		}

		if i < len(g.paramNames) {
			slot.Name = g.paramNames[i]
		}
		if slot.Name == "" && sym != 0 {
			slot.Name = g.NameOf(sym)
		}
		if i < len(g.paramDeclPoints) {
			slot.DeclPoint = g.paramDeclPoints[i]
		}

		if hasImplicitSelf {
			if i == 0 {
				slot.SourceIndex = -1
				slot.IsImplicitSelf = true
			} else {
				slot.SourceIndex = i - 1
			}
		}

		if srcIdx, ok := slot.SourceParamIndex(); ok && srcIdx < len(parTypes) {
			slot.TypeAnnotation = parTypes[srcIdx]
		}

		slots = append(slots, slot)
	}

	return slots
}
