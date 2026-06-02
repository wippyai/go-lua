package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
)

type symbolStorageClass uint8

const (
	symbolStorageEnv symbolStorageClass = iota + 1
	symbolStorageOwnerCell
	symbolStorageCapturedCell
)

func (c symbolStorageClass) usesCells() bool {
	return c == symbolStorageOwnerCell || c == symbolStorageCapturedCell
}

func (c symbolStorageClass) emitsCellEffects() bool {
	return c == symbolStorageCapturedCell
}

// symbolStoragePolicy is the compiler-aware boundary between lexical symbols
// and PointState storage axes.
//
// types/flow owns primitive Env/Cells reads and writes, but it intentionally
// does not know whether a symbol is a local value, an owner cell captured by a
// nested closure, or a free captured cell from an enclosing scope. That lexical
// policy lives here, then delegates mechanics to PointFacts/PointWriter.
type symbolStoragePolicy struct {
	graph      *cfg.Graph
	params     map[cfg.SymbolID]int
	ownerCells map[cfg.SymbolID]struct{}
}

func newSymbolStoragePolicy(g *cfg.Graph, params map[cfg.SymbolID]int, ownerCells []cfg.SymbolID) symbolStoragePolicy {
	p := symbolStoragePolicy{
		graph:  g,
		params: params,
	}
	if len(ownerCells) != 0 {
		p.ownerCells = make(map[cfg.SymbolID]struct{}, len(ownerCells))
		for _, sym := range ownerCells {
			if sym != 0 {
				p.ownerCells[sym] = struct{}{}
			}
		}
	}
	return p
}

func (p symbolStoragePolicy) class(sym cfg.SymbolID) symbolStorageClass {
	if sym == 0 {
		return symbolStorageEnv
	}
	if _, ok := p.ownerCells[sym]; ok {
		return symbolStorageOwnerCell
	}
	if p.isCapturedFreeVar(sym) {
		return symbolStorageCapturedCell
	}
	return symbolStorageEnv
}

func (p symbolStoragePolicy) isCellBacked(sym cfg.SymbolID) bool {
	return p.class(sym).usesCells()
}

func (p symbolStoragePolicy) read(out *flow.PointState, sym cfg.SymbolID) (product.AbstractValue, bool) {
	if out == nil || sym == 0 {
		return product.AbstractValue{}, false
	}
	facts := flow.PointFactsOf(*out)
	switch p.class(sym) {
	case symbolStorageOwnerCell, symbolStorageCapturedCell:
		if av, ok := facts.CellValue(sym); ok && !valueIsBottom(av) {
			return av, true
		}
		av, ok := facts.EnvValue(flow.SymbolValueKey(sym))
		if !ok || valueIsBottom(av) {
			return product.AbstractValue{}, false
		}
		return av, true
	default:
		av, ok := facts.EnvValue(flow.SymbolValueKey(sym))
		if !ok || valueIsBottom(av) {
			return product.AbstractValue{}, false
		}
		return av, true
	}
}

func (p symbolStoragePolicy) write(
	out *flow.PointState,
	sym cfg.SymbolID,
	val product.AbstractValue,
	joinExisting bool,
	emitEffect bool,
) {
	if out == nil || sym == 0 {
		return
	}
	class := p.class(sym)
	flow.NewPointWriter(out).WriteSymbolValue(
		sym,
		val,
		class.usesCells(),
		joinExisting,
		emitEffect && class.emitsCellEffects(),
	)
}

func (p symbolStoragePolicy) hasCellEffectTarget(out *flow.PointState, sym cfg.SymbolID) bool {
	if out == nil || sym == 0 {
		return false
	}
	if _, ok := out.Cells.Value(sym); ok {
		return true
	}
	return p.class(sym) == symbolStorageOwnerCell
}

func (p symbolStoragePolicy) clear(out *flow.PointState, sym cfg.SymbolID, joinExisting bool, emitEffect bool) bool {
	if out == nil || sym == 0 {
		return false
	}
	class := p.class(sym)
	if class.usesCells() {
		p.write(out, sym, product.Domain.Top(), joinExisting, emitEffect)
		return true
	}
	return flow.NewPointWriter(out).DeleteValueKey(flow.SymbolValueKey(sym))
}

func (p symbolStoragePolicy) isCapturedFreeVar(sym cfg.SymbolID) bool {
	if sym == 0 || p.graph == nil {
		return false
	}
	if _, isParam := p.params[sym]; isParam {
		return false
	}
	if k, ok := p.graph.SymbolKind(sym); ok {
		switch k {
		case cfg.SymbolLocal, cfg.SymbolParam:
			return false
		}
	}
	return true
}
