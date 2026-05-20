package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

// Parts is the per-symbol evidence used to publish canonical function facts.
type Parts struct {
	Params  []typ.Type
	Summary []typ.Type
	Type    typ.Type
}

// FromParts builds canonical function facts from per-symbol evidence.
func FromParts(parts map[cfg.SymbolID]Parts) api.FunctionFacts {
	if len(parts) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(parts))
	for _, sym := range cfg.SortedSymbolIDs(parts) {
		if sym == 0 {
			continue
		}
		part := parts[sym]
		ff := Join(api.FunctionFact{}, api.FunctionFact{
			Params:  part.Params,
			Summary: part.Summary,
			Type:    part.Type,
		})
		if Empty(ff) {
			continue
		}
		out[sym] = ff
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FromMaps builds canonical function facts from parallel evidence maps.
func FromMaps(
	params map[cfg.SymbolID][]typ.Type,
	summaries map[cfg.SymbolID][]typ.Type,
	types map[cfg.SymbolID]typ.Type,
) api.FunctionFacts {
	total := len(params) + len(summaries) + len(types)
	if total == 0 {
		return nil
	}
	parts := make(map[cfg.SymbolID]Parts, total)
	addParts(params, parts, func(part *Parts, v []typ.Type) { part.Params = v })
	addParts(summaries, parts, func(part *Parts, v []typ.Type) { part.Summary = v })
	for sym, t := range types {
		if sym == 0 {
			continue
		}
		part := parts[sym]
		part.Type = t
		parts[sym] = part
	}
	return FromParts(parts)
}

// FromSummaries builds canonical function facts from return summaries.
func FromSummaries(summaries map[cfg.SymbolID][]typ.Type) api.FunctionFacts {
	return FromSummariesExcept(summaries, 0)
}

// FromSummariesExcept builds canonical function facts from return summaries,
// excluding one symbol when exclude is nonzero.
func FromSummariesExcept(summaries map[cfg.SymbolID][]typ.Type, exclude cfg.SymbolID) api.FunctionFacts {
	if len(summaries) == 0 {
		return nil
	}
	parts := make(map[cfg.SymbolID]Parts, len(summaries))
	for sym, summary := range summaries {
		if sym == 0 || sym == exclude {
			continue
		}
		parts[sym] = Parts{Summary: summary}
	}
	return FromParts(parts)
}

func addParts(src map[cfg.SymbolID][]typ.Type, dst map[cfg.SymbolID]Parts, set func(*Parts, []typ.Type)) {
	for sym, value := range src {
		if sym == 0 {
			continue
		}
		part := dst[sym]
		set(&part, value)
		dst[sym] = part
	}
}
