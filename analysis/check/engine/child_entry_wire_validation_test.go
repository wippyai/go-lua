package engine

import (
	"encoding/json"
	"testing"
)

// TestChildEntryWireRejectsMalformedPayloads is a table-driven trust-seam
// test.  Decode is deliberately the first consumer of a foreign entry packet:
// no malformed packet may reach entryKernel and be only partly applied.
func TestChildEntryWireRejectsMalformedPayloads(t *testing.T) {
	valid := childEntryWire{
		Version: 4,
		Seeds:   []entrySeed{{Term: "path/sym1", Value: []byte("scalar/claim/claim-kind/3/\"any\"")}},
	}
	cases := []struct {
		name string
		wire childEntryWire
	}{
		{"duplicate-seed", childEntryWire{Version: 4, Seeds: []entrySeed{{Term: "path/sym1", Value: []byte("x")}, {Term: "path/sym1", Value: []byte("y")}}}},
		{"capability-without-seed", childEntryWire{Version: 4, ClosureSeeds: []entryClosureSeed{{Term: "path/sym1", Handle: closureHandle{Prototype: "child"}}}}},
		{"gradual-any-without-seed", childEntryWire{Version: 4, GradualAnyTerms: []string{"path/sym1"}}},
		{"malformed-placement", childEntryWire{Version: 6, Seeds: valid.Seeds, PlacementSeeds: []entryPlacementSeed{{Term: "path/sym1", Allocation: placementAllocationFact{Identity: "i"}}}}},
		{"member-cell-without-table-identity", childEntryWire{Version: 4, Seeds: valid.Seeds, MemberCellSeeds: []entryMemberCellSeed{{Identity: []byte("table"), Suffix: ".field", Wire: memberCellWire{Value: []byte("value")}}}}},
		{"version-skew", childEntryWire{Version: 5, Seeds: valid.Seeds, PlacementSeeds: []entryPlacementSeed{{Term: "path/sym1", Allocation: placementAllocationFact{Identity: "i", Result: "path/sym1", Kind: "table"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.wire)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeChildEntryWire(append([]byte("front/child-entry/v"+string(rune('0'+tc.wire.Version))+"/"), payload...)); err == nil {
				t.Fatal("decodeChildEntryWire accepted malformed payload")
			}
		})
	}
}
