package accessgeometry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestPathCursorRejectsMalformedParent(t *testing.T) {
	id := identity.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	result := &Result{
		selectorRows: []selectorRow{
			{root: root, plane: selectorPlaneRead, external: true},
			// The parent ordinal is deliberately outside the row plane.
			{root: root, parent: 99, suffix: 7, depth: 1, external: true, plane: selectorPlaneRead},
		},
		selectorRowReads:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2), read},
		selectorReadSlots: []uint32{0, 2}, publicationStart: 3,
		sourceID: id,
		flowID:   id,
		moduleID: id,
		staticID: id,
	}
	if _, ok := result.ExactReads().PathCursor(read); ok {
		t.Fatal("PathCursor accepted malformed parent")
	}
}

func TestDirectCallRejectsSlotWithoutMatchingRow(t *testing.T) {
	id := identity.ContentID{0: 1}
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	result := &Result{
		selectorRows:      []selectorRow{{root: keyspace.MakeTerm(keyspace.FamilyCell, 1), external: true, plane: selectorPlaneRead}},
		selectorRowReads:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2)},
		selectorReadSlots: []uint32{0, 1}, publicationStart: 2,
		directCalls: []directCallRow{{}, {read: read, form: selectorCallPlain}},
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
	id := identity.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	readRoot := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	readLeaf := keyspace.MakeTerm(keyspace.FamilyRead, 2)

	tests := []struct {
		name      string
		read      keyspace.Term
		rows      []selectorRow
		readSlots []uint32
	}{
		{
			name:      "depth-zero root carries suffix",
			read:      readRoot,
			readSlots: []uint32{0, 1},
			rows: []selectorRow{
				{root: root, suffix: 7, plane: selectorPlaneRead, external: true},
			},
		},
		{
			name:      "forward parent",
			read:      readLeaf,
			readSlots: []uint32{0, 1, 2, 3},
			rows: []selectorRow{
				{root: root, plane: selectorPlaneRead, external: true, typePath: true},
				{root: root, parent: 3, suffix: 7, depth: 1, plane: selectorPlaneRead, external: true, typePath: true},
				{root: root, parent: 1, suffix: 8, depth: 2, plane: selectorPlaneRead, external: true, typePath: true},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{
				selectorRows:      test.rows,
				selectorRowReads:  []keyspace.Term{readRoot, readLeaf, keyspace.MakeTerm(keyspace.FamilyRead, 3)},
				selectorReadSlots: test.readSlots,
				publicationStart:  uint32(len(test.rows) + 1),
				sourceID:          id,
				flowID:            id,
				staticID:          id,
				moduleID:          id,
			}
			if _, ok := result.ExactReads().PathCursor(test.read); ok {
				t.Fatal("PathCursor accepted malformed root/forward parent")
			}
		})
	}
}

func TestPathCursorRejectsDepthZeroNonzeroParent(t *testing.T) {
	id := identity.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	result := &Result{
		selectorRows:      []selectorRow{{root: root, parent: 1, plane: selectorPlaneRead, external: true}},
		selectorRowReads:  []keyspace.Term{read},
		selectorReadSlots: []uint32{0, 1}, publicationStart: 2,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	if _, ok := result.ExactReads().PathCursor(read); ok {
		t.Fatal("PathCursor accepted depth-zero root with parent")
	}
	result.selectorRows[0].parent = 0
	result.selectorRows[0].suffix = 7
	if _, ok := result.ExactReads().PathCursor(read); ok {
		t.Fatal("PathCursor accepted depth-zero root with suffix")
	}
}

func TestPathCursorRejectsCycleAndCrossPlane(t *testing.T) {
	id := identity.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read1 := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	read2 := keyspace.MakeTerm(keyspace.FamilyRead, 2)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)

	cycle := &Result{
		selectorRows: []selectorRow{
			{root: root, parent: 2, suffix: 1, depth: 1, plane: selectorPlaneRead, external: true, typePath: true},
			{root: root, parent: 1, suffix: 2, depth: 2, plane: selectorPlaneRead, external: true, typePath: true},
		},
		selectorRowReads:  []keyspace.Term{read1, read2},
		selectorReadSlots: []uint32{0, 1, 2}, publicationStart: 3,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	cursor, ok := cycle.ExactReads().PathCursor(read2)
	if !ok {
		t.Fatal("PathCursor rejected the cycle before traversal")
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatal("PathCursor accepted a cyclic chain edge")
	}

	forgedPublication := &Result{
		selectorRows: []selectorRow{
			{root: root, plane: selectorPlaneRead, external: true, typePath: true},
			// The publication slot points at a binding row.
			{root: root, parent: 1, suffix: 2, depth: 1, plane: selectorPlaneRead, external: true, typePath: true},
		},
		selectorRowReads:  []keyspace.Term{read1, read2},
		selectorReadSlots: []uint32{0, 1, 2},
		publicationSlots:  []uint32{0, 2},
		publicationStart:  3,
		publicationOwners: []keyspace.Term{0, body},
		sourceID:          id,
		flowID:            id,
		staticID:          id,
		moduleID:          id,
	}
	if _, _, _, ok := forgedPublication.TypePublications().Get(publication); ok {
		t.Fatal("TypePublications.Get accepted forged binding row")
	}
	if _, ok := forgedPublication.TypePublications().PathCursor(publication); ok {
		t.Fatal("TypePublications.PathCursor accepted forged binding row")
	}
}

func TestPathCursorRejectsCrossOwnerRootAndParent(t *testing.T) {
	id := identity.ContentID{0: 1}
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)

	wrongRoot := &Result{
		selectorRows:      []selectorRow{{root: keyspace.MakeTerm(keyspace.FamilyBody, 1), plane: selectorPlaneRead, external: true}},
		selectorRowReads:  []keyspace.Term{read},
		selectorReadSlots: []uint32{0, 1}, publicationStart: 2,
		sourceID: id,
		flowID:   id,
		staticID: id,
		moduleID: id,
	}
	if _, ok := wrongRoot.ExactReads().PathCursor(read); ok {
		t.Fatal("PathCursor accepted a non-Cell root")
	}

	parentLocal := &Result{
		selectorRows: []selectorRow{
			{root: root, plane: selectorPlaneRead, external: false, typePath: true},
			{root: root, parent: 1, suffix: 2, depth: 1, plane: selectorPlaneRead, external: true, typePath: true},
		},
		selectorRowReads:  []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyRead, 2), read},
		selectorReadSlots: []uint32{0, 2}, publicationStart: 3,
		sourceID: id, flowID: id, staticID: id, moduleID: id,
	}
	cursor, ok := parentLocal.ExactReads().PathCursor(read)
	if !ok {
		t.Fatal("PathCursor rejected leaf before cross-owner parent check")
	}
	if _, _, ok := cursor.Segment(); ok {
		t.Fatal("PathCursor accepted a non-external parent")
	}
}
