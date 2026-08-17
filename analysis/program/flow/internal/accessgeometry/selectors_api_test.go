package accessgeometry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestQueriesFailClosedWithoutAllOwnerIDs(t *testing.T) {
	id := identity.ContentID{0: 1}
	root := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	read := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	publication := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	result := &Result{
		selectorRows: []selectorRow{
			{root: root, plane: selectorPlaneRead, external: true, typePath: true},
			{root: root, parent: 1, suffix: 1, depth: 1, external: true, plane: selectorPlanePublication, typePath: true},
		},
		selectorRowReads:  []keyspace.Term{read, 0},
		selectorReadSlots: []uint32{0, 2},
		publicationSlots:  []uint32{0, 2},
		publicationStart:  2,
		publicationOwners: []keyspace.Term{0, body},
		directCalls:       []directCallRow{{}, {read: read, form: selectorCallPlain}},
		sourceID:          id,
		flowID:            id,
		moduleID:          id,
		staticID:          id,
	}

	if result.ExactReads().Count() != 1 || result.TypePublications().Count() != 1 || result.DirectCalls().Count() != 1 {
		t.Fatal("well-provenanced result did not expose dense denominators")
	}
	result.staticID = identity.ContentID{}
	if result.ExactReads().Count() != 0 || result.TypePublications().Count() != 0 || result.DirectCalls().Count() != 0 {
		t.Fatal("queries exposed rows without all four owner identities")
	}
	if _, _, ok := result.ExactReads().Get(read); ok {
		t.Fatal("ExactReads.Get accepted unavailable provenance")
	}
	if _, _, _, ok := result.TypePublications().Get(publication); ok {
		t.Fatal("TypePublications.Get accepted unavailable provenance")
	}
	if _, _, ok := result.DirectCalls().Get(call); ok {
		t.Fatal("DirectCalls.Get accepted unavailable provenance")
	}
}
