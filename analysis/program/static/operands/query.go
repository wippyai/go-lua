package operands

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// View is the immutable query surface over a sealed operand table. It holds
// the sealed table by value: the enclosing owner checks its publication fence
// once when it mints the view. A zero View is permanently unavailable.
//
// census is the sealed cardinality column the annotation-target admission
// reads, so the query boundary admits exactly the targets Build admitted
// rather than recounting a relation to decide the same question.
type View struct {
	table     Table
	census    [keyspace.FamilyCount]uint32
	available bool
}

// NewView returns a query surface over this sealed table under the owner's
// sealed census column.
func (table Table) NewView(census [keyspace.FamilyCount]uint32) View {
	return View{table: table, census: census, available: true}
}

// Available reports whether this view resolves to a sealed operand set.
func (view View) Available() bool { return view.available }

type ClaimTargets struct{ view View }
type TypeValueTargets struct{ view View }
type Annotations struct{ view View }

func (view View) Claims() ClaimTargets         { return ClaimTargets{view: view} }
func (view View) TypeValues() TypeValueTargets { return TypeValueTargets{view: view} }
func (view View) Annotations() Annotations     { return Annotations{view: view} }

// Count is the sparse semantic ClaimTarget denominator: only claims with an
// authored static target participate.
func (view ClaimTargets) Count() int {
	if !view.view.available {
		return 0
	}
	return view.view.table.claim.Count()
}

func (view ClaimTargets) At(index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.claim.At(index)
	return row.Claim, ok
}

// Target reports one member of the sparse semantic ClaimTarget relation. A
// false result means this Claim has no authored static target. Callers that
// need to distinguish a known Flow claim from an unknown identity query Flow;
// this relation must not overload its presence bit with Flow membership.
func (view ClaimTargets) Target(claim keyspace.Term) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	if keyspace.TermFamily(claim) != keyspace.FamilyValueClaim || keyspace.TermOrdinal(claim) == 0 {
		return 0, false
	}
	rows := view.view.table.claim
	position := sort.Search(rows.Count(), func(index int) bool {
		row, ok := rows.At(index)
		return ok && row.Claim >= claim
	})
	if position == rows.Count() {
		return 0, false
	}
	row, ok := rows.At(position)
	if !ok || row.Claim != claim || row.Target == 0 {
		return 0, false
	}
	return row.Target, true
}

func (view TypeValueTargets) Count() int {
	if !view.view.available {
		return 0
	}
	return view.view.table.Count(keyspace.FamilyTypeValue)
}

func (view TypeValueTargets) At(index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	return view.view.table.typeValue.Term(index)
}

func (view TypeValueTargets) Target(term keyspace.Term) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	return view.view.table.typeValue.Row(term)
}

func (view Annotations) Count() int {
	if !view.view.available {
		return 0
	}
	return view.view.table.Count(keyspace.FamilyAnnotation)
}

func (view Annotations) At(index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	return view.view.table.annotation.Term(index)
}

func (view Annotations) Get(term keyspace.Term) (Annotation, bool) {
	if !view.view.available {
		return Annotation{}, false
	}
	return view.view.table.annotation.Row(term)
}

// ForCount distinguishes a valid target with no annotations (0, true) from an
// invalid target (0, false). The index is only an allocation-free query
// acceleration; Annotation rows remain the sole semantic relation.
func (view Annotations) ForCount(target keyspace.Term) (int, bool) {
	if !view.view.available || !view.targetPresent(target) {
		return 0, false
	}
	count, ok := view.view.table.index.Count(target)
	if !ok {
		return 0, true
	}
	return count, true
}

func (view Annotations) ForAt(target keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.view.available || index < 0 || !view.targetPresent(target) {
		return 0, false
	}
	return view.view.table.index.At(target, index)
}

// targetPresent binds the authored Annotation target role to the sealed
// census column its owner assigned.
func (view Annotations) targetPresent(target keyspace.Term) bool {
	return staticrole.AnnotationTarget(view.view.census, target)
}
