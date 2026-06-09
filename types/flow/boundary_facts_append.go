package flow

import (
	"slices"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

func compactBoundaryAppendKeys(xs []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendKeyFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) {
			continue
		}
		if fact.HasTable && !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryAppendKey(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendKey)
	return slices.CompactFunc(out, func(a, b BoundaryAppendKeyFact) bool {
		return compareBoundaryAppendKey(a, b) == 0
	})
}

func compactBoundaryAppendHistoryBases(xs []BoundaryAppendHistoryBaseFact) []BoundaryAppendHistoryBaseFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryBaseFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryBase(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryBase)
	return slices.CompactFunc(out, func(a, b BoundaryAppendHistoryBaseFact) bool {
		return compareBoundaryAppendHistoryBase(a, b) == 0
	})
}

func compactBoundaryAppendHistoryEvents(xs []BoundaryAppendHistoryEventFact) []BoundaryAppendHistoryEventFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryEventFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryEvent(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryEvent)
	return slices.CompactFunc(out, func(a, b BoundaryAppendHistoryEventFact) bool {
		return compareBoundaryAppendHistoryEvent(a, b) == 0
	})
}

func compactBoundaryAppendHistoryCoverage(xs []BoundaryAppendHistoryCoverageFact) []BoundaryAppendHistoryCoverageFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryCoverageFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryCoverage(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryCoverage)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryAppendHistoryCoverage(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryAppendHistoryCoverageFact(nil), dst...)
}

func compactBoundaryAppendHistoryTableCoverage(xs []BoundaryAppendHistoryTableCoverageFact) []BoundaryAppendHistoryTableCoverageFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendHistoryTableCoverageFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryTableCoverage(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendHistoryTableCoverage)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryAppendHistoryTableCoverage(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryAppendHistoryTableCoverageFact(nil), dst...)
}

