package directbinding

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

func TestQueriesFailClosedWithoutAllOwnerIDs(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	result := &Result{
		selections: []selectionRow{
			{root: root, plane: selectionPlaneRead, external: true, typePath: true},
			{root: root, parent: 1, suffix: 1, depth: 1, external: true, plane: selectionPlanePublication, typePath: true},
		},
		rowReads:          []keyspace.Term{read, 0},
		readSlots:         []uint32{0, 2},
		publication:       []uint32{0, 2},
		publicationStart:  2,
		publicationOwners: []keyspace.Term{0, body},
		directCalls:       []directCallRow{{}, {read: read, form: CallFormPlain}},
		sourceID:          id,
		flowID:            id,
		moduleID:          id,
		staticID:          id,
	}

	if result.BindingSelections().Count() != 1 || result.PublicationPaths().Count() != 1 || result.DirectCalls().Count() != 1 {
		t.Fatal("well-provenanced result did not expose dense denominators")
	}
	result.staticID = keyspace.ContentID{}
	if result.BindingSelections().Count() != 0 || result.PublicationPaths().Count() != 0 || result.DirectCalls().Count() != 0 {
		t.Fatal("queries exposed rows without all four owner identities")
	}
	if _, _, ok := result.BindingSelections().Get(read); ok {
		t.Fatal("BindingSelections.Get accepted unavailable provenance")
	}
	if _, _, _, ok := result.PublicationPaths().Get(publication); ok {
		t.Fatal("PublicationPaths.Get accepted unavailable provenance")
	}
	if _, _, ok := result.DirectCalls().Get(call); ok {
		t.Fatal("DirectCalls.Get accepted unavailable provenance")
	}
}

func TestPathCursorRejectsMalformedParent(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	result := &Result{
		selections: []selectionRow{
			{root: root, plane: selectionPlaneRead, external: true},
			// The parent ordinal is deliberately outside the row plane.
			{root: root, parent: 99, suffix: 7, depth: 1, external: true, plane: selectionPlaneRead},
		},
		rowReads:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2), read},
		readSlots: []uint32{0, 2}, publicationStart: 3,
		sourceID: id,
		flowID:   id,
		moduleID: id,
		staticID: id,
	}
	if _, ok := result.BindingSelections().PathCursor(read); ok {
		t.Fatal("PathCursor accepted malformed parent")
	}
}

func TestDirectCallRejectsSlotWithoutMatchingRow(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	result := &Result{
		selections: []selectionRow{{root: keyspace.MakeTerm(keyspace.FamilyCell, 1), external: true, plane: selectionPlaneRead}},
		rowReads:   []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2)},
		readSlots:  []uint32{0, 1}, publicationStart: 2,
		directCalls: []directCallRow{{}, {read: read, form: CallFormPlain}},
		sourceID:    id,
		flowID:      id,
		moduleID:    id,
		staticID:    id,
	}
	if _, _, ok := result.DirectCalls().Get(call); ok {
		t.Fatal("DirectCalls.Get accepted a slot whose row read sidecar disagrees")
	}
}

func TestPathCursorRejectsMalformedRootAndForwardParent(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	readRoot := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readLeaf := keyspace.MakeTerm(keyspace.FamilyRead, 2)

	tests := []struct {
		name      string
		read      keyspace.Term
		rows      []selectionRow
		readSlots []uint32
	}{
		{
			name:      "depth-zero root carries suffix",
			read:      readRoot,
			readSlots: []uint32{0, 1},
			rows: []selectionRow{
				{root: root, suffix: 7, plane: selectionPlaneRead, external: true},
			},
		},
		{
			name:      "forward parent",
			read:      readLeaf,
			readSlots: []uint32{0, 1, 2, 3},
			rows: []selectionRow{
				{root: root, plane: selectionPlaneRead, external: true, typePath: true},
				{root: root, parent: 3, suffix: 7, depth: 1, plane: selectionPlaneRead, external: true, typePath: true},
				{root: root, parent: 1, suffix: 8, depth: 2, plane: selectionPlaneRead, external: true, typePath: true},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{
				selections:       test.rows,
				rowReads:         []keyspace.Term{readRoot, readLeaf, keyspace.MakeTerm(keyspace.FamilyRead, 3)},
				readSlots:        test.readSlots,
				publicationStart: uint32(len(test.rows) + 1),
				sourceID:         id,
				flowID:           id,
				staticID:         id,
				moduleID:         id,
			}
			if _, ok := result.BindingSelections().PathCursor(test.read); ok {
				t.Fatal("PathCursor accepted malformed root/forward parent")
			}
		})
	}
}

