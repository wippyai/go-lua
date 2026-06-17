package summary

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

type pathValueFactKey pathdom.PathKey

func normalizePathValueFacts(reg *axis.Registry, in []callboundary.PathValueFact) []callboundary.PathValueFact {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[pathValueFactKey]callboundary.PathValueFact, len(in))
	bottom := product.Bottom(reg)
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() || product.Equal(reg, fact.Value, bottom) {
			continue
		}
		fact.Path = fact.Path.Clone()
		key := pathValueFactKey(fact.Path.Key())
		if existing, ok := merged[key]; ok {
			existing.Value = product.Join(reg, existing.Value, fact.Value)
			merged[key] = existing
			continue
		}
		merged[key] = fact
	}
	return sortedPathValueFacts(merged)
}

func normalizePathStaticMemberFacts(reg *axis.Registry, in []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	merged := make(map[pathValueFactKey]callboundary.PathStaticMemberFact, len(in))
	for _, fact := range in {
		if !fact.Path.IsPlaceholder() {
			continue
		}
		fact.Path = fact.Path.Clone()
		key := pathValueFactKey(fact.Path.Key())
		if existing, ok := merged[key]; ok {
			existing.Value = product.Join(reg, existing.Value, fact.Value)
			merged[key] = existing
			continue
		}
		merged[key] = fact
	}
	return sortedPathStaticMemberFacts(merged)
}

func clonePathValueFacts(in []callboundary.PathValueFact) []callboundary.PathValueFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func clonePathStaticMemberFacts(in []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberFact, len(in))
	for i, fact := range in {
		fact.Path = fact.Path.Clone()
		out[i] = fact
	}
	return out
}

func pathValueFactsEqual(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	a = normalizePathValueFacts(reg, a)
	b = normalizePathValueFacts(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path.Key() != b[i].Path.Key() || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func pathStaticMemberFactsEqual(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	a = normalizePathStaticMemberFacts(reg, a)
	b = normalizePathStaticMemberFacts(reg, b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path.Key() != b[i].Path.Key() || !product.Equal(reg, a[i].Value, b[i].Value) {
			return false
		}
	}
	return true
}

func pathValueFactsLessOrEq(reg *axis.Registry, a, b []callboundary.PathValueFact) bool {
	aMap := pathValueFactsMap(reg, a)
	bMap := pathValueFactsMap(reg, b)
	bottom := product.Bottom(reg)
	for key, av := range aMap {
		bv, ok := bMap[key]
		if !ok {
			bv.Value = bottom
		}
		if !product.LessOrEq(reg, av.Value, bv.Value) {
			return false
		}
	}
	for key, bv := range bMap {
		if _, ok := aMap[key]; ok {
			continue
		}
		if !product.LessOrEq(reg, bottom, bv.Value) {
			return false
		}
	}
	return true
}

func pathStaticMemberFactsLessOrEq(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) bool {
	aMap := pathStaticMemberFactsMap(reg, a)
	bMap := pathStaticMemberFactsMap(reg, b)
	for key, bv := range bMap {
		av, ok := aMap[key]
		if !ok || !product.LessOrEq(reg, av.Value, bv.Value) {
			return false
		}
	}
	return true
}

func joinPathValueFacts(reg *axis.Registry, a, b []callboundary.PathValueFact) []callboundary.PathValueFact {
	return combinePathValueMaps(reg, pathValueFactsMap(reg, a), pathValueFactsMap(reg, b), product.Join)
}

func widenPathValueFacts(reg *axis.Registry, prev, next []callboundary.PathValueFact) []callboundary.PathValueFact {
	return combinePathValueMaps(reg, pathValueFactsMap(reg, prev), pathValueFactsMap(reg, next), product.Widen)
}

func joinPathStaticMemberFacts(reg *axis.Registry, a, b []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return combinePathStaticMemberMaps(reg, pathStaticMemberFactsMap(reg, a), pathStaticMemberFactsMap(reg, b), product.Join)
}

func widenPathStaticMemberFacts(reg *axis.Registry, prev, next []callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	return combinePathStaticMemberMaps(reg, pathStaticMemberFactsMap(reg, prev), pathStaticMemberFactsMap(reg, next), product.Widen)
}

func pathValueFactsMap(reg *axis.Registry, in []callboundary.PathValueFact) map[pathValueFactKey]callboundary.PathValueFact {
	out := normalizePathValueFacts(reg, in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[pathValueFactKey]callboundary.PathValueFact, len(out))
	for _, fact := range out {
		m[pathValueFactKey(fact.Path.Key())] = fact
	}
	return m
}

func pathStaticMemberFactsMap(reg *axis.Registry, in []callboundary.PathStaticMemberFact) map[pathValueFactKey]callboundary.PathStaticMemberFact {
	out := normalizePathStaticMemberFacts(reg, in)
	if len(out) == 0 {
		return nil
	}
	m := make(map[pathValueFactKey]callboundary.PathStaticMemberFact, len(out))
	for _, fact := range out {
		m[pathValueFactKey(fact.Path.Key())] = fact
	}
	return m
}

func combinePathValueMaps(
	reg *axis.Registry,
	a map[pathValueFactKey]callboundary.PathValueFact,
	b map[pathValueFactKey]callboundary.PathValueFact,
	combine func(*axis.Registry, product.Value, product.Value) product.Value,
) []callboundary.PathValueFact {
	keys := unionPathValueKeys(a, b)
	if len(keys) == 0 {
		return nil
	}
	bottom := product.Bottom(reg)
	out := make(map[pathValueFactKey]callboundary.PathValueFact, len(keys))
	for _, key := range keys {
		left, lok := a[key]
		right, rok := b[key]
		if !lok {
			left = right
			left.Value = bottom
		}
		if !rok {
			right = left
			right.Value = bottom
		}
		left.Value = combine(reg, left.Value, right.Value)
		if product.Equal(reg, left.Value, bottom) {
			continue
		}
		out[key] = left
	}
	return sortedPathValueFacts(out)
}

func combinePathStaticMemberMaps(
	reg *axis.Registry,
	a map[pathValueFactKey]callboundary.PathStaticMemberFact,
	b map[pathValueFactKey]callboundary.PathStaticMemberFact,
	combine func(*axis.Registry, product.Value, product.Value) product.Value,
) []callboundary.PathStaticMemberFact {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	out := make(map[pathValueFactKey]callboundary.PathStaticMemberFact)
	for key, left := range a {
		right, ok := b[key]
		if !ok {
			continue
		}
		left.Value = combine(reg, left.Value, right.Value)
		out[key] = left
	}
	return sortedPathStaticMemberFacts(out)
}

func sortedPathValueFacts(in map[pathValueFactKey]callboundary.PathValueFact) []callboundary.PathValueFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}

func sortedPathStaticMemberFacts(in map[pathValueFactKey]callboundary.PathStaticMemberFact) []callboundary.PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberFact, 0, len(in))
	for _, fact := range in {
		out = append(out, fact)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path.Less(out[j].Path) })
	return out
}

func unionPathValueKeys[V any](a map[pathValueFactKey]V, b map[pathValueFactKey]V) []pathValueFactKey {
	keys := make([]pathValueFactKey, 0, len(a)+len(b))
	seen := make(map[pathValueFactKey]struct{}, len(a)+len(b))
	for key := range a {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range b {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
