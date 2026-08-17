package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticOperatorRowsRequireAllChildren(t *testing.T) {
	rows := &staticRows{}
	if err := rows.TypeOf(staticTestTerm(keyspace.FamilyTypeOf, 1), 1, 0); err == nil {
		t.Fatal("TypeOf accepted a missing operand")
	}
	if err := rows.Conditional(staticTestTerm(keyspace.FamilyTypeConditional, 1), 1, 2, 3, 0); err == nil {
		t.Fatal("Conditional accepted an incomplete child tuple")
	}
}
