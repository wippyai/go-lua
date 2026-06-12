package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type effectDeltaKey struct {
	target pathdom.PathKey
	site   string
	kind   EffectDeltaKind
}

func normalizeEffectDeltas(reg *axis.Registry, in []EffectDelta) []EffectDelta {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[effectDeltaKey]EffectDelta, len(in))
	bottom := effectDeltaBottom(reg)
	for _, delta := range in {
		if !delta.Target.IsPlaceholder() || delta.Site == "" || delta.Kind == 0 {
			continue
		}
		delta.Target = cloneSummaryPath(delta.Target)
		if effectDeltaEqual(reg, delta, bottom) {
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

func cloneEffectDeltas(in []EffectDelta) []EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]EffectDelta, len(in))
	for i, delta := range in {
		delta.Target = cloneSummaryPath(delta.Target)
		out[i] = delta
	}
	return out
}

func effectDeltasEqual(reg *axis.Registry, a, b []EffectDelta) bool {
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

func effectDeltasLessOrEq(reg *axis.Registry, a, b []EffectDelta) bool {
	aMap := effectDeltasMap(reg, a)
	bMap := effectDeltasMap(reg, b)
	bottom := effectDeltaBottom(reg)
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

func joinEffectDeltas(reg *axis.Registry, a, b []EffectDelta) []EffectDelta {
	return combineEffectDeltaMaps(reg, effectDeltasMap(reg, a), effectDeltasMap(reg, b), joinEffectDelta)
}

func widenEffectDeltas(reg *axis.Registry, prev, next []EffectDelta) []EffectDelta {
	return combineEffectDeltaMaps(reg, effectDeltasMap(reg, prev), effectDeltasMap(reg, next), widenEffectDelta)
}

func effectDeltasMap(reg *axis.Registry, in []EffectDelta) map[effectDeltaKey]EffectDelta {
	out := normalizeEffectDeltas(reg, in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[effectDeltaKey]EffectDelta, len(out))
	for _, delta := range out {
		m[effectDeltaKeyOf(delta)] = delta
	}
	return m
}

func combineEffectDeltaMaps(
	reg *axis.Registry,
	a map[effectDeltaKey]EffectDelta,
	b map[effectDeltaKey]EffectDelta,
	combine func(*axis.Registry, EffectDelta, EffectDelta) EffectDelta,
) []EffectDelta {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[effectDeltaKey]EffectDelta, len(a)+len(b))
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

func effectDeltaBottom(reg *axis.Registry) EffectDelta {
	return EffectDelta{
		Before: product.Bottom(reg),
		After:  product.Bottom(reg),
		Change: EffectDeltaChangeBottom,
	}
}

func effectDeltaEqual(reg *axis.Registry, a, b EffectDelta) bool {
	return product.Equal(reg, a.Before, b.Before) &&
		product.Equal(reg, a.After, b.After) &&
		a.Change == b.Change
}

func effectDeltaLessOrEq(reg *axis.Registry, a, b EffectDelta) bool {
	return product.LessOrEq(reg, a.Before, b.Before) &&
		product.LessOrEq(reg, a.After, b.After) &&
		effectDeltaChangeLessOrEq(a.Change, b.Change)
}

func joinEffectDelta(reg *axis.Registry, a, b EffectDelta) EffectDelta {
	return EffectDelta{
		Target: a.Target,
		Site:   a.Site,
		Kind:   a.Kind,
		Before: product.Join(reg, a.Before, b.Before),
		After:  product.Join(reg, a.After, b.After),
		Change: effectDeltaChangeJoin(a.Change, b.Change),
	}
}

func widenEffectDelta(reg *axis.Registry, prev, next EffectDelta) EffectDelta {
	return EffectDelta{
		Target: prev.Target,
		Site:   prev.Site,
		Kind:   prev.Kind,
		Before: product.Widen(reg, prev.Before, next.Before),
		After:  product.Widen(reg, prev.After, next.After),
		Change: effectDeltaChangeJoin(prev.Change, next.Change),
	}
}

func effectDeltaChangeLessOrEq(a, b EffectDeltaChange) bool {
	return a == b || a == EffectDeltaChangeBottom || b == EffectDeltaChangeUnknown
}

func effectDeltaChangeJoin(a, b EffectDeltaChange) EffectDeltaChange {
	if a == b {
		return a
	}
	if a == EffectDeltaChangeBottom {
		return b
	}
	if b == EffectDeltaChangeBottom {
		return a
	}
	return EffectDeltaChangeUnknown
}

func effectDeltaKeyOf(delta EffectDelta) effectDeltaKey {
	return effectDeltaKey{target: delta.Target.Key(), site: delta.Site, kind: delta.Kind}
}

func sortedEffectDeltas(in map[effectDeltaKey]EffectDelta) []EffectDelta {
	if len(in) == 0 {
		return nil
	}
	out := make([]EffectDelta, 0, len(in))
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
