package summary

import (
	"sync"

	"github.com/wippyai/go-lua/types/flow"
)

// EntryFactsKey is an exact comparable key for caller-projected function-entry
// path facts. The carrier is flow.BoundaryFacts restricted to parameter-relative
// paths: summary owns the interprocedural context axis, while flow owns the fact
// language and lattice laws.
type EntryFactsKey struct {
	n *entryFactsKeyNode
}

type entryFactsKeyNode struct {
	facts flow.BoundaryFacts
	hash  uint64
}

type entryFactsKeyInterner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*entryFactsKeyNode
}

var canonicalEntryFactsKeys = &entryFactsKeyInterner{buckets: make(map[uint64][]*entryFactsKeyNode)}

// resetEntryFactsKeyInterner clears the comparable-key interner for one checker
// analysis scope.
func resetEntryFactsKeyInterner() {
	canonicalEntryFactsKeys.mu.Lock()
	defer canonicalEntryFactsKeys.mu.Unlock()
	canonicalEntryFactsKeys.buckets = make(map[uint64][]*entryFactsKeyNode)
}

// entryFactsKeyOf returns a comparable key for caller-projected function-entry
// facts. Recursive self calls normalize their facts before key construction; for
// ordinary exact contexts the full boundary-fact payload remains part of the key
// because it is also the callee entry seed.
func entryFactsKeyOf(facts flow.BoundaryFacts) EntryFactsKey {
	return internEntryFactsKey(facts)
}

// Facts returns the immutable entry facts represented by k. The zero key denotes
// no finite entry facts.
func (k EntryFactsKey) Facts() flow.BoundaryFacts {
	if k.n == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return k.n.facts.Clone()
}

func internEntryFactsKey(facts flow.BoundaryFacts) EntryFactsKey {
	canonical := facts.Clone()
	if !canonical.HasProof() {
		return EntryFactsKey{}
	}
	h := canonical.IdentityHash("summary.EntryFactsKey")

	canonicalEntryFactsKeys.mu.RLock()
	if existing, ok := lookupEntryFactsKey(canonicalEntryFactsKeys.buckets[h], canonical); ok {
		canonicalEntryFactsKeys.mu.RUnlock()
		return EntryFactsKey{n: existing}
	}
	canonicalEntryFactsKeys.mu.RUnlock()

	canonicalEntryFactsKeys.mu.Lock()
	defer canonicalEntryFactsKeys.mu.Unlock()
	if existing, ok := lookupEntryFactsKey(canonicalEntryFactsKeys.buckets[h], canonical); ok {
		return EntryFactsKey{n: existing}
	}
	node := &entryFactsKeyNode{facts: canonical, hash: h}
	canonicalEntryFactsKeys.buckets[h] = append(canonicalEntryFactsKeys.buckets[h], node)
	return EntryFactsKey{n: node}
}

func lookupEntryFactsKey(bucket []*entryFactsKeyNode, facts flow.BoundaryFacts) (*entryFactsKeyNode, bool) {
	for _, node := range bucket {
		if flow.BoundaryFactsDomain.Equal(node.facts, facts) {
			return node, true
		}
	}
	return nil, false
}

func mergeEntryFacts(a, b flow.BoundaryFacts) flow.BoundaryFacts {
	return flow.UnionBoundaryFactProofs(a.Clone(), b.Clone())
}
