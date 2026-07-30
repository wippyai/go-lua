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
		Hash:      func(v int) uint64 { return uint64(v + 2) },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[int](),
		Canonical: axis.PendingCanonical[int]("test-only axis"),
	}.Erase())

	durable := Set(reg, Top(), key, 0)
	for value := 1; value <= internerShardMaxNode; value++ {
		_ = Set(reg, Top(), key, value)
	}

	count := 0
	for index := range globalInterner.shards {
		shard := &globalInterner.shards[index]
		shard.mu.Lock()
		count += len(shard.fifo)
		shard.mu.Unlock()
	}
	if count > internerShards*internerShardMaxNode {
		t.Fatalf("interner retained %d nodes, limit is %d", count, internerShards*internerShardMaxNode)
	}

	again := Set(reg, Top(), key, 0)
	if !Equal(reg, durable, again) {
		t.Fatal("value changed meaning after its interner entry was evicted")
	}
	if got := Get(reg, durable, key); got != 0 {
		t.Fatalf("durable value = %d, want 0", got)
	}
}

func TestInternerShardsOneRegistryByCandidateHash(t *testing.T) {
	i := newInterner()
	previous := globalInterner
	globalInterner = i
	t.Cleanup(func() { globalInterner = previous })

	key := axis.NewKey[int]("test.interner.same-candidate-hash")
	reg := mustRegistry(t, axis.Spec[int]{
		Key:      key,
		Bottom:   func() int { return -2 },
		Top:      func() int { return -1 },
		Equal:    func(a, b int) bool { return a == b },
		LessOrEq: func(a, b int) bool { return a == b || b == -1 },
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
		Hash:      func(int) uint64 { return 0 },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[int](),
		Canonical: axis.PendingCanonical[int]("test-only axis"),
	}.Erase())

	first := Set(reg, Top(), key, 0)
	second := Set(reg, Top(), key, 1)
	if first.n == nil || second.n == nil || first.n == second.n {
		t.Fatalf("same-hash candidates were not retained as distinct nodes: first=%#v second=%#v", first, second)
	}
	if first.n.hash != second.n.hash {
		t.Fatalf("candidate hashes = %x and %x, want collision", first.n.hash, second.n.hash)
	}

	shard := i.shardFor(reg, first.n.hash)
	shard.mu.Lock()
	bucket := shard.nodes[reg][first.n.hash]
	shard.mu.Unlock()
	if len(bucket) != 2 || bucket[0] != first.n || bucket[1] != second.n {
		t.Fatalf("same-hash bucket = %#v, want [first second]", bucket)
	}
	for index := range i.shards {
		candidate := &i.shards[index]
		if candidate == shard {
			continue
		}
		candidate.mu.Lock()
		_, found := candidate.nodes[reg]
		candidate.mu.Unlock()
		if found {
			t.Fatalf("registry's same-hash candidates also appeared in shard %d", index)
		}
	}
}
