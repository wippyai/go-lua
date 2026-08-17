package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticValidationSeparatesCountedNodesAndRawNames(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	if !validCountedTerm(counts, staticTestTerm(keyspace.FamilyTypePrimitive, 1)) {
		t.Fatal("validCountedTerm rejected an in-range primitive")
	}
	if validCountedTerm(counts, staticTestTerm(keyspace.FamilyTypePrimitive, 2)) {
		t.Fatal("validCountedTerm accepted a future primitive")
	}
	if validRawName(staticRawKey{present: true, value: keyspace.LiteralValue{Kind: keyspace.LiteralString}}) {
		t.Fatal("validRawName accepted an empty string payload")
	}
}
