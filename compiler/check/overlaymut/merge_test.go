package overlaymut

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestApplyIndexerMergeToOverlay_TreatsNilWriteAsDeletion(t *testing.T) {
	overlay := make(map[cfg.SymbolID]typ.Type)
	ApplyIndexerMergeToOverlay(overlay, map[cfg.SymbolID][]IndexerInfo{
		1: {
			{KeyType: typ.String, ValType: typ.Nil},
		},
	})
	if got := overlay[1]; got != nil {
		t.Fatalf("nil index write should not create map value evidence, got %v", got)
	}
}

func TestApplyIndexerMergeToOverlay_DeletionDoesNotPoisonWriteValue(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	overlay := make(map[cfg.SymbolID]typ.Type)
	ApplyIndexerMergeToOverlay(overlay, map[cfg.SymbolID][]IndexerInfo{
		1: {
			{KeyType: typ.String, ValType: typ.Nil},
			{KeyType: typ.String, ValType: entry},
		},
	})

	got, ok := overlay[1].(*typ.Map)
	if !ok {
		t.Fatalf("mixed deletion/write should create map evidence, got %T", overlay[1])
	}
	if !typ.TypeEquals(got.Value, entry) {
		t.Fatalf("map value evidence = %v, want %v", got.Value, entry)
	}
}
