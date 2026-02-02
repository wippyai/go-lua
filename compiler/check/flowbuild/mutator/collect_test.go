package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexerInfo(t *testing.T) {
	info := IndexerInfo{
		KeyType: typ.String,
		ValType: typ.Integer,
	}
	if info.KeyType != typ.String {
		t.Errorf("expected KeyType to be String, got %v", info.KeyType)
	}
	if info.ValType != typ.Integer {
		t.Errorf("expected ValType to be Integer, got %v", info.ValType)
	}
}

func TestCollectTableInsertMutations_NilGraph(t *testing.T) {
	result := CollectTableInsertMutations(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil map for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestCollectTableInsertOnDirect_NilGraph(t *testing.T) {
	result := CollectTableInsertOnDirect(nil, nil, nil)
	if result == nil {
		t.Error("expected non-nil map for nil graph")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestMergeIndexerMutations_EmptyInputs(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	mutations := make(map[cfg.SymbolID][]IndexerInfo)

	MergeIndexerMutations(indexers, mutations)

	if len(indexers) != 0 {
		t.Errorf("expected empty indexers after merging empty mutations, got %d", len(indexers))
	}
}

func TestMergeIndexerMutations_MergesCorrectly(t *testing.T) {
	indexers := make(map[cfg.SymbolID][]IndexerInfo)
	indexers[1] = []IndexerInfo{{KeyType: typ.String, ValType: typ.Integer}}

	mutations := make(map[cfg.SymbolID][]IndexerInfo)
	mutations[1] = []IndexerInfo{{KeyType: typ.Integer, ValType: typ.String}}
	mutations[2] = []IndexerInfo{{KeyType: typ.Number, ValType: typ.Boolean}}

	MergeIndexerMutations(indexers, mutations)

	if len(indexers[1]) != 2 {
		t.Errorf("expected 2 infos for symbol 1, got %d", len(indexers[1]))
	}
	if len(indexers[2]) != 1 {
		t.Errorf("expected 1 info for symbol 2, got %d", len(indexers[2]))
	}
}