func compactBoundaryAppendElementFieldOrigins(xs []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendElementFieldOriginFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || len(fact.Field) == 0 || !validBoundaryPath(fact.Source) {
			continue
		}
		out = append(out, cloneBoundaryAppendElementFieldOrigin(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendElementFieldOrigin)
	return slices.CompactFunc(out, func(a, b BoundaryAppendElementFieldOriginFact) bool {
		return compareBoundaryAppendElementFieldOrigin(a, b) == 0
	})
}

func cloneBoundaryAppendKey(f BoundaryAppendKeyFact) BoundaryAppendKeyFact {
	return BoundaryAppendKeyFact{
		Array:    cloneBoundaryPath(f.Array),
		Key:      cloneBoundaryPath(f.Key),
		Table:    cloneBoundaryPath(f.Table),
		HasTable: f.HasTable,
	}
}

func cloneBoundaryAppendHistoryBase(f BoundaryAppendHistoryBaseFact) BoundaryAppendHistoryBaseFact {
	return BoundaryAppendHistoryBaseFact{Array: cloneBoundaryPath(f.Array)}
}

func cloneBoundaryAppendHistoryEvent(f BoundaryAppendHistoryEventFact) BoundaryAppendHistoryEventFact {
	return BoundaryAppendHistoryEventFact{
		Array: cloneBoundaryPath(f.Array),
		Key:   cloneBoundaryPath(f.Key),
	}
}

func cloneBoundaryAppendHistoryCoverage(f BoundaryAppendHistoryCoverageFact) BoundaryAppendHistoryCoverageFact {
	return BoundaryAppendHistoryCoverageFact{
		Array: cloneBoundaryPath(f.Array),
		Key:   cloneBoundaryPath(f.Key),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendHistoryTableCoverage(f BoundaryAppendHistoryTableCoverageFact) BoundaryAppendHistoryTableCoverageFact {
	return BoundaryAppendHistoryTableCoverageFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendElementFieldOrigin(f BoundaryAppendElementFieldOriginFact) BoundaryAppendElementFieldOriginFact {
	return BoundaryAppendElementFieldOriginFact{
		Array:       cloneBoundaryPath(f.Array),
		Field:       append([]constraint.Segment(nil), f.Field...),
		Source:      cloneBoundaryPath(f.Source),
		SourceField: append([]constraint.Segment(nil), f.SourceField...),
	}
}

func compareBoundaryAppendKey(a, b BoundaryAppendKeyFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Key, b.Key); c != 0 {
		return c
	}
	if a.HasTable != b.HasTable {
		if !a.HasTable {
			return -1
		}
		return 1
	}
	if !a.HasTable {
		return 0
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendHistoryBase(a, b BoundaryAppendHistoryBaseFact) int {
	return compareBoundaryPath(a.Array, b.Array)
}

func compareBoundaryAppendHistoryEvent(a, b BoundaryAppendHistoryEventFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryAppendHistoryCoverage(a, b BoundaryAppendHistoryCoverageFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Key, b.Key); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendHistoryTableCoverage(a, b BoundaryAppendHistoryTableCoverageFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendElementFieldOrigin(a, b BoundaryAppendElementFieldOriginFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareConstraintSegments(a.Field, b.Field); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Source, b.Source); c != 0 {
		return c
	}
	return compareConstraintSegments(a.SourceField, b.SourceField)
}

var (
	boundaryAppendKeyRowIdentity = orderedRowIdentity[BoundaryAppendKeyFact]{
		less: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) < 0 },
		same: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) == 0 },
	}
	boundaryAppendHistoryBaseRowIdentity = orderedRowIdentity[BoundaryAppendHistoryBaseFact]{
		less: func(a, b BoundaryAppendHistoryBaseFact) bool {
			return compareBoundaryAppendHistoryBase(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryBaseFact) bool {
			return compareBoundaryAppendHistoryBase(a, b) == 0
		},
	}
	boundaryAppendHistoryEventRowIdentity = orderedRowIdentity[BoundaryAppendHistoryEventFact]{
		less: func(a, b BoundaryAppendHistoryEventFact) bool {
			return compareBoundaryAppendHistoryEvent(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryEventFact) bool {
			return compareBoundaryAppendHistoryEvent(a, b) == 0
		},
	}
	boundaryAppendHistoryCoverageRowIdentity = orderedRowIdentity[BoundaryAppendHistoryCoverageFact]{
		less: func(a, b BoundaryAppendHistoryCoverageFact) bool {
			return compareBoundaryAppendHistoryCoverage(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryCoverageFact) bool {
			return compareBoundaryAppendHistoryCoverage(a, b) == 0
		},
	}
	boundaryAppendHistoryTableCoverageRowIdentity = orderedRowIdentity[BoundaryAppendHistoryTableCoverageFact]{
		less: func(a, b BoundaryAppendHistoryTableCoverageFact) bool {
			return compareBoundaryAppendHistoryTableCoverage(a, b) < 0
		},
		same: func(a, b BoundaryAppendHistoryTableCoverageFact) bool {
			return compareBoundaryAppendHistoryTableCoverage(a, b) == 0
		},
	}
	boundaryAppendElementFieldOriginRowIdentity = orderedRowIdentity[BoundaryAppendElementFieldOriginFact]{
		less: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) < 0
		},
		same: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) == 0
		},
	}
)

func boundaryAppendFactsEqual(a, b BoundaryFacts) bool {
	return boundaryAppendKeyRowIdentity.Equal(a.appendKeys, b.appendKeys) &&
		boundaryAppendHistoryBaseRowIdentity.Equal(a.appendBases, b.appendBases) &&
		boundaryAppendHistoryEventRowIdentity.Equal(a.appendEvents, b.appendEvents) &&
		boundaryAppendHistoryCoverageRowIdentity.EqualBy(a.appendCoverage, b.appendCoverage, func(x, y BoundaryAppendHistoryCoverageFact) bool {
			return compareBoundaryAppendHistoryCoverage(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
		}) &&
		boundaryAppendHistoryTableCoverageRowIdentity.EqualBy(a.appendTableCoverage, b.appendTableCoverage, func(x, y BoundaryAppendHistoryTableCoverageFact) bool {
			return compareBoundaryAppendHistoryTableCoverage(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
		}) &&
		boundaryAppendElementFieldOriginRowIdentity.Equal(a.appendOrigins, b.appendOrigins)
}

func boundaryAppendFactsLessOrEq(a, b BoundaryFacts) bool {
	return boundaryAppendKeysContainAll(a.appendKeys, b.appendKeys) &&
		boundaryAppendHistoryBasesContainAll(a.appendBases, b.appendBases) &&
		boundaryAppendHistoryEventsContainAll(a.appendEvents, b.appendEvents) &&
		boundaryAppendHistoryCoverageContainAll(a.appendCoverage, b.appendCoverage) &&
		boundaryAppendHistoryTableCoverageContainAll(a.appendTableCoverage, b.appendTableCoverage) &&
		boundaryAppendElementFieldOriginsContainAll(a.appendOrigins, b.appendOrigins)
}

func boundaryAppendIntersectParts(a, b BoundaryFacts, widenPayload bool) BoundaryFactParts {
	appendBases := intersectBoundaryAppendHistoryBases(a.appendBases, b.appendBases)
	appendEvents := intersectBoundaryAppendHistoryEventsWithBases(a.appendEvents, b.appendEvents, appendBases)
	return BoundaryFactParts{
		AppendKeys:          intersectBoundaryAppendKeys(a.appendKeys, b.appendKeys),
		AppendBases:         appendBases,
		AppendEvents:        appendEvents,
		AppendCoverage:      intersectBoundaryAppendHistoryCoverageWithBases(a.appendCoverage, b.appendCoverage, appendBases, appendEvents, widenPayload),
		AppendTableCoverage: intersectBoundaryAppendHistoryTableCoverageWithBases(a.appendTableCoverage, b.appendTableCoverage, appendBases, widenPayload),
		AppendOrigins:       intersectBoundaryAppendElementFieldOriginsWithBases(a.appendOrigins, b.appendOrigins, appendBases),
	}
}

func boundaryAppendKeysContainAll(have, want []BoundaryAppendKeyFact) bool {
	return boundaryAppendKeyRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryBasesContainAll(have, want []BoundaryAppendHistoryBaseFact) bool {
	return boundaryAppendHistoryBaseRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryEventsContainAll(have, want []BoundaryAppendHistoryEventFact) bool {
	return boundaryAppendHistoryEventRowIdentity.ContainsAll(have, want)
}

func boundaryAppendHistoryCoverageContainAll(have, want []BoundaryAppendHistoryCoverageFact) bool {
	return boundaryAppendHistoryCoverageRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryAppendHistoryCoverageFact) bool {
		return compareBoundaryAppendHistoryCoverage(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendHistoryTableCoverageContainAll(have, want []BoundaryAppendHistoryTableCoverageFact) bool {
	return boundaryAppendHistoryTableCoverageRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryAppendHistoryTableCoverageFact) bool {
		return compareBoundaryAppendHistoryTableCoverage(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendElementFieldOriginsContainAll(have, want []BoundaryAppendElementFieldOriginFact) bool {
	return boundaryAppendElementFieldOriginRowIdentity.ContainsAll(have, want)
}

func intersectBoundaryAppendKeys(a, b []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	return boundaryAppendKeyRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendKeyFact) (BoundaryAppendKeyFact, bool) {
		return cloneBoundaryAppendKey(left), true
	})
}

func intersectBoundaryAppendHistoryBases(a, b []BoundaryAppendHistoryBaseFact) []BoundaryAppendHistoryBaseFact {
	return boundaryAppendHistoryBaseRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendHistoryBaseFact) (BoundaryAppendHistoryBaseFact, bool) {
		return cloneBoundaryAppendHistoryBase(left), true
	})
}

func intersectBoundaryAppendHistoryEventsWithBases(
	a, b []BoundaryAppendHistoryEventFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryEventFact {
	out := intersectBoundaryAppendHistoryEvents(a, b)
	out = append(out, boundaryAppendHistoryEventsCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendHistoryEventsCoveredByBases(b, bases)...)
	return compactBoundaryAppendHistoryEvents(out)
}

func intersectBoundaryAppendHistoryEvents(a, b []BoundaryAppendHistoryEventFact) []BoundaryAppendHistoryEventFact {
	return boundaryAppendHistoryEventRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendHistoryEventFact) (BoundaryAppendHistoryEventFact, bool) {
		return cloneBoundaryAppendHistoryEvent(left), true
	})
}

func boundaryAppendHistoryEventsCoveredByBases(
	events []BoundaryAppendHistoryEventFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryEventFact {
	if len(events) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryEventFact
	for _, event := range events {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: event.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryEvent(event))
	}
	return out
}

func intersectBoundaryAppendHistoryCoverageWithBases(
	a, b []BoundaryAppendHistoryCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	events []BoundaryAppendHistoryEventFact,
	widenPayload bool,
) []BoundaryAppendHistoryCoverageFact {
	out := intersectBoundaryAppendHistoryCoverage(a, b, widenPayload)
	out = append(out, boundaryAppendHistoryCoverageCoveredByBases(a, bases, events)...)
	out = append(out, boundaryAppendHistoryCoverageCoveredByBases(b, bases, events)...)
	return compactBoundaryAppendHistoryCoverage(out)
}

func intersectBoundaryAppendHistoryCoverage(
	a, b []BoundaryAppendHistoryCoverageFact,
	widenPayload bool,
) []BoundaryAppendHistoryCoverageFact {
	out := boundaryAppendHistoryCoverageRowIdentity.MergeIntersect(a, b, func(left, right BoundaryAppendHistoryCoverageFact) (BoundaryAppendHistoryCoverageFact, bool) {
		fact := cloneBoundaryAppendHistoryCoverage(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryAppendHistoryCoverage(out)
}

func boundaryAppendHistoryCoverageCoveredByBases(
	coverage []BoundaryAppendHistoryCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	events []BoundaryAppendHistoryEventFact,
) []BoundaryAppendHistoryCoverageFact {
	if len(coverage) == 0 || len(bases) == 0 || len(events) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryCoverageFact
	for _, fact := range coverage {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: fact.Array}); !ok {
			continue
		}
		if _, ok := boundaryAppendHistoryEventRowIdentity.Find(events, BoundaryAppendHistoryEventFact{Array: fact.Array, Key: fact.Key}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryCoverage(fact))
	}
	return out
}

func intersectBoundaryAppendHistoryTableCoverageWithBases(
	a, b []BoundaryAppendHistoryTableCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
	widenPayload bool,
) []BoundaryAppendHistoryTableCoverageFact {
	out := intersectBoundaryAppendHistoryTableCoverage(a, b, widenPayload)
	out = append(out, boundaryAppendHistoryTableCoverageCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendHistoryTableCoverageCoveredByBases(b, bases)...)
	return compactBoundaryAppendHistoryTableCoverage(out)
}

func intersectBoundaryAppendHistoryTableCoverage(
	a, b []BoundaryAppendHistoryTableCoverageFact,
	widenPayload bool,
) []BoundaryAppendHistoryTableCoverageFact {
	out := boundaryAppendHistoryTableCoverageRowIdentity.MergeIntersect(a, b, func(left, right BoundaryAppendHistoryTableCoverageFact) (BoundaryAppendHistoryTableCoverageFact, bool) {
		fact := cloneBoundaryAppendHistoryTableCoverage(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryAppendHistoryTableCoverage(out)
}

func boundaryAppendHistoryTableCoverageCoveredByBases(
	coverage []BoundaryAppendHistoryTableCoverageFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendHistoryTableCoverageFact {
	if len(coverage) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendHistoryTableCoverageFact
	for _, fact := range coverage {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: fact.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendHistoryTableCoverage(fact))
	}
	return out
}

func intersectBoundaryAppendElementFieldOriginsWithBases(
	a, b []BoundaryAppendElementFieldOriginFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendElementFieldOriginFact {
	out := intersectBoundaryAppendElementFieldOrigins(a, b)
	out = append(out, boundaryAppendElementFieldOriginsCoveredByBases(a, bases)...)
	out = append(out, boundaryAppendElementFieldOriginsCoveredByBases(b, bases)...)
	return compactBoundaryAppendElementFieldOrigins(out)
}

func intersectBoundaryAppendElementFieldOrigins(a, b []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	return boundaryAppendElementFieldOriginRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendElementFieldOriginFact) (BoundaryAppendElementFieldOriginFact, bool) {
		return cloneBoundaryAppendElementFieldOrigin(left), true
	})
}

func boundaryAppendElementFieldOriginsCoveredByBases(
	origins []BoundaryAppendElementFieldOriginFact,
	bases []BoundaryAppendHistoryBaseFact,
) []BoundaryAppendElementFieldOriginFact {
	if len(origins) == 0 || len(bases) == 0 {
		return nil
	}
	var out []BoundaryAppendElementFieldOriginFact
	for _, origin := range origins {
		if _, ok := boundaryAppendHistoryBaseRowIdentity.Find(bases, BoundaryAppendHistoryBaseFact{Array: origin.Array}); !ok {
			continue
		}
		out = append(out, cloneBoundaryAppendElementFieldOrigin(origin))
	}
	return out
}
