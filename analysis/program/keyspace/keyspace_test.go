package keyspace

import "testing"

func TestLiteralValueRetainsAuthoredPayloadByKind(t *testing.T) {
	values := []LiteralValue{
		{Kind: LiteralBool, Bool: true},
		{Kind: LiteralInteger, Integer: -7},
		{Kind: LiteralFloat, FloatBits: 0x8000000000000000},
		{Kind: LiteralString, String: "a\x00z"},
	}
	if !values[0].Bool || values[1].Integer != -7 || values[2].FloatBits != 0x8000000000000000 || values[3].String != "a\x00z" {
		t.Fatalf("LiteralValue lost an authored payload: %#v", values)
	}
	if LiteralBool == LiteralInteger || LiteralInteger == LiteralFloat || LiteralFloat == LiteralString {
		t.Fatal("literal kind vocabulary is not disjoint")
	}
}
