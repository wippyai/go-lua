package flow

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/types/domain/value/product"
)

func compactBoundaryLengthLower(xs []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthLowerBound, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Lower <= 0 {
			continue
		}
		out = append(out, cloneBoundaryLengthLower(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthLower)
	return slices.CompactFunc(out, func(a, b BoundaryLengthLowerBound) bool {
		return compareBoundaryLengthLower(a, b) == 0
	})
}

func compactBoundaryLengthUpper(xs []BoundaryLengthUpperBound) []BoundaryLengthUpperBound {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthUpperBound, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Upper < 0 {
			continue
		}
		out = append(out, cloneBoundaryLengthUpper(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthUpper)
	return slices.CompactFunc(out, func(a, b BoundaryLengthUpperBound) bool {
		return compareBoundaryLengthUpper(a, b) == 0
	})
}

func compactBoundaryLengthRelations(xs []BoundaryLengthRelationFact) []BoundaryLengthRelationFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthRelationFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || !validBoundaryPath(fact.Source) {
			continue
		}
		out = append(out, cloneBoundaryLengthRelation(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthRelation)
	return slices.CompactFunc(out, func(a, b BoundaryLengthRelationFact) bool {
		return compareBoundaryLengthRelation(a, b) == 0
	})
}

func compactBoundaryStaticMembers(xs []BoundaryStaticMemberFact) []BoundaryStaticMemberFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryStaticMemberFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryStaticMember(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryStaticMember)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryStaticMember(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryStaticMemberFact(nil), dst...)
}

func cloneBoundaryLengthLower(f BoundaryLengthLowerBound) BoundaryLengthLowerBound {
	return BoundaryLengthLowerBound{
		Target: cloneBoundaryPath(f.Target),
		Lower:  f.Lower,
	}
}

func cloneBoundaryLengthUpper(f BoundaryLengthUpperBound) BoundaryLengthUpperBound {
	return BoundaryLengthUpperBound{
		Target: cloneBoundaryPath(f.Target),
		Upper:  f.Upper,
	}
}

func cloneBoundaryLengthRelation(f BoundaryLengthRelationFact) BoundaryLengthRelationFact {
	return BoundaryLengthRelationFact{
		Target: cloneBoundaryPath(f.Target),
		Source: cloneBoundaryPath(f.Source),
	}
}

func cloneBoundaryStaticMember(f BoundaryStaticMemberFact) BoundaryStaticMemberFact {
	return BoundaryStaticMemberFact{
		Target: cloneBoundaryPath(f.Target),
		Value:  f.Value,
	}
}

func compareBoundaryLengthLower(a, b BoundaryLengthLowerBound) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return cmp.Compare(a.Lower, b.Lower)
}

func compareBoundaryLengthUpper(a, b BoundaryLengthUpperBound) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return cmp.Compare(a.Upper, b.Upper)
}

func compareBoundaryLengthRelation(a, b BoundaryLengthRelationFact) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Source, b.Source)
}

func compareBoundaryStaticMember(a, b BoundaryStaticMemberFact) int {
	return compareBoundaryPath(a.Target, b.Target)
}

var (
	boundaryLengthLowerRowIdentity = orderedRowIdentity[BoundaryLengthLowerBound]{
		less: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) < 0 },
		same: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) == 0 },
	}
	boundaryLengthUpperRowIdentity = orderedRowIdentity[BoundaryLengthUpperBound]{
		less: func(a, b BoundaryLengthUpperBound) bool { return compareBoundaryLengthUpper(a, b) < 0 },
		same: func(a, b BoundaryLengthUpperBound) bool { return compareBoundaryLengthUpper(a, b) == 0 },
	}
	boundaryLengthRelationRowIdentity = orderedRowIdentity[BoundaryLengthRelationFact]{
		less: func(a, b BoundaryLengthRelationFact) bool { return compareBoundaryLengthRelation(a, b) < 0 },
		same: func(a, b BoundaryLengthRelationFact) bool { return compareBoundaryLengthRelation(a, b) == 0 },
	}
	boundaryStaticMemberRowIdentity = orderedRowIdentity[BoundaryStaticMemberFact]{
		less: func(a, b BoundaryStaticMemberFact) bool { return compareBoundaryStaticMember(a, b) < 0 },
		same: func(a, b BoundaryStaticMemberFact) bool { return compareBoundaryStaticMember(a, b) == 0 },
	}
)

