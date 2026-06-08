package summary

import (
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/constraint"
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

// entryFactsKeyOf returns an exact comparable key for facts.
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
	h := entryFactsKeyHash(canonical)

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

func entryFactsKeyHash(facts flow.BoundaryFacts) uint64 {
	h := internal.FnvString("summary.EntryFactsKey")
	for _, fact := range facts.KeyPresence() {
		h = internal.HashCombine(h, internal.FnvString("kp"))
		h = hashBoundaryPath(h, fact.Table)
		h = hashBoundaryPath(h, fact.Key)
	}
	for _, fact := range facts.KeyArrays() {
		h = internal.HashCombine(h, internal.FnvString("ka"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Table)
	}
	for _, fact := range facts.KeyArrayValues() {
		h = internal.HashCombine(h, internal.FnvString("kav"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Table)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	for _, fact := range facts.AppendKeys() {
		h = internal.HashCombine(h, internal.FnvString("ak"))
		h = hashBoundaryPath(h, fact.Array)
		h = hashBoundaryPath(h, fact.Key)
		if fact.HasTable {
			h = internal.HashCombine(h, 1)
			h = hashBoundaryPath(h, fact.Table)
		} else {
			h = internal.HashCombine(h, 0)
		}
	}
	for _, fact := range facts.AppendElementFieldOrigins() {
		h = internal.HashCombine(h, internal.FnvString("aefo"))
		h = hashBoundaryPath(h, fact.Array)
		h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(fact.Field)))
		h = hashBoundaryPath(h, fact.Source)
		h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(fact.SourceField)))
	}
	for _, fact := range facts.LengthLowerBounds() {
		h = internal.HashCombine(h, internal.FnvString("len"))
		h = hashBoundaryPath(h, fact.Target)
		h = internal.HashCombine(h, uint64(fact.Lower))
	}
	for _, fact := range facts.LengthRelations() {
		h = internal.HashCombine(h, internal.FnvString("lenrel"))
		h = hashBoundaryPath(h, fact.Target)
		h = hashBoundaryPath(h, fact.Source)
	}
	for _, fact := range facts.IndexWrites() {
		h = internal.HashCombine(h, internal.FnvString("iw"))
		h = hashBoundaryPath(h, fact.Table)
		h = hashBoundaryPath(h, fact.Key)
		h = internal.HashCombine(h, fact.Value.Hash())
	}
	return h
}

func hashBoundaryPath(h uint64, path flow.BoundaryPath) uint64 {
	h = internal.HashCombine(h, uint64(path.Kind))
	h = internal.HashCombine(h, uint64(path.Index+1))
	h = internal.HashCombine(h, internal.FnvString(constraint.FormatSegments(path.Segments)))
	return h
}

func mergeEntryFacts(a, b flow.BoundaryFacts) flow.BoundaryFacts {
	return flow.MergeBoundaryFactProofs(a.Clone(), b.Clone())
}
