// Package operands owns the three authored Static operand sidecars: the
// sparse ClaimTarget relation, the dense runtime TypeValue targets, and
// authored annotations with their query index.
//
// The package is independent of the enclosing Static component. It does not
// inspect Flow claim kinds, reconstruct values, or derive annotations: those
// would duplicate Flow and Source authority.
package operands

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/rows"
)

// ClaimTarget is the optional authored type operand of one Flow ValueClaim.
// Its sparse denominator is the nonzero target relation, not ValueClaim
// cardinality: a postfix non-nil claim has no static type operand.
type ClaimTarget struct {
	Claim  keyspace.Term
	Target keyspace.Term
}

// TypeValueTarget is the authored runtime-loadable target of one Flow
// TypeValue. The relation is dense by the TypeValue canonical ordinal.
type TypeValueTarget struct{ Target keyspace.Term }

// Annotation is authored metadata attached to one static type occurrence or
// field. Scope and Values are cross-owner references; neither is a concrete
// static type child.
type Annotation struct {
	Scope  keyspace.Term
	Target keyspace.Term
	Name   keyspace.Key
	Values keyspace.Term
}

// Input contains the three exact Static operand sidecars. Claim is sparse;
// TypeValue and Annotation are dense by their canonical families.
type Input struct {
	Claim      []ClaimTarget
	TypeValue  []TypeValueTarget
	Annotation []Annotation
}

// AnnotationIndex is a query-only acceleration structure over complete
// Annotation rows: for each distinct target, in target order, the window of
// annotation terms that name it. It is never a second semantic authority and
// never a future artifact denominator, so it is sealed apart from the rows
// and excluded from the section stream.
type AnnotationIndex struct {
	targets rows.Rows[keyspace.Term]
	windows rows.Rows[rows.Span]
	terms   rows.Pool[keyspace.Term]
}

// Count is the number of annotations naming target, and whether the index
// holds a window for it at all.
func (index AnnotationIndex) Count(target keyspace.Term) (int, bool) {
	position := index.find(target)
	if position < 0 {
		return 0, false
	}
	window, ok := index.windows.At(position)
	if !ok {
		return 0, false
	}
	return index.terms.Count(window), true
}

// At returns one annotation term naming target.
func (index AnnotationIndex) At(target keyspace.Term, offset int) (keyspace.Term, bool) {
	position := index.find(target)
	if position < 0 {
		return 0, false
	}
	window, ok := index.windows.At(position)
	if !ok {
		return 0, false
	}
	return index.terms.At(window, offset)
}

// find is a binary search over the index's stable target order.
func (index AnnotationIndex) find(target keyspace.Term) int {
	count := index.targets.Count()
	position := sort.Search(count, func(candidate int) bool {
		value, ok := index.targets.At(candidate)
		return ok && value >= target
	})
	if position == count {
		return -1
	}
	if value, ok := index.targets.At(position); !ok || value != target {
		return -1
	}
	return position
}

// Table is the sealed immutable operand relation set.
//
// claim is the sparse semantic relation in canonical Flow ValueClaim order.
// Queries resolve it directly; no second dense target plane is retained.
type Table struct {
	claim      rows.Rows[ClaimTarget]
	typeValue  rows.Table[keyspace.Term]
	annotation rows.Table[Annotation]
	index      AnnotationIndex
}

// ClaimCount is the sparse semantic ClaimTarget denominator: only claims with
// an authored static target participate. It is the denominator the schema
// publishes for this relation.
func (table Table) ClaimCount() int { return table.claim.Count() }

// Count reports the sealed row denominator of one dense operand family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypeValue:
		return table.typeValue.Count()
	case keyspace.FamilyAnnotation:
		return table.annotation.Count()
	default:
		return 0
	}
}

// CountsMatch reports the native operand denominators. ClaimTarget is sparse:
// its authored rows must fit inside Flow's ValueClaim universe rather than
// fill that universe.
func (table Table) CountsMatch(counts [keyspace.FamilyCount]uint32) bool {
	return table.Count(keyspace.FamilyTypeValue) == int(counts[keyspace.FamilyTypeValue]) &&
		table.Count(keyspace.FamilyAnnotation) == int(counts[keyspace.FamilyAnnotation]) &&
		table.ClaimCount() <= int(counts[keyspace.FamilyValueClaim])
}

// CountRows publishes this typed owner's native operand measures under the
// generated ProgramStatic denominator identities.
func (table Table) CountRows() (denominator.CountRows, bool) {
	claimCount := table.ClaimCount()
	typeValueCount := table.Count(keyspace.FamilyTypeValue)
	annotationCount := table.Count(keyspace.FamilyAnnotation)
	if !keyspace.TermOrdinalFits(claimCount) || !keyspace.TermOrdinalFits(typeValueCount) || !keyspace.TermOrdinalFits(annotationCount) {
		return denominator.CountRows{}, false
	}
	ids := denominator.GeneratedProgramStaticIDs()
	claim, ok := denominator.NewCountRow(ids.ProgramStaticClaimTarget, uint64(claimCount))
	if !ok {
		return denominator.CountRows{}, false
	}
	typeValue, ok := denominator.NewCountRow(ids.ProgramStaticTypeValueTarget, uint64(typeValueCount))
	if !ok {
		return denominator.CountRows{}, false
	}
	annotation, ok := denominator.NewCountRow(ids.ProgramStaticAnnotation, uint64(annotationCount))
	if !ok {
		return denominator.CountRows{}, false
	}
	return denominator.NewCountRows([]denominator.CountRow{claim, typeValue, annotation})
}

// VisitContainment retains Flow terms solely as opaque parents of the
// authored Static operand they already identify. Annotation is metadata, not
// containment, and therefore deliberately emits no edge.
func (table Table) VisitContainment(attach func(parent, child keyspace.Term) bool) bool {
	if attach == nil {
		return false
	}
	for _, row := range table.claim.All() {
		if row.Target != 0 && !attach(row.Claim, row.Target) {
			return false
		}
	}
	for value, target := range table.typeValue.Terms() {
		if !attach(value, target) {
			return false
		}
	}
	return true
}
