package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestProjectReturnKeyParamProofProjectsKeyPresenceToParamRelation(t *testing.T) {
	paramSym := cfg.SymbolID(11)
	keySym := cfg.SymbolID(12)
	tablePath := constraint.NewPath(paramSym, "self").Field("nodes")
	keyPath := constraint.NewPath(keySym, "id")

	rels := ProjectReturnKeyParamProof(ReturnKeyParamProofQuery{
		ReturnIndex: 0,
		KeyPath:     keyPath,
		KeyPresence: KeyPresenceFacts{}.WithAddresses(
			returnKeyParamProjectionAddress(t, tablePath),
			returnKeyParamProjectionAddress(t, keyPath),
		),
		Boundary: NewBoundaryPathProjection(map[cfg.SymbolID]int{paramSym: 2}, nil),
	})

	want := ReturnKeyParamRelation{
		ReturnIndex: 0,
		ParamIndex:  2,
		ParamSegments: []constraint.Segment{{
			Kind: constraint.SegmentField,
			Name: "nodes",
		}},
	}
	if !rels.HasKeyParam(want) {
		t.Fatalf("return-key relations = %#v, want %#v", rels, want)
	}
}

func TestProjectReturnKeyParamProofIgnoresNonBoundaryTables(t *testing.T) {
	paramSym := cfg.SymbolID(21)
	otherSym := cfg.SymbolID(22)
	keySym := cfg.SymbolID(23)
	tablePath := constraint.NewPath(otherSym, "localTable")
	keyPath := constraint.NewPath(keySym, "id")

	rels := ProjectReturnKeyParamProof(ReturnKeyParamProofQuery{
		ReturnIndex: 0,
		KeyPath:     keyPath,
		KeyPresence: KeyPresenceFacts{}.WithAddresses(
			returnKeyParamProjectionAddress(t, tablePath),
			returnKeyParamProjectionAddress(t, keyPath),
		),
		Boundary: NewBoundaryPathProjection(map[cfg.SymbolID]int{paramSym: 0}, nil),
	})

	if rels.HasProof() {
		t.Fatalf("return-key relations projected non-boundary table: %#v", rels)
	}
}

func returnKeyParamProjectionAddress(t *testing.T, path constraint.Path) StableAddress {
	t.Helper()
	addr, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	return addr
}
