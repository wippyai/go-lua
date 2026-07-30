package callboundary

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/lattice/factmap"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
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
func effectDeltaMap(reg *axis.Registry) factmap.Map[effectDeltaFactKey, EffectDelta, effectdelta.Value] {
	return factmap.Map[effectDeltaFactKey, EffectDelta, effectdelta.Value]{
		Key:       effectDeltaKeyOf,
		Value:     func(d EffectDelta) effectdelta.Value { return d.Value },
		WithValue: func(d EffectDelta, v effectdelta.Value) EffectDelta { d.Value = v; return d },
		Less:      effectDeltaLess,
		Valid:     func(d EffectDelta) bool { return d.Target.IsPlaceholder() && d.Site != "" && d.Kind != 0 },
		CloneFact: func(d EffectDelta) EffectDelta { d.Target = d.Target.Clone(); return d },
		Domain:    effectdelta.Domain(reg),
	}
}

func cloneEffectDeltas(in []EffectDelta) []EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]EffectDelta, len(in))
	for i, delta := range in {
		delta.Target = delta.Target.Clone()
		out[i] = delta
	}
	return out
}

func effectDeltaKeyOf(delta EffectDelta) effectDeltaFactKey {
	return effectDeltaFactKey{target: delta.Target.Key(), site: delta.Site, kind: delta.Kind}
}

func effectDeltaLess(a, b EffectDelta) bool {
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
