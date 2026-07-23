package pathevidence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestEqualityQuotientClassLookupIsAllocationFreeAtCorpusScale(t *testing.T) {
	keys := keyspace.New()
	const width = 4096
	left, leftOK := keys.FromStableSymbol(symbol.ID(1), nil)
	right, rightOK := keys.FromStableSymbol(symbol.ID(2), nil)
	if !leftOK || !rightOK {
		t.Fatal("equality endpoints")
	}
	proofs := make([]BranchProof, 0, width+1)
	proofs = append(proofs, BranchProof{Kind: BranchProofPathEqual, Path: left, Other: right})
	queries := make([]keyspace.Key, 0, width)
	for index := 0; index < width; index++ {
		candidate, ok := keys.FromStableSymbol(symbol.ID(10000+index), nil)
		if !ok {
			t.Fatal("query key")
		}
		queries = append(queries, candidate)
		proofs = append(proofs, BranchProof{Kind: BranchProofPathNotEqual, Path: candidate, Other: left})
	}
	lane, changed := (Lane{}).AddBranchProofs(proofs)
	if !changed {
		t.Fatal("proof publication unchanged")
	}
	quotient, ok := lane.SealEqualityQuotient(keys)
	if !ok {
		t.Fatal("quotient seal")
	}
	class, ok := quotient.Class(left)
	if !ok {
		t.Fatal("equality endpoint has no indexed class")
	}
	members := 0
	if !quotient.RangeClass(class, func(keyspace.Key) { members++ }) || members != 2 {
		t.Fatalf("equality class members = %d, want 2", members)
	}
	allocations := testing.AllocsPerRun(100, func() {
		for _, query := range queries {
			if _, found := quotient.Class(query); !found {
				panic("indexed observation has no class")
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("indexed lookup allocations/run=%g, want 0", allocations)
	}
}
