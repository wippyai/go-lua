package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func TestInternerEvictionKeepsDurableValuesValid(t *testing.T) {
	previous := globalInterner
	globalInterner = newInterner()
	t.Cleanup(func() { globalInterner = previous })

	key := axis.NewKey[int]("test.interner.retention")
	reg := mustRegistry(t, axis.Spec[int]{
		Key:    key,
		Bottom: func() int { return -2 },
		Top:    func() int { return -1 },
		Equal:  func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool {
			return a == b || b == -1
		},
		Join: func(a, b int) int {
			if a == b {
				return a
			}
			return -1
		},
		Meet: func(a, b int) int {
			if a == b {
				return a
			}
			return -2
		},
		Hash: func(v int) uint64 { return uint64(v + 2) },
	}.Erase())

	durable := Set(reg, Top(), key, 0)
	for value := 1; value <= internerShardMaxNode; value++ {
		_ = Set(reg, Top(), key, value)
	}

	shard := globalInterner.shardFor(reg)
	shard.mu.Lock()
	count := len(shard.fifo)
	shard.mu.Unlock()
	if count > internerShardMaxNode {
		t.Fatalf("interner shard retained %d nodes, limit is %d", count, internerShardMaxNode)
	}

	again := Set(reg, Top(), key, 0)
	if !Equal(reg, durable, again) {
		t.Fatal("value changed meaning after its interner entry was evicted")
	}
	if got := Get(reg, durable, key); got != 0 {
		t.Fatalf("durable value = %d, want 0", got)
	}
}
