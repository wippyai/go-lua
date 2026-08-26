package database

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// deriveStableColumns projects the immutable stable-coordinate inventory from
// sealed mounted layouts once, when the initial database root is born. Declared
// KeySchema components and owner-issued exact Join/Project correspondence
// vectors enter this inventory. Merge-promoted lookup-only vectors remain
// physically indexed but mutable. The sorted inventory is shared by every
// successor.
func deriveStableColumns(mounted witness.Mounted, layouts []arrangement.Layout) ([]model.ColumnID, bool) {
	if !mounted.Available() || layouts == nil {
		return nil, false
	}
	seen := make(map[model.ColumnID]struct{})
	columns := make([]model.ColumnID, 0)
	for _, layout := range layouts {
		if !layout.Available() || !layout.ValidFor(mounted.Fence()) {
			return nil, false
		}
		access := layout.Access()
		keyColumns := layout.KeyColumns()
		switch layout.CoordinateClass() {
		case arrangement.CoordinateClassNone, arrangement.CoordinateClassLookupOnly:
			// Ordinary vectors and Merge-promoted lookup-only vectors are
			// physically indexed but may contain ascending facts. They are
			// advanced by the before/after index delta, never by the stable
			// coordinate fence.
			continue
		case arrangement.CoordinateClassDeclaredKey, arrangement.CoordinateClassStableCorrespondence:
			// These are the only owner-issued immutable coordinate classes.
		default:
			return nil, false
		}
		if len(keyColumns) == 0 {
			return nil, false
		}
		for _, column := range keyColumns {
			if !column.Available() || column.Relation() != access.Relation() {
				return nil, false
			}
			if _, duplicate := seen[column]; duplicate {
				continue
			}
			seen[column] = struct{}{}
			columns = append(columns, column)
		}
	}
	sort.Slice(columns, func(left, right int) bool {
		return stableColumnCompare(columns[left], columns[right]) < 0
	})
	if !validStableColumns(columns) {
		return nil, false
	}
	return columns, true
}

func validStableColumns(columns []model.ColumnID) bool {
	if columns == nil {
		return false
	}
	for index, column := range columns {
		if !column.Available() || index > 0 && stableColumnCompare(columns[index-1], column) >= 0 {
			return false
		}
	}
	return true
}

func stableColumnCompare(left, right model.ColumnID) int {
	leftOwner, rightOwner := left.Relation().Owner().Content(), right.Relation().Owner().Content()
	if result := bytes.Compare(leftOwner[:], rightOwner[:]); result != 0 {
		return result
	}
	leftRelation, rightRelation := left.Relation().Content(), right.Relation().Content()
	if result := bytes.Compare(leftRelation[:], rightRelation[:]); result != 0 {
		return result
	}
	leftColumn, rightColumn := left.Content(), right.Content()
	return bytes.Compare(leftColumn[:], rightColumn[:])
}

func isStableColumn(columns []model.ColumnID, wanted model.ColumnID) bool {
	if columns == nil || !wanted.Available() {
		return false
	}
	position := sort.Search(len(columns), func(index int) bool {
		return stableColumnCompare(columns[index], wanted) >= 0
	})
	return position < len(columns) && columns[position] == wanted
}

// stableColumnDelta enforces the W3 stable-coordinate law over the one canonical
// store delta consumed by database publication. Stable columns are the
// immutable inventory projected from sealed mounted layouts when the root was
// born; no relation/domain declaration is reopened here.
//
// A stable cell that was unavailable may become available. Once its value is
// available, however, the successor must retain an owner-equal value. In
// particular, neither a replacement nor a transition to an unavailable cell
// can cross the aggregate publication boundary. Non-coordinate columns are
// left to their ordinary ascending state semantics.
func stableColumnDelta(mounted witness.Mounted, stableColumns []model.ColumnID, delta store.Delta) bool {
	if !mounted.Available() || stableColumns == nil || !delta.Available() || !delta.Base().Available() || !delta.Next().Available() {
		return false
	}

	for _, change := range delta.Changes() {
		if !isStableColumn(stableColumns, change.ColumnID()) {
			continue
		}
		for index := 0; index < change.Len(); index++ {
			entry, ok := change.At(index)
			if !ok {
				return false
			}
			before, beforePresence, beforeOK := entry.Before()
			after, afterPresence, afterOK := entry.After()
			if beforeOK && !beforePresence.Available() || afterOK && !afterPresence.Available() {
				return false
			}
			beforeAvailable := beforeOK && before.Available()
			afterAvailable := afterOK && after.Available()
			if !beforeAvailable {
				// Sparse/explicitly absent coordinate cells may be established by a
				// later transaction. There is no prior semantic key to keep.
				continue
			}
			if !afterAvailable || !ownerEqualKey(mounted, before, after) {
				return false
			}
		}
	}
	return true
}

func ownerEqualKey(mounted witness.Mounted, before, after binding.ValueToken) bool {
	if !mounted.Available() || !before.Available() || !after.Available() || before.Type() != after.Type() || !before.ValidFor(mounted.RuntimeFence()) || !after.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	equality, ok := mounted.Equality(before.Type())
	return ok && equality != nil && equality.Type() == before.Type() && equality.Equal(before, after)
}
