package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactOperatorsDecoderRetainsEachTypedOperator(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, operatorFixture())
	if len(decoded.Operators.TypeOf) != 2 || len(decoded.Operators.KeyOf) != 1 ||
		len(decoded.Operators.IndexAccess) != 1 || len(decoded.Operators.Conditional) != 1 {
		t.Fatalf("decoded operator counts = typeof:%d keyof:%d index:%d conditional:%d",
			len(decoded.Operators.TypeOf), len(decoded.Operators.KeyOf), len(decoded.Operators.IndexAccess), len(decoded.Operators.Conditional))
	}
	if row := decoded.Operators.TypeOf[0]; row.Scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) ||
		row.Operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("decoded typeof row = %+v", row)
	}
	if row := decoded.Operators.Conditional[0]; row.Check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) ||
		row.Extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) ||
		row.Then != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5) ||
		row.Else != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 6) {
		t.Fatalf("decoded conditional row = %+v", row)
	}
}
