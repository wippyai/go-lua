// Package publications owns the authored binding from an Assign write-pair to
// a resolved type reference.
//
// The package is independent of the enclosing Static component. Assign
// geometry stays with Flow and reference spelling stays with References; this
// vertical owns neither and reconstructs no dotted export path. Link later
// derives the export namespace from this relation.
package publications

import (
	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

// Publication is one authored write-pair to type-reference binding. The row
// is already the sealed representation: it has no variable-width child, so it
// needs no separate row shape.
type Publication struct {
	Assign keyspace.Term
	Pair   uint32
	Target keyspace.Term
}

// Input is the complete dense TypePublication denominator.
type Input struct{ Type []Publication }

// Table is the sealed immutable publication relation. The duplicate-pair set
// exists only while building; paths, roots, selection frontiers, and reverse
// maps are owned elsewhere or derived later by Link.
type Table struct{ publication rows.Table[Publication] }

// Count is the sealed publication denominator.
func (table Table) Count() int { return table.publication.Count() }

// CountsMatch reports the native publication denominator against the
// enclosing sealed family column.
func (table Table) CountsMatch(counts [keyspace.FamilyCount]uint32) bool {
	return table.Count() == int(counts[keyspace.FamilyTypePublication])
}

// CountRows publishes this typed owner's native publication measure under the
// generated ProgramStatic denominator identity.
func (table Table) CountRows() (denominator.CountRows, bool) {
	value := table.Count()
	if !keyspace.TermOrdinalFits(value) {
		return denominator.CountRows{}, false
	}
	row, ok := denominator.NewCountRow(denominator.GeneratedProgramStaticIDs().ProgramStaticPublication, uint64(value))
	if !ok {
		return denominator.CountRows{}, false
	}
	return denominator.NewCountRows([]denominator.CountRow{row})
}

// VisitContainment emits the resolved TypeRef child of each publication.
// Assign remains a Flow anchor and is deliberately not traversed.
func (table Table) VisitContainment(attach func(parent, child keyspace.Term) bool) bool {
	if attach == nil {
		return false
	}
	for parent, row := range table.publication.Terms() {
		if !attach(parent, row.Target) {
			return false
		}
	}
	return true
}
