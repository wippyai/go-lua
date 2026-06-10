package relation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/type/kind"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

const recursiveRecordFamilyName = "FlowJoin"

type recursiveRewriteKey struct {
	bodyKind kind.Kind
	bodyPtr  uintptr
	bodyHash uint64
	fromID   uint64
	toID     uint64
}

func (c *productCoalescer) recursiveRewrite(key recursiveRewriteKey) (Type, bool) {
	if c == nil || c.recursiveRewrites == nil {
		return nil, false
	}
	cached, ok := c.recursiveRewrites[key]
	return cached, ok
}

func (c *productCoalescer) cacheRecursiveRewrite(key recursiveRewriteKey, t Type) {
	if c.recursiveRewrites == nil {
		c.recursiveRewrites = make(map[recursiveRewriteKey]Type)
	}
	c.recursiveRewrites[key] = t
}

// CoalesceRecursiveRecordFamilies merges recursive record observations that
// describe the same inferred table family. The recursive wrapper is the
// finite-height representation of one abstract table family; compatible
// observations join into one recursive product with optional/merged body fields
// instead of a growing union of construction histories.
func CoalesceRecursiveRecordFamilies(types []Type) []Type {
	return CoalesceRecursiveRecordFamiliesWithSlotJoin(types, nil)
}

// CoalesceRecursiveRecordFamiliesWithSlotJoin merges recursive record
// observations using slotJoin for nested body slots. A nil slotJoin preserves
// JoinReturnSlot behavior.
func CoalesceRecursiveRecordFamiliesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.product.coalesceRecursiveRecordFamiliesWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

func (c *productCoalescer) coalesceRecursiveRecordFamiliesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	types = state.coalesceFoldedProductFamilyMembers(types)
	if len(types) < 2 {
		return types
	}

	recs := make([]*Recursive, len(types))
	keys := make([]uint64, len(types))
	buckets := make(map[uint64][]int)
	for i, t := range types {
		rec := unaliasRecursive(t)
		if rec == nil {
			continue
		}
		key := state.recursiveRecordCoalesceKey(rec)
		recs[i] = rec
		keys[i] = key
		buckets[key] = append(buckets[key], i)
	}

	used := make([]bool, len(types))
	out := make([]Type, 0, len(types))
	for i, t := range types {
		if used[i] {
			continue
		}
		rec := recs[i]
		if rec == nil {
			out = append(out, t)
			continue
		}

		merged := NewRecursivePlaceholder(recursiveRecordFamilyName)
		body := state.rewriteRecursiveFamilySelf(rec.Body, rec, merged)
		mergedAny := false
		bodyChanged := false
		for _, j := range buckets[keys[i]] {
			if j <= i {
				continue
			}
			if used[j] {
				continue
			}
			next := recs[j]
			if next == nil {
				continue
			}
			nextBody := state.rewriteRecursiveFamilySelf(next.Body, next, merged)
			if !recursiveFamilyBodiesShareAnchor(body, nextBody) {
				continue
			}
			joinedBody, ok := state.joinRecursiveFamilyBodiesWithSlotJoin(body, nextBody, slotJoin)
			if !ok {
				continue
			}
			if !sameRecursiveFamilyBody(body, joinedBody) {
				bodyChanged = true
			}
			body = joinedBody
			mergedAny = true
			used[j] = true
		}
		if !mergedAny || !bodyChanged {
			out = append(out, t)
			continue
		}
		merged.SetBody(body)
		out = append(out, merged)
	}
	return out
}

func (c *productCoalescer) recursiveRecordCoalesceKey(rec *Recursive) uint64 {
	if rec == nil {
		return 0
	}
	body, _ := UnwrapAnnotated(rec.Body).(*Record)
	if body == nil {
		return productFamilyHash(rec)
	}
	h := hash.HashCombine(uint64(kind.Recursive), uint64(kind.Record))
	if body.HasMapComponent() {
		h = hash.HashCombine(h, 1)
	} else {
		h = hash.HashCombine(h, 0)
	}
	tags := c.discriminantDetector().RequiredTags(body)
	if len(tags) == 0 {
		return h
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		h = hash.HashCombine(h, hash.FnvString(key))
		h = hash.HashCombine(h, tags[key])
	}
	return h
}

func sameRecursiveFamilyBody(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if !ContainsRecursive(a) && !ContainsRecursive(b) {
		return false
	}
	return sameProductFamily(a, b)
}

func recursiveFamilyBodiesShareAnchor(a, b Type) bool {
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return false
	}
	i, j := 0, 0
	for i < len(ar.Fields) && j < len(br.Fields) {
		left := ar.Fields[i]
		right := br.Fields[j]
		switch {
		case left.Name == right.Name:
			if (!left.Optional || !right.Optional) && recursiveFamilyAnchorTypesCompatible(left.Type, right.Type) {
				return true
			}
			i++
			j++
		case left.Name < right.Name:
			i++
		default:
			j++
		}
	}
	return false
}

func recursiveFamilyAnchorTypesCompatible(a, b Type) bool {
	if SameNodeOrAcyclicEqual(a, b) {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if ContainsRecursive(a) || ContainsRecursive(b) {
		return sameProductFamily(a, b)
	}
	return TypeEquals(a, b)
}

func (c *productCoalescer) joinRecursiveFamilyBodiesWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	previous := state.recursiveFamilyFold
	state.recursiveFamilyFold = true
	defer func() {
		state.recursiveFamilyFold = previous
	}()
	return state.joinCompatibleRecordsWithSlotJoin(a, b, slotJoin)
}

func unaliasRecursive(t Type) *Recursive {
	for {
		a, ok := t.(*Alias)
		if !ok {
			break
		}
		t = a.Target
	}
	rec, _ := t.(*Recursive)
	return rec
}

func (c *productCoalescer) rewriteRecursiveFamilySelf(t Type, from, to *Recursive) Type {
	if from == nil || to == nil {
		return t
	}
	if from == to {
		return t
	}
	if c == nil {
		c = newProductCoalescer()
	}
	key, ok := recursiveRewriteCacheKey(t, from, to)
	if ok {
		if cached, found := c.recursiveRewrite(key); found {
			return cached
		}
	}
	out := Rewrite(t, func(node Type) (Type, bool) {
		if IsRecursiveRef(node, from) {
			return to, true
		}
		return nil, false
	})
	if ok {
		c.cacheRecursiveRewrite(key, out)
	}
	return out
}

func recursiveRewriteCacheKey(t Type, from, to *Recursive) (recursiveRewriteKey, bool) {
	if t == nil || from == nil || to == nil {
		return recursiveRewriteKey{}, false
	}
	t = UnwrapAnnotated(t)
	if t == nil {
		return recursiveRewriteKey{}, false
	}
	ptr := TypePointer(t)
	key := recursiveRewriteKey{
		bodyKind: t.Kind(),
		bodyPtr:  ptr,
		fromID:   from.ID,
		toID:     to.ID,
	}
	if ptr == 0 {
		key.bodyHash = EqualityHash(t)
	}
	return key, true
}
