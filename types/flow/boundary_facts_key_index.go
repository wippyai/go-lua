package flow

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func compactBoundaryKeyPresence(xs []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyPresenceFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Table) || !validBoundaryPath(fact.Key) {
			continue
		}
		out = append(out, cloneBoundaryKeyPresence(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyPresence)
	return slices.CompactFunc(out, func(a, b BoundaryKeyPresenceFact) bool {
		return compareBoundaryKeyPresence(a, b) == 0
	})
}

func compactBoundaryKeyArrays(xs []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryKeyArray(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArray)
	return slices.CompactFunc(out, func(a, b BoundaryKeyArrayFact) bool {
		return compareBoundaryKeyArray(a, b) == 0
	})
}

func compactBoundaryKeyArrayValues(xs []BoundaryKeyArrayValueFact) []BoundaryKeyArrayValueFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayValueFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryKeyArrayValue(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArrayValue)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryKeyArrayValue(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryKeyArrayValueFact(nil), dst...)
}

func compactBoundaryIndexWrites(xs []BoundaryIndexWriteFact) []BoundaryIndexWriteFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryIndexWriteFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryIndexWrite(fact) {
			continue
		}
		out = append(out, cloneBoundaryIndexWrite(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryIndexWrite)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryIndexWrite(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].KeyValue = product.Domain.Join(dst[len(dst)-1].KeyValue, fact.KeyValue)
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryIndexWriteFact(nil), dst...)
}

func cloneBoundaryKeyPresence(f BoundaryKeyPresenceFact) BoundaryKeyPresenceFact {
	return BoundaryKeyPresenceFact{
		Table: cloneBoundaryPath(f.Table),
		Key:   cloneBoundaryPath(f.Key),
	}
}

func cloneBoundaryKeyArray(f BoundaryKeyArrayFact) BoundaryKeyArrayFact {
	return BoundaryKeyArrayFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
	}
}

func cloneBoundaryKeyArrayValue(f BoundaryKeyArrayValueFact) BoundaryKeyArrayValueFact {
	return BoundaryKeyArrayValueFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryIndexWrite(f BoundaryIndexWriteFact) BoundaryIndexWriteFact {
	return BoundaryIndexWriteFact{
		Table:        cloneBoundaryPath(f.Table),
		KeyPath:      cloneBoundaryPath(f.KeyPath),
		HasKeyPath:   f.HasKeyPath,
		KeyValue:     f.KeyValue,
		ValuePath:    cloneBoundaryPath(f.ValuePath),
		HasValuePath: f.HasValuePath,
		Value:        f.Value,
	}
}

func compareBoundaryKeyPresence(a, b BoundaryKeyPresenceFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryKeyArray(a, b BoundaryKeyArrayFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryKeyArrayValue(a, b BoundaryKeyArrayValueFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryIndexWrite(a, b BoundaryIndexWriteFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	if c := compareBoundaryIndexKey(a, b); c != 0 {
		return c
	}
	if c := compareBoundaryBool(a.HasValuePath, b.HasValuePath); c != 0 {
		return c
	}
	if a.HasValuePath {
		return compareBoundaryPath(a.ValuePath, b.ValuePath)
	}
	return 0
}

func compareBoundaryIndexKey(a, b BoundaryIndexWriteFact) int {
	if c := compareBoundaryBool(a.HasKeyPath, b.HasKeyPath); c != 0 {
		return c
	}
	if a.HasKeyPath {
		return compareBoundaryPath(a.KeyPath, b.KeyPath)
	}
	if c := cmp.Compare(a.KeyValue.Hash(), b.KeyValue.Hash()); c != 0 {
		return c
	}
	if product.Domain.Equal(a.KeyValue, b.KeyValue) {
		return 0
	}
	return cmp.Compare(product.ProjectValueOrUnknown(a.KeyValue).String(), product.ProjectValueOrUnknown(b.KeyValue).String())
}

func hashBoundaryIndexKey(h uint64, fact BoundaryIndexWriteFact) uint64 {
	if fact.HasKeyPath {
		h = internal.HashCombine(h, 1)
		return hashBoundaryPath(h, fact.KeyPath)
	}
	h = internal.HashCombine(h, 0)
	return internal.HashCombine(h, fact.KeyValue.Hash())
}

func validBoundaryIndexWrite(fact BoundaryIndexWriteFact) bool {
	if !validBoundaryPath(fact.Table) || fact.KeyValue.IsZero() || fact.Value.IsZero() {
		return false
	}
	if fact.HasKeyPath && !validBoundaryPath(fact.KeyPath) {
		return false
	}
	if fact.HasValuePath && !validBoundaryPath(fact.ValuePath) {
		return false
	}
	return true
}

var (
	boundaryKeyPresenceRowIdentity = orderedRowIdentity[BoundaryKeyPresenceFact]{
		less: func(a, b BoundaryKeyPresenceFact) bool { return compareBoundaryKeyPresence(a, b) < 0 },
		same: func(a, b BoundaryKeyPresenceFact) bool {
			return compareBoundaryKeyPresence(a, b) == 0
		},
	}
	boundaryKeyArrayRowIdentity = orderedRowIdentity[BoundaryKeyArrayFact]{
		less: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) == 0 },
	}
	boundaryKeyArrayValueRowIdentity = orderedRowIdentity[BoundaryKeyArrayValueFact]{
		less: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) == 0 },
	}
	boundaryIndexWriteRowIdentity = orderedRowIdentity[BoundaryIndexWriteFact]{
		less: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) < 0 },
		same: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) == 0 },
	}
)