func TestPathCursorRejectsDepthZeroNonzeroParent(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	result := &Result{
		selections: []selectionRow{{root: root, parent: 1, plane: selectionPlaneRead, external: true}},
		rowReads:   []keyspace.Term{read},
		readSlots:  []uint32{0, 1}, publicationStart: 2,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	if _, ok := result.BindingSelections().PathCursor(read); ok {
		t.Fatal("PathCursor accepted depth-zero root with parent")
	}
	result.selections[0].parent = 0
	result.selections[0].suffix = 7
	if _, ok := result.BindingSelections().PathCursor(read); ok {
		t.Fatal("PathCursor accepted depth-zero root with suffix")
	}
}

func TestPathCursorRejectsCycleAndCrossPlane(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read1 := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	cycle := &Result{
		selections: []selectionRow{
			{root: root, parent: 2, suffix: 1, depth: 1, plane: selectionPlaneRead, external: true, typePath: true},
			{root: root, parent: 1, suffix: 2, depth: 2, plane: selectionPlaneRead, external: true, typePath: true},
		},
		rowReads:  []keyspace.Term{read1, read2},
		readSlots: []uint32{0, 1, 2}, publicationStart: 3,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	cursor, ok := cycle.BindingSelections().PathCursor(read2)
	if !ok {
		t.Fatal("PathCursor rejected the cycle before traversal")
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatal("PathCursor accepted a cyclic chain edge")
	}

	forgedPublication := &Result{
		selections: []selectionRow{
			{root: root, plane: selectionPlaneRead, external: true, typePath: true},
			// The publication slot points at a binding row.
			{root: root, parent: 1, suffix: 2, depth: 1, plane: selectionPlaneRead, external: true, typePath: true},
		},
		rowReads:          []keyspace.Term{read1, read2},
		readSlots:         []uint32{0, 1, 2},
		publication:       []uint32{0, 2},
		publicationStart:  3,
		publicationOwners: []keyspace.Term{0, body},
		sourceID:          id,
		flowID:            id,
		staticID:          id,
		moduleID:          id,
	}
	if _, _, _, ok := forgedPublication.PublicationPaths().Get(publication); ok {
		t.Fatal("PublicationPaths.Get accepted forged binding row")
	}
	if _, ok := forgedPublication.PublicationPaths().PathCursor(publication); ok {
		t.Fatal("PublicationPaths.PathCursor accepted forged binding row")
	}
}

func TestPathCursorRejectsCrossOwnerRootAndParent(t *testing.T) {
	id := keyspace.ContentID{0: 1}
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)

	wrongRoot := &Result{
		selections: []selectionRow{{root: keyspace.MakeTerm(keyspace.FamilyBody, 1), plane: selectionPlaneRead, external: true}},
		rowReads:   []keyspace.Term{read},
		readSlots:  []uint32{0, 1}, publicationStart: 2,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	if _, ok := wrongRoot.BindingSelections().PathCursor(read); ok {
		t.Fatal("PathCursor accepted a non-Cell root")
	}

	parentLocal := &Result{
		selections: []selectionRow{
			{root: root, plane: selectionPlaneRead, external: false, typePath: true},
			{root: root, parent: 1, suffix: 2, depth: 1, plane: selectionPlaneRead, external: true, typePath: true},
		},
		rowReads:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2), read},
		readSlots: []uint32{0, 2}, publicationStart: 3,
		sourceID: id, flowID: id, staticID: id, moduleID: id,
	}
	cursor, ok := parentLocal.BindingSelections().PathCursor(read)
	if !ok {
		t.Fatal("PathCursor rejected leaf before cross-owner parent check")
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatal("PathCursor accepted a non-external parent")
	}
}
