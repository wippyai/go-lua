package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticReferenceRowsPreserveResolutionDisposition(t *testing.T) {
	rows := &staticRows{}
	term := staticTestTerm(keyspace.FamilyTypeRef, 1)
	if err := rows.TypeRefUnresolved(term, 0, []string{"Type"}); err != nil {
		t.Fatal(err)
	}
	if err := rows.TypeRefDeclaration(staticTestTerm(keyspace.FamilyTypeRef, 2), 0, 0, []string{"Type"}); err == nil {
		t.Fatal("TypeRefDeclaration accepted a missing declaration target")
	}
}