func boundaryKeyIndexFactsEqual(a, b BoundaryFacts) bool {
	return boundaryKeyPresenceRowIdentity.Equal(a.keyPresence, b.keyPresence) &&
		boundaryKeyArrayRowIdentity.Equal(a.keyArrays, b.keyArrays) &&
		boundaryKeyArrayValueRowIdentity.EqualBy(a.keyArrayValues, b.keyArrayValues, func(x, y BoundaryKeyArrayValueFact) bool {
			return compareBoundaryKeyArrayValue(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
		}) &&
		boundaryIndexWriteRowIdentity.EqualBy(a.indexWrites, b.indexWrites, func(x, y BoundaryIndexWriteFact) bool {
			return compareBoundaryIndexWrite(x, y) == 0 &&
				product.Domain.Equal(x.KeyValue, y.KeyValue) &&
				product.Domain.Equal(x.Value, y.Value)
		})
}

func boundaryKeyIndexFactsLessOrEq(a, b BoundaryFacts) bool {
	return boundaryKeyPresenceContainAll(a.keyPresence, b.keyPresence) &&
		boundaryKeyArraysContainAll(a.keyArrays, b.keyArrays) &&
		boundaryKeyArrayValuesContainAll(a.keyArrayValues, b.keyArrayValues) &&
		boundaryIndexWritesContainAll(a.indexWrites, b.indexWrites)
}

func boundaryKeyIndexIntersectParts(a, b BoundaryFacts, widenPayload bool) BoundaryFactParts {
	return BoundaryFactParts{
		KeyPresence:    intersectBoundaryKeyPresence(a.keyPresence, b.keyPresence),
		KeyArrays:      intersectBoundaryKeyArrays(a.keyArrays, b.keyArrays),
		KeyArrayValues: intersectBoundaryKeyArrayValues(a.keyArrayValues, b.keyArrayValues, widenPayload),
		IndexWrites:    intersectBoundaryIndexWrites(a.indexWrites, b.indexWrites, widenPayload),
	}
}

func boundaryKeyPresenceContainAll(have, want []BoundaryKeyPresenceFact) bool {
	return boundaryKeyPresenceRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArraysContainAll(have, want []BoundaryKeyArrayFact) bool {
	return boundaryKeyArrayRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArrayValuesContainAll(have, want []BoundaryKeyArrayValueFact) bool {
	return boundaryKeyArrayValueRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryKeyArrayValueFact) bool {
		return compareBoundaryKeyArrayValue(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryIndexWritesContainAll(have, want []BoundaryIndexWriteFact) bool {
	return boundaryIndexWriteRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryIndexWriteFact) bool {
		return compareBoundaryIndexWrite(have, want) == 0 &&
			product.Domain.LessOrEq(have.KeyValue, want.KeyValue) &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func intersectBoundaryKeyPresence(a, b []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	return boundaryKeyPresenceRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyPresenceFact) (BoundaryKeyPresenceFact, bool) {
		return cloneBoundaryKeyPresence(left), true
	})
}

func intersectBoundaryKeyArrays(a, b []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	return boundaryKeyArrayRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyArrayFact) (BoundaryKeyArrayFact, bool) {
		return cloneBoundaryKeyArray(left), true
	})
}

func intersectBoundaryKeyArrayValues(a, b []BoundaryKeyArrayValueFact, widenPayload bool) []BoundaryKeyArrayValueFact {
	out := boundaryKeyArrayValueRowIdentity.MergeIntersect(a, b, func(left, right BoundaryKeyArrayValueFact) (BoundaryKeyArrayValueFact, bool) {
		fact := cloneBoundaryKeyArrayValue(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryKeyArrayValues(out)
}

func intersectBoundaryIndexWrites(a, b []BoundaryIndexWriteFact, widenPayload bool) []BoundaryIndexWriteFact {
	out := boundaryIndexWriteRowIdentity.MergeIntersect(a, b, func(left, right BoundaryIndexWriteFact) (BoundaryIndexWriteFact, bool) {
		fact := cloneBoundaryIndexWrite(left)
		if widenPayload {
			fact.KeyValue = product.Domain.Widen(fact.KeyValue, right.KeyValue)
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.KeyValue = product.Domain.Join(fact.KeyValue, right.KeyValue)
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryIndexWrites(out)
}
