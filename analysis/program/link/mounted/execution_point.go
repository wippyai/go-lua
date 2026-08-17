// Package mounted owns the populations a Link seals when it places compiled
// Programs at mounts: which semantic execution points exist, which of them are
// independent execution roots, and where results are observed.
//
// A mount is a Link fact and a point is an artifact fact, so the population
// that relates the two belongs to neither alone. It is derived here, from the
// immutable compiled artifact plus the Link-local module identity that places
// it, and from nothing else. No query family, diagnostic flag, or solver state
// is consulted: what exists to be executed is a property of the sealed program
// at its mount, not of who intends to read it.
//
// Each population is a plain sealed value: an ordered row set with an exact
// count and an addressable row. Order is canonical -- rows are sorted by the
// bytes of their key -- so a census is a function of the sealed content alone
// and never of map iteration, registration order, mount order, or the order a
// consumer published in.
package mounted

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
)

// ExecutionPointDenominator is the authored identity of the closed world
// ExecutionPoints is the membership of. A totality claim about mounted
// execution is only as good as the set it quantifies over, so the set has one
// spelling in the analyzer and this is it. It is declared as text rather than
// as a declaration-table key because the surface entry that will own the
// universe seals above this package.
const ExecutionPointDenominator = "subjects/mounted-execution-point"

// ExecutionPoint is one semantic execution point of one mount. It is the
// subject a mounted result is held about: the Link-local module identity that
// places a compiled Program, and the artifact-issued LocalWTO phase vertex
// inside it.
type ExecutionPoint struct {
	Mount identity.ContentID
	Point identity.ContentID
}

func (key ExecutionPoint) Available() bool {
	return key.Mount.Available() && key.Point.Available()
}

// CompareExecutionPoint is the canonical order of the key: the bytes of the
// mount identity, then the bytes of the point identity. It is a total order on
// available keys and is a function of the key alone, so a census sorted under
// it is independent of the order its rows were derived in.
func CompareExecutionPoint(left, right ExecutionPoint) int {
	if order := bytes.Compare(left.Mount[:], right.Mount[:]); order != 0 {
		return order
	}
	return bytes.Compare(left.Point[:], right.Point[:])
}

// ExecutionPoints is the complete mounted execution-point population of a
// sealed Link: every point the demand graph admits at every mount, including
// the interiors of callable bodies whether a call ever reaches them or not.
// The set is the denominator; the reached subset is the engine's to publish
// against it, and is deliberately not represented here.
type ExecutionPoints struct {
	rows   []ExecutionPoint
	sealed bool
}

func (set ExecutionPoints) Available() bool { return set.sealed && len(set.rows) != 0 }

func (set ExecutionPoints) Count() int {
	if !set.Available() {
		return 0
	}
	return len(set.rows)
}

func (set ExecutionPoints) At(index int) (ExecutionPoint, bool) {
	if !set.Available() || index < 0 || index >= len(set.rows) {
		return ExecutionPoint{}, false
	}
	return set.rows[index], true
}

// Contains answers membership of the denominator. The rows are held in
// canonical order, so the answer is a binary search over the sealed column
// rather than a second index a caller would have to keep in step.
func (set ExecutionPoints) Contains(key ExecutionPoint) bool {
	if !set.Available() || !key.Available() {
		return false
	}
	index := sort.Search(len(set.rows), func(index int) bool {
		return CompareExecutionPoint(set.rows[index], key) >= 0
	})
	return index < len(set.rows) && set.rows[index] == key
}

// SealExecutionPoints derives the denominator from the placed artifacts. Every
// point row of every mount is a member: a point is admitted to the schedule by
// the sealed program that owns it, so membership cannot depend on a body being
// callable, on a call selecting it, or on an observation naming it.
func SealExecutionPoints(mounts []Mount) (ExecutionPoints, bool) {
	if !mountsAvailable(mounts) {
		return ExecutionPoints{}, false
	}
	rows := make([]ExecutionPoint, 0)
	for _, mount := range mounts {
		for index := 0; index < mount.Artifact.PointCount(); index++ {
			point, ok := mount.Artifact.PointAt(index)
			if !ok || !point.Available() || !point.ID().Available() {
				return ExecutionPoints{}, false
			}
			rows = append(rows, ExecutionPoint{Mount: mount.ModuleKey, Point: point.ID()})
		}
	}
	return sealExecutionPointColumn(rows)
}

// sealExecutionPointColumn freezes a derived row set into canonical order. A
// repeated key is corruption of the derivation rather than a duplicate to
// collapse, so the column fails closed instead of deduplicating.
func sealExecutionPointColumn(rows []ExecutionPoint) (ExecutionPoints, bool) {
	if len(rows) == 0 {
		return ExecutionPoints{}, false
	}
	sort.Slice(rows, func(left, right int) bool {
		return CompareExecutionPoint(rows[left], rows[right]) < 0
	})
	for index, row := range rows {
		if !row.Available() || index != 0 && CompareExecutionPoint(rows[index-1], row) >= 0 {
			return ExecutionPoints{}, false
		}
	}
	return ExecutionPoints{rows: rows, sealed: true}, true
}
