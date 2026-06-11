package factflow

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// Return describes the ordered value sources returned at a CFG point.
type Return struct {
	sources []ValueSource
}

// NewReturn creates a return fact from ordered return-slot sources.
func NewReturn(sources []ValueSource) Return {
	return Return{sources: copyValueSources(sources)}
}

// Sources returns the ordered return-slot sources.
func (r Return) Sources() []ValueSource { return copyValueSources(r.sources) }

func (r Return) copy() Return {
	r.sources = copyValueSources(r.sources)
	return r
}

func copyReturnMap(in map[cfg.Point]Return) map[cfg.Point]Return {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point]Return, len(in))
	for point, fact := range in {
		out[point] = fact.copy()
	}
	return out
}
