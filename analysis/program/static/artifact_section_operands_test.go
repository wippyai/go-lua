package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactOperandsDecoderRetainsSparseAndDenseRelations(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, operandsFixture(t))
	if len(decoded.Operands.Claim) != 1 || len(decoded.Operands.TypeValue) != 1 || len(decoded.Operands.Annotation) != 2 {
		t.Fatalf("decoded operand counts = claims:%d type-values:%d annotations:%d",
			len(decoded.Operands.Claim), len(decoded.Operands.TypeValue), len(decoded.Operands.Annotation))
	}
	if row := decoded.Operands.Claim[0]; row.Claim != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) ||
		row.Target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("decoded claim row = %+v", row)
	}
	if target := decoded.Operands.TypeValue[0].Target; target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("decoded type-value target = %v", target)
	}
	if decoded.Operands.Annotation[1].Scope != keyspace.MakeTerm(keyspace.FamilyValueClaim, 2) ||
		decoded.Operands.Annotation[1].Name != 3 {
		t.Fatalf("decoded annotation row = %+v", decoded.Operands.Annotation[1])
	}
}
