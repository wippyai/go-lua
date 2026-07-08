package cfgfacts

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Metadata stores Lua sidecar facts keyed by CFG point.
type Metadata struct {
	genericFors map[cfg.Point]GenericForFact
}

func (m Metadata) GenericFor(point cfg.Point) (GenericForFact, bool) {
	fact, ok := m.genericFors[point]
	if !ok {
		return GenericForFact{}, false
	}
	return copyGenericForFact(fact), true
}

func (m *Metadata) SetGenericFor(point cfg.Point, fact GenericForFact) {
	if m.genericFors == nil {
		m.genericFors = make(map[cfg.Point]GenericForFact)
	}
	m.genericFors[point] = copyGenericForFact(fact)
}
