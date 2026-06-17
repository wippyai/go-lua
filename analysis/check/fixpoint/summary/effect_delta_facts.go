package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

type effectDeltaKey struct {
	target pathdom.PathKey
	site   effectdelta.Site
	kind   effectdelta.Kind
}

func normalizeEffectDeltas(reg *axis.Registry, in []callboundary.EffectDelta) []callboundary.EffectDelta {
	if len(in) == 0 {
		return nil
	}
	domain := effectdelta.Domain(reg)
	merged := make(map[effectDeltaKey]callboundary.EffectDelta, len(in))
	bottom := domain.Bottom()
	for _, delta := range in {
		if !delta.Target.IsPlaceholder() || delta.Site == "" || delta.Kind == 0 {
			continue
		}
		delta.Target = delta.Target.Clone()
		if domain.Equal(delta.Value, bottom) {
			continue
		}
		key := effectDeltaKeyOf(delta)
		if existing, ok := merged[key]; ok {
			merged[key] = joinEffectDelta(reg, existing, delta)
			continue
		}
		merged[key] = delta
	}
	return sortedEffectDeltas(merged)
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

func effectDeltasEqual(reg *axis.Registry, a, b []callboundary.EffectDelta) bool {
	a = normalizeEffectDeltas(reg, a)
	b = normalizeEffectDeltas(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if effectDeltaKeyOf(a[i]) != effectDeltaKeyOf(b[i]) || !effectDeltaEqual(reg, a[i], b[i]) {
			return false
		}
	}
	return true
}

func effectDeltasLessOrEq(reg *axis.Registry, a, b []callboundary.EffectDelta) bool {
	aMap := effectDeltasMap(reg, a)
	bMap := effectDeltasMap(reg, b)
	bottom := callboundary.EffectDelta{Value: effectdelta.Domain(reg).Bottom()}
	for key, av := range aMap {
		bv, ok := bMap[key]
		if !ok {
			bv = bottom
		}
		if !effectDeltaLessOrEq(reg, av, bv) {
			return false
		}
	}
	for key, bv := range bMap {
		if _, ok := aMap[key]; ok {
			continue
		}
		if !effectDeltaLessOrEq(reg, bottom, bv) {
			return false
		}
	}
	return true
}

func joinEffectDeltas(reg *axis.Registry, a, b []callboundary.EffectDelta) []callboundary.EffectDelta {
	return combineEffectDeltaMaps(reg, effectDeltasMap(reg, a), effectDeltasMap(reg, b), joinEffectDelta)
}

func widenEffectDeltas(reg *axis.Registry, prev, next []callboundary.EffectDelta) []callboundary.EffectDelta {
	return combineEffectDeltaMaps(reg, effectDeltasMap(reg, prev), effectDeltasMap(reg, next), widenEffectDelta)
}

func effectDeltasMap(reg *axis.Registry, in []callboundary.EffectDelta) map[effectDeltaKey]callboundary.EffectDelta {
	out := normalizeEffectDeltas(reg, in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[effectDeltaKey]callboundary.EffectDelta, len(out))
	for _, delta := range out {
		m[effectDeltaKeyOf(delta)] = delta
	}
	return m
}

func combineEffectDeltaMaps(
	reg *axis.Registry,
	a map[effectDeltaKey]callboundary.EffectDelta,
	b map[effectDeltaKey]callboundary.EffectDelta,
	combine func(*axis.Registry, callboundary.EffectDelta, callboundary.EffectDelta) callboundary.EffectDelta,
) []callboundary.EffectDelta {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[effectDeltaKey]callboundary.EffectDelta, len(a)+len(b))
	for key, left := range a {
		if right, ok := b[key]; ok {
			out[key] = combine(reg, left, right)
			continue
		}
		out[key] = left
	}
	for key, right := range b {
		if _, ok := a[key]; ok {
			continue
		}
		out[key] = right
	}
	return sortedEffectDeltas(out)
}

func effectDeltaEqual(reg *axis.Registry, a, b callboundary.EffectDelta) bool {
	return effectdelta.Domain(reg).Equal(a.Value, b.Value)
}

func effectDeltaLessOrEq(reg *axis.Registry, a, b callboundary.EffectDelta) bool {
	return effectdelta.Domain(reg).LessOrEq(a.Value, b.Value)
}

func joinEffectDelta(reg *axis.Registry, a, b callboundary.EffectDelta) callboundary.EffectDelta {
	return callboundary.EffectDelta{
		Target: a.Target,
		Site:   a.Site,
		Kind:   a.Kind,
		Value:  effectdelta.Domain(reg).Join(a.Value, b.Value),
	}
}

func widenEffectDelta(reg *axis.Registry, prev, next callboundary.EffectDelta) callboundary.EffectDelta {
	return callboundary.EffectDelta{
		Target: prev.Target,
		Site:   prev.Site,
		Kind:   prev.Kind,
		Value:  effectdelta.Domain(reg).Widen(prev.Value, next.Value),
	}
}

func effectDeltaKeyOf(delta callboundary.EffectDelta) effectDeltaKey {
	return effectDeltaKey{target: delta.Target.Key(), site: delta.Site, kind: delta.Kind}
}

func sortedEffectDeltas(in map[effectDeltaKey]callboundary.EffectDelta) []callboundary.EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.EffectDelta, 0, len(in))
	for _, delta := range in {
		out = append(out, delta)
	}
	sort.Slice(out, func(i, j int) bool {
		left := effectDeltaKeyOf(out[i])
		right := effectDeltaKeyOf(out[j])
		if left.target != right.target {
			return left.target < right.target
		}
		if left.site != right.site {
			return left.site < right.site
		}
		return left.kind < right.kind
	})
	return out
}
