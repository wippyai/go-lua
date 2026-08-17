package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticPublicationRowsRequireAssignAndReferenceTarget(t *testing.T) {
	rows := &staticRows{}
	publication := staticTestTerm(keyspace.FamilyTypePublication, 1)
	if err := rows.TypePublication(publication, staticTestTerm(keyspace.FamilyAssign, 1), 0, staticTestTerm(keyspace.FamilyTypeRef, 1)); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypePublication(staticTestTerm(keyspace.FamilyTypePublication, 2), 0, 0, 0); err == nil {
		t.Fatal("TypePublication accepted incomplete ownership")
	}
}