func boundaryLengthFactsEqual(a, b BoundaryFacts) bool {
	return boundaryLengthLowerRowIdentity.Equal(a.lenLower, b.lenLower) &&
		boundaryLengthUpperRowIdentity.Equal(a.lenUpper, b.lenUpper) &&
		boundaryLengthRelationRowIdentity.Equal(a.lenRelations, b.lenRelations)
}

func boundaryLengthFactsLessOrEq(a, b BoundaryFacts) bool {
	return boundaryLengthLowerContainAll(a.lenLower, b.lenLower) &&
		boundaryLengthUpperContainAll(a.lenUpper, b.lenUpper) &&
		boundaryLengthRelationContainAll(a.lenRelations, b.lenRelations)
}

func boundaryLengthIntersectParts(a, b BoundaryFacts) BoundaryFactParts {
	return BoundaryFactParts{
		LengthLower:     intersectBoundaryLengthLower(a.lenLower, b.lenLower),
		LengthUpper:     intersectBoundaryLengthUpper(a.lenUpper, b.lenUpper),
		LengthRelations: intersectBoundaryLengthRelations(a.lenRelations, b.lenRelations),
	}
}

func boundaryStaticMemberFactsEqual(a, b BoundaryFacts) bool {
	return boundaryStaticMemberRowIdentity.EqualBy(a.staticMembers, b.staticMembers, func(x, y BoundaryStaticMemberFact) bool {
		return compareBoundaryStaticMember(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
	})
}

func boundaryStaticMemberFactsLessOrEq(a, b BoundaryFacts) bool {
	return boundaryStaticMembersContainAll(a.staticMembers, b.staticMembers)
}

func boundaryStaticMemberIntersectParts(a, b BoundaryFacts, widenPayload bool) BoundaryFactParts {
	return BoundaryFactParts{
		StaticMembers: intersectBoundaryStaticMembers(a.staticMembers, b.staticMembers, widenPayload),
	}
}

func boundaryLengthLowerContainAll(have, want []BoundaryLengthLowerBound) bool {
	return boundaryLengthLowerRowIdentity.ContainsAll(have, want)
}

func boundaryLengthUpperContainAll(have, want []BoundaryLengthUpperBound) bool {
	return boundaryLengthUpperRowIdentity.ContainsAll(have, want)
}

func boundaryLengthRelationContainAll(have, want []BoundaryLengthRelationFact) bool {
	return boundaryLengthRelationRowIdentity.ContainsAll(have, want)
}

func boundaryStaticMembersContainAll(have, want []BoundaryStaticMemberFact) bool {
	return boundaryStaticMemberRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryStaticMemberFact) bool {
		return compareBoundaryStaticMember(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func intersectBoundaryLengthLower(a, b []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	return boundaryLengthLowerRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthLowerBound) (BoundaryLengthLowerBound, bool) {
		return cloneBoundaryLengthLower(left), true
	})
}

func intersectBoundaryLengthUpper(a, b []BoundaryLengthUpperBound) []BoundaryLengthUpperBound {
	return boundaryLengthUpperRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthUpperBound) (BoundaryLengthUpperBound, bool) {
		return cloneBoundaryLengthUpper(left), true
	})
}

func intersectBoundaryLengthRelations(a, b []BoundaryLengthRelationFact) []BoundaryLengthRelationFact {
	return boundaryLengthRelationRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthRelationFact) (BoundaryLengthRelationFact, bool) {
		return cloneBoundaryLengthRelation(left), true
	})
}

func intersectBoundaryStaticMembers(a, b []BoundaryStaticMemberFact, widenPayload bool) []BoundaryStaticMemberFact {
	out := boundaryStaticMemberRowIdentity.MergeIntersect(a, b, func(left, right BoundaryStaticMemberFact) (BoundaryStaticMemberFact, bool) {
		fact := cloneBoundaryStaticMember(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryStaticMembers(out)
}
