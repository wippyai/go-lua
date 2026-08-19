package static

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func TestStaticTypeRowsKeepDensePrimitiveAndLiteralFamilies(t *testing.T) {
	rows := &staticRows{}
	primitive := staticTestTerm(keyspace.FamilyTypePrimitive, 1)
	literal := staticTestTerm(keyspace.FamilyTypeLiteral, 1)
	if err := rows.Primitive(primitive, statictypes.PrimitiveString); err != nil {
		t.Fatal(err)
	}
	if err := rows.LiteralString(literal, "literal"); err != nil {
		t.Fatal(err)
	}
	if err := rows.LiteralFloat(staticTestTerm(keyspace.FamilyTypeLiteral, 2), math.Float64bits(math.NaN())); err == nil {
		t.Fatal("LiteralFloat accepted NaN")
	}
}
