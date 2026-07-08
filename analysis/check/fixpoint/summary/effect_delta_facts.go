package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

type effectDeltaFactKey struct {
	target pathdom.PathKey
	site   effectdelta.Site
	kind   effectdelta.Kind
}

// effectDeltaMap is the canonical pointwise map lattice for effect deltas: one
// delta per (target, site, kind) carrying an effect-delta value merged through
// the value domain.
func effectDeltaMap(reg *axis.Registry) factmap.Map[effectDeltaFactKey, callboundary.EffectDelta, effectdelta.Value] {
	return factmap.Map[effectDeltaFactKey, callboundary.EffectDelta, effectdelta.Value]{
		Key:       effectDeltaKeyOf,
		Value:     func(d callboundary.EffectDelta) effectdelta.Value { return d.Value },
		WithValue: func(d callboundary.EffectDelta, v effectdelta.Value) callboundary.EffectDelta { d.Value = v; return d },
		Less:      effectDeltaLess,
		Valid:     func(d callboundary.EffectDelta) bool { return d.Target.IsPlaceholder() && d.Site != "" && d.Kind != 0 },
		CloneFact: func(d callboundary.EffectDelta) callboundary.EffectDelta { d.Target = d.Target.Clone(); return d },
		Domain:    effectdelta.Domain(reg),
	}
}

func cloneEffectDeltas(in []callboundary.EffectDelta) []callboundary.EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.EffectDelta, len(in))
	for i, delta := range in {
		delta.Target = delta.Target.Clone()
		out[i] = delta
	}
	return out
}

func effectDeltaEqual(reg *axis.Registry, a, b callboundary.EffectDelta) bool {
	return effectdelta.Domain(reg).Equal(a.Value, b.Value)
}

func effectDeltaKeyOf(delta callboundary.EffectDelta) effectDeltaFactKey {
	return effectDeltaFactKey{target: delta.Target.Key(), site: delta.Site, kind: delta.Kind}
}

func effectDeltaLess(a, b callboundary.EffectDelta) bool {
	left := effectDeltaKeyOf(a)
	right := effectDeltaKeyOf(b)
	if left.target != right.target {
		return left.target < right.target
	}
	if left.site != right.site {
		return left.site < right.site
	}
	return left.kind < right.kind
}
