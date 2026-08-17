package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactSignaturesDecoderRetainsFunctionAndAssertionRows(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, signatureFixture(t))
	if len(decoded.Signatures.TypeFunction) != 1 || len(decoded.Signatures.TypeAsserts) != 1 {
		t.Fatalf("decoded signature counts = functions:%d assertions:%d",
			len(decoded.Signatures.TypeFunction), len(decoded.Signatures.TypeAsserts))
	}
	function := decoded.Signatures.TypeFunction[0]
	if function.Scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || len(function.TypeParams) != 1 ||
		len(function.Parameters) != 1 || function.Parameters[0].Type != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		function.Variadic != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) ||
		!function.ReturnsKnown || len(function.Returns) != 1 {
		t.Fatalf("decoded function signature = %+v", function)
	}
	assertion := decoded.Signatures.TypeAsserts[0]
	if !assertion.Bound || assertion.Name != 9 || assertion.Narrow != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("decoded assertion = %+v", assertion)
	}
}
