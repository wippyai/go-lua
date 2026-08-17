package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactContractsDecoderRetainsFunctionAndCallRows(t *testing.T) {
	decoded := decodeStaticArtifactInputForTest(t, contractsFixture(t))
	if len(decoded.Contracts.Function) != 1 || len(decoded.Contracts.Call) != 1 {
		t.Fatalf("decoded contract rows = (%d, %d), want (1, 1)", len(decoded.Contracts.Function), len(decoded.Contracts.Call))
	}
	function := decoded.Contracts.Function[0]
	if !function.ReturnsKnown || len(function.TypeParams) != 1 ||
		function.TypeParams[0] != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) ||
		len(function.Returns) != 1 || function.Returns[0] != keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1) {
		t.Fatalf("decoded function contract = %+v", function)
	}
	call := decoded.Contracts.Call[0]
	wantArgs := []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3),
	}
	if len(call.TypeArguments) != len(wantArgs) || call.TypeArguments[0] != wantArgs[0] || call.TypeArguments[1] != wantArgs[1] {
		t.Fatalf("decoded call contract arguments = %v, want %v", call.TypeArguments, wantArgs)
	}
}
