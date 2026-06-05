package summary

import (
	"sort"
	"sync"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// EntryValuesKey is an exact comparable key for caller-to-callee product entry
// values. Values are interned by EntryValuesDomain equality, so summary keys are
// stable and Salsa-friendly without degrading the product carrier to strings.
type EntryValuesKey struct {
	n *entryValuesKeyNode
}

type entryValuesKeyNode struct {
	values EntryValues
	hash   uint64
}

type entryValuesKeyInterner struct {
	mu      sync.RWMutex
	buckets map[uint64][]*entryValuesKeyNode
}

var canonicalEntryValuesKeys = &entryValuesKeyInterner{buckets: make(map[uint64][]*entryValuesKeyNode)}

// ResetEntryValuesKeyInterner clears the comparable-key interner for one checker
// analysis scope.
func ResetEntryValuesKeyInterner() {
	canonicalEntryValuesKeys.mu.Lock()
	defer canonicalEntryValuesKeys.mu.Unlock()
	canonicalEntryValuesKeys.buckets = make(map[uint64][]*entryValuesKeyNode)
	ResetEntryFactsKeyInterner()
}

// EntryValuesKeyOf returns an exact comparable key for values.
func EntryValuesKeyOf(values EntryValues) EntryValuesKey {
	return internEntryValuesKey(values)
}

// Values returns the immutable entry-values map represented by k. The zero key
// denotes entryValuesDomain bottom.
func (k EntryValuesKey) Values() EntryValues {
	if k.n == nil {
		return entryValuesDomain.Bottom()
	}
	return entryValuesDomain.Join(k.n.values, nil)
}

func internEntryValuesKey(values EntryValues) EntryValuesKey {
	canonical := entryValuesDomain.Join(values, nil)
	if len(canonical) == 0 {
		return EntryValuesKey{}
	}
	h := entryValuesKeyHash(canonical)

	canonicalEntryValuesKeys.mu.RLock()
	if existing, ok := lookupEntryValuesKey(canonicalEntryValuesKeys.buckets[h], canonical); ok {
		canonicalEntryValuesKeys.mu.RUnlock()
		return EntryValuesKey{n: existing}
	}
	canonicalEntryValuesKeys.mu.RUnlock()

	canonicalEntryValuesKeys.mu.Lock()
	defer canonicalEntryValuesKeys.mu.Unlock()
	if existing, ok := lookupEntryValuesKey(canonicalEntryValuesKeys.buckets[h], canonical); ok {
		return EntryValuesKey{n: existing}
	}
	node := &entryValuesKeyNode{values: cloneEntryValues(canonical), hash: h}
	canonicalEntryValuesKeys.buckets[h] = append(canonicalEntryValuesKeys.buckets[h], node)
	return EntryValuesKey{n: node}
}

func lookupEntryValuesKey(bucket []*entryValuesKeyNode, values EntryValues) (*entryValuesKeyNode, bool) {
	for _, node := range bucket {
		if entryValuesDomain.Equal(node.values, values) {
			return node, true
		}
	}
	return nil, false
}

func entryValuesKeyHash(values EntryValues) uint64 {
	h := internal.FnvString("summary.EntryValuesKey")
	slots := make([]int, 0, len(values))
	for slot := range values {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	for _, slot := range slots {
		av := values[slot]
		if product.Domain.Equal(av, product.Domain.Bottom()) {
			continue
		}
		h = internal.HashCombine(h, uint64(slot+1))
		h = internal.HashCombine(h, av.Hash())
	}
	return h
}

func cloneEntryValues(in EntryValues) EntryValues {
	if len(in) == 0 {
		return nil
	}
	out := make(EntryValues, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
