package flow

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// BoundaryPathKind identifies a caller-visible function boundary root. Boundary
// facts are projected from point-local facts by rebasing symbols to either a
// runtime parameter slot or a return slot.
type BoundaryPathKind uint8

const (
	BoundaryPathParam BoundaryPathKind = iota + 1
	BoundaryPathReturn
)

// BoundaryPath is a canonical path relative to a function boundary root.
type BoundaryPath struct {
	Kind     BoundaryPathKind
	Index    int
	Segments []constraint.Segment
}

// BoundaryKeyPresenceFact proves Table[Key] is present after the call returns.
type BoundaryKeyPresenceFact struct {
	Table BoundaryPath
	Key   BoundaryPath
}

// BoundaryKeyArrayFact proves every current element of Array is a key of Table
// after the call returns.
type BoundaryKeyArrayFact struct {
	Array BoundaryPath
	Table BoundaryPath
}

// BoundaryKeyArrayValueFact proves every current element of Array is a key of
// Table and Table[element] has Value after the call returns.
type BoundaryKeyArrayValueFact struct {
	Array BoundaryPath
	Table BoundaryPath
	Value product.AbstractValue
}

// BoundaryAppendKeyFact proves that Key was appended to Array during the call.
// When HasTable is true, the append is tied to a table whose key-array relation
// was being preserved. Without HasTable, callers may materialize a table
// relation only from their own fresh-empty array state.
type BoundaryAppendKeyFact struct {
	Array    BoundaryPath
	Key      BoundaryPath
	Table    BoundaryPath
	HasTable bool
}

// BoundaryAppendElementFieldOriginFact records that a tracked append to Array
// may have sourced the appended element-relative Field from Source. It is a
// boundary-relative counterpart of AppendElementFieldOriginFact, used to carry
// demand-routing provenance across function calls and returns.
type BoundaryAppendElementFieldOriginFact struct {
	Array       BoundaryPath
	Field       []constraint.Segment
	Source      BoundaryPath
	SourceField []constraint.Segment
}

// BoundaryLengthLowerBound proves len(Target) >= Lower after the call returns.
type BoundaryLengthLowerBound struct {
	Target BoundaryPath
	Lower  int64
}

// BoundaryIndexWriteFact proves Table[Key] has Value after the call returns.
// Table and Key are boundary-relative paths; Value is a product value because the
// fact is symbolic readback, not just key presence.
type BoundaryIndexWriteFact struct {
	Table BoundaryPath
	Key   BoundaryPath
	Value product.AbstractValue
}

// BoundaryFacts is the finite caller-visible postcondition component for facts
// that already exist point-locally but must cross a function boundary. It is a
// must-fact lattice: join keeps only facts proved by every normal return path.
type BoundaryFacts struct {
	bottom         bool
	keyPresence    []BoundaryKeyPresenceFact
	keyArrays      []BoundaryKeyArrayFact
	keyArrayValues []BoundaryKeyArrayValueFact
	appendKeys     []BoundaryAppendKeyFact
	appendOrigins  []BoundaryAppendElementFieldOriginFact
	lenLower       []BoundaryLengthLowerBound
	indexWrites    []BoundaryIndexWriteFact
}

// BoundaryFactsOf builds a canonical finite boundary-fact value.
func BoundaryFactsOf(
	keyPresence []BoundaryKeyPresenceFact,
	keyArrays []BoundaryKeyArrayFact,
	keyArrayValues []BoundaryKeyArrayValueFact,
	appendKeys []BoundaryAppendKeyFact,
	lenLower []BoundaryLengthLowerBound,
	indexWrites []BoundaryIndexWriteFact,
) BoundaryFacts {
	return boundaryFactsOfFull(keyPresence, keyArrays, keyArrayValues, appendKeys, nil, lenLower, indexWrites)
}

func boundaryFactsOfFull(
	keyPresence []BoundaryKeyPresenceFact,
	keyArrays []BoundaryKeyArrayFact,
	keyArrayValues []BoundaryKeyArrayValueFact,
	appendKeys []BoundaryAppendKeyFact,
	appendOrigins []BoundaryAppendElementFieldOriginFact,
	lenLower []BoundaryLengthLowerBound,
	indexWrites []BoundaryIndexWriteFact,
) BoundaryFacts {
	return BoundaryFacts{
		keyPresence:    compactBoundaryKeyPresence(keyPresence),
		keyArrays:      compactBoundaryKeyArrays(keyArrays),
		keyArrayValues: compactBoundaryKeyArrayValues(keyArrayValues),
		appendKeys:     compactBoundaryAppendKeys(appendKeys),
		appendOrigins:  compactBoundaryAppendElementFieldOrigins(appendOrigins),
		lenLower:       compactBoundaryLengthLower(lenLower),
		indexWrites:    compactBoundaryIndexWrites(indexWrites),
	}
}

// WithAppendElementFieldOrigins returns f plus canonical append-field origin
// proofs. The base constructor intentionally stays source-compatible while this
// fact lane stabilizes; reduction can later group all fact lanes behind one
// parts/builder API.
func (f BoundaryFacts) WithAppendElementFieldOrigins(origins []BoundaryAppendElementFieldOriginFact) BoundaryFacts {
	if f.bottom || len(origins) == 0 {
		return f
	}
	return boundaryFactsOfFull(
		f.keyPresence,
		f.keyArrays,
		f.keyArrayValues,
		f.appendKeys,
		append(f.AppendElementFieldOrigins(), origins...),
		f.lenLower,
		f.indexWrites,
	)
}

// BoundaryReturnFactBucket groups finite boundary facts by the return slots
// their paths mention. Summary projection uses these buckets to apply the
// must-fact join only to return points where those slots are actually bound.
type BoundaryReturnFactBucket struct {
	indices []int
	facts   BoundaryFacts
}

// Indices returns the sorted return-slot indices mentioned by the bucket.
func (b BoundaryReturnFactBucket) Indices() []int {
	return append([]int(nil), b.indices...)
}

// Facts returns the finite facts carried by this return-slot bucket.
func (b BoundaryReturnFactBucket) Facts() BoundaryFacts {
	return b.facts
}

// MergeBoundaryFactProofs combines independently-proven finite boundary facts.
// It is a proof builder, not the lattice Join: if two derivations both hold on
// the same path, callers may consume the union of their facts. Top contributes
// no finite proof; Bottom is treated as unreachable/no consumable proof here.
func MergeBoundaryFactProofs(a, b BoundaryFacts) BoundaryFacts {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return boundaryFactsOfFull(
		append(a.KeyPresence(), b.KeyPresence()...),
		append(a.KeyArrays(), b.KeyArrays()...),
		append(a.KeyArrayValues(), b.KeyArrayValues()...),
		append(a.AppendKeys(), b.AppendKeys()...),
		append(a.AppendElementFieldOrigins(), b.AppendElementFieldOrigins()...),
		append(a.LengthLowerBounds(), b.LengthLowerBounds()...),
		append(a.IndexWrites(), b.IndexWrites()...),
	)
}

// BoundaryFactsDomain is the lattice over boundary postconditions.
var BoundaryFactsDomain = lattice.Lattice[BoundaryFacts]{
	Bottom: func() BoundaryFacts { return BoundaryFacts{bottom: true} },
	Top:    func() BoundaryFacts { return BoundaryFacts{} },
	Equal: func(a, b BoundaryFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return boundaryKeyPresenceRowIdentity.Equal(a.keyPresence, b.keyPresence) &&
			boundaryKeyArrayRowIdentity.Equal(a.keyArrays, b.keyArrays) &&
			boundaryKeyArrayValueRowIdentity.EqualBy(a.keyArrayValues, b.keyArrayValues, func(x, y BoundaryKeyArrayValueFact) bool {
				return compareBoundaryKeyArrayValue(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			}) &&
			boundaryAppendKeyRowIdentity.Equal(a.appendKeys, b.appendKeys) &&
			boundaryAppendElementFieldOriginRowIdentity.Equal(a.appendOrigins, b.appendOrigins) &&
			boundaryLengthLowerRowIdentity.Equal(a.lenLower, b.lenLower) &&
			boundaryIndexWriteRowIdentity.EqualBy(a.indexWrites, b.indexWrites, func(x, y BoundaryIndexWriteFact) bool {
				return compareBoundaryIndexWrite(x, y) == 0 && product.Domain.Equal(x.Value, y.Value)
			})
	},
	LessOrEq: func(a, b BoundaryFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return boundaryKeyPresenceContainAll(a.keyPresence, b.keyPresence) &&
			boundaryKeyArraysContainAll(a.keyArrays, b.keyArrays) &&
			boundaryKeyArrayValuesContainAll(a.keyArrayValues, b.keyArrayValues) &&
			boundaryAppendKeysContainAll(a.appendKeys, b.appendKeys) &&
			boundaryAppendElementFieldOriginsContainAll(a.appendOrigins, b.appendOrigins) &&
			boundaryLengthLowerContainAll(a.lenLower, b.lenLower) &&
			boundaryIndexWritesContainAll(a.indexWrites, b.indexWrites)
	},
	Join:  joinBoundaryFacts,
	Meet:  nil,
	Widen: widenBoundaryFacts,
}

func (f BoundaryFacts) IsBottom() bool { return f.bottom }

func (f BoundaryFacts) HasProof() bool {
	return !f.bottom && (len(f.keyPresence) > 0 || len(f.keyArrays) > 0 || len(f.keyArrayValues) > 0 || len(f.appendKeys) > 0 || len(f.appendOrigins) > 0 || len(f.lenLower) > 0 || len(f.indexWrites) > 0)
}

// PartitionByReturnIndices separates parameter-only facts from facts whose
// boundary paths mention return slots. Return-relative facts are grouped by the
// sorted set of return indices they mention, so must-fact projection can reason
// about eligibility per returned value instead of treating all returns as one
// unconditional postcondition stream.
func (f BoundaryFacts) PartitionByReturnIndices() (BoundaryFacts, []BoundaryReturnFactBucket) {
	if f.bottom || !f.HasProof() {
		return f, nil
	}
	var params boundaryFactParts
	buckets := make(map[string]boundaryFactParts)
	bucketIndices := make(map[string][]int)
	addReturnFact := func(indices []int, add func(*boundaryFactParts)) {
		key := boundaryReturnIndicesKey(indices)
		parts := buckets[key]
		add(&parts)
		buckets[key] = parts
		if _, ok := bucketIndices[key]; !ok {
			bucketIndices[key] = append([]int(nil), indices...)
		}
	}
	for _, fact := range f.KeyPresence() {
		indices := boundaryPathReturnIndices(fact.Table, fact.Key)
		if len(indices) == 0 {
			params.keyPresence = append(params.keyPresence, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.keyPresence = append(parts.keyPresence, fact)
		})
	}
	for _, fact := range f.KeyArrays() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Table)
		if len(indices) == 0 {
			params.keyArrays = append(params.keyArrays, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.keyArrays = append(parts.keyArrays, fact)
		})
	}
	for _, fact := range f.KeyArrayValues() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Table)
		if len(indices) == 0 {
			params.keyArrayValues = append(params.keyArrayValues, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.keyArrayValues = append(parts.keyArrayValues, fact)
		})
	}
	for _, fact := range f.AppendKeys() {
		paths := []BoundaryPath{fact.Array, fact.Key}
		if fact.HasTable {
			paths = append(paths, fact.Table)
		}
		indices := boundaryPathReturnIndices(paths...)
		if len(indices) == 0 {
			params.appendKeys = append(params.appendKeys, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.appendKeys = append(parts.appendKeys, fact)
		})
	}
	for _, fact := range f.AppendElementFieldOrigins() {
		indices := boundaryPathReturnIndices(fact.Array, fact.Source)
		if len(indices) == 0 {
			params.appendOrigins = append(params.appendOrigins, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.appendOrigins = append(parts.appendOrigins, fact)
		})
	}
	for _, fact := range f.LengthLowerBounds() {
		indices := boundaryPathReturnIndices(fact.Target)
		if len(indices) == 0 {
			params.lenLower = append(params.lenLower, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.lenLower = append(parts.lenLower, fact)
		})
	}
	for _, fact := range f.IndexWrites() {
		indices := boundaryPathReturnIndices(fact.Table, fact.Key)
		if len(indices) == 0 {
			params.indexWrites = append(params.indexWrites, fact)
			continue
		}
		addReturnFact(indices, func(parts *boundaryFactParts) {
			parts.indexWrites = append(parts.indexWrites, fact)
		})
	}
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]BoundaryReturnFactBucket, 0, len(keys))
	for _, key := range keys {
		out = append(out, BoundaryReturnFactBucket{
			indices: append([]int(nil), bucketIndices[key]...),
			facts:   buckets[key].facts(),
		})
	}
	return params.facts(), out
}

func (f BoundaryFacts) KeyPresence() []BoundaryKeyPresenceFact {
	if f.bottom || len(f.keyPresence) == 0 {
		return nil
	}
	out := make([]BoundaryKeyPresenceFact, 0, len(f.keyPresence))
	for _, fact := range f.keyPresence {
		out = append(out, cloneBoundaryKeyPresence(fact))
	}
	return out
}

func (f BoundaryFacts) KeyArrays() []BoundaryKeyArrayFact {
	if f.bottom || len(f.keyArrays) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayFact, 0, len(f.keyArrays))
	for _, fact := range f.keyArrays {
		out = append(out, cloneBoundaryKeyArray(fact))
	}
	return out
}

func (f BoundaryFacts) KeyArrayValues() []BoundaryKeyArrayValueFact {
	if f.bottom || len(f.keyArrayValues) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayValueFact, 0, len(f.keyArrayValues))
	for _, fact := range f.keyArrayValues {
		out = append(out, cloneBoundaryKeyArrayValue(fact))
	}
	return out
}

func (f BoundaryFacts) AppendKeys() []BoundaryAppendKeyFact {
	if f.bottom || len(f.appendKeys) == 0 {
		return nil
	}
	out := make([]BoundaryAppendKeyFact, 0, len(f.appendKeys))
	for _, fact := range f.appendKeys {
		out = append(out, cloneBoundaryAppendKey(fact))
	}
	return out
}

func (f BoundaryFacts) AppendElementFieldOrigins() []BoundaryAppendElementFieldOriginFact {
	if f.bottom || len(f.appendOrigins) == 0 {
		return nil
	}
	out := make([]BoundaryAppendElementFieldOriginFact, 0, len(f.appendOrigins))
	for _, fact := range f.appendOrigins {
		out = append(out, cloneBoundaryAppendElementFieldOrigin(fact))
	}
	return out
}

func (f BoundaryFacts) LengthLowerBounds() []BoundaryLengthLowerBound {
	if f.bottom || len(f.lenLower) == 0 {
		return nil
	}
	out := make([]BoundaryLengthLowerBound, 0, len(f.lenLower))
	for _, fact := range f.lenLower {
		out = append(out, cloneBoundaryLengthLower(fact))
	}
	return out
}

func (f BoundaryFacts) IndexWrites() []BoundaryIndexWriteFact {
	if f.bottom || len(f.indexWrites) == 0 {
		return nil
	}
	out := make([]BoundaryIndexWriteFact, 0, len(f.indexWrites))
	for _, fact := range f.indexWrites {
		out = append(out, cloneBoundaryIndexWrite(fact))
	}
	return out
}

func (f BoundaryFacts) HasKeyPresence(fact BoundaryKeyPresenceFact) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryKeyPresenceRowIdentity.Find(f.keyPresence, fact)
	return ok
}

func (f BoundaryFacts) HasKeyArray(fact BoundaryKeyArrayFact) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryKeyArrayRowIdentity.Find(f.keyArrays, fact)
	return ok
}

func (f BoundaryFacts) HasLengthLowerBound(fact BoundaryLengthLowerBound) bool {
	if f.bottom {
		return false
	}
	_, ok := boundaryLengthLowerRowIdentity.Find(f.lenLower, fact)
	return ok
}

func (f BoundaryFacts) HasIndexWrite(fact BoundaryIndexWriteFact) bool {
	if f.bottom {
		return false
	}
	idx, ok := boundaryIndexWriteRowIdentity.Find(f.indexWrites, fact)
	return ok && product.Domain.Equal(f.indexWrites[idx].Value, fact.Value)
}

func joinBoundaryFacts(a, b BoundaryFacts) BoundaryFacts {
	return mergeBoundaryFacts(a, b, false)
}

func widenBoundaryFacts(prev, next BoundaryFacts) BoundaryFacts {
	return mergeBoundaryFacts(prev, next, true)
}

func mergeBoundaryFacts(a, b BoundaryFacts, widenPayload bool) BoundaryFacts {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return BoundaryFacts{
		keyPresence:    intersectBoundaryKeyPresence(a.keyPresence, b.keyPresence),
		keyArrays:      intersectBoundaryKeyArrays(a.keyArrays, b.keyArrays),
		keyArrayValues: intersectBoundaryKeyArrayValues(a.keyArrayValues, b.keyArrayValues, widenPayload),
		appendKeys:     intersectBoundaryAppendKeys(a.appendKeys, b.appendKeys),
		appendOrigins:  intersectBoundaryAppendElementFieldOrigins(a.appendOrigins, b.appendOrigins),
		lenLower:       intersectBoundaryLengthLower(a.lenLower, b.lenLower),
		indexWrites:    intersectBoundaryIndexWrites(a.indexWrites, b.indexWrites, widenPayload),
	}
}

type boundaryFactParts struct {
	keyPresence    []BoundaryKeyPresenceFact
	keyArrays      []BoundaryKeyArrayFact
	keyArrayValues []BoundaryKeyArrayValueFact
	appendKeys     []BoundaryAppendKeyFact
	appendOrigins  []BoundaryAppendElementFieldOriginFact
	lenLower       []BoundaryLengthLowerBound
	indexWrites    []BoundaryIndexWriteFact
}

func (p boundaryFactParts) facts() BoundaryFacts {
	return boundaryFactsOfFull(p.keyPresence, p.keyArrays, p.keyArrayValues, p.appendKeys, p.appendOrigins, p.lenLower, p.indexWrites)
}

func boundaryPathReturnIndices(paths ...BoundaryPath) []int {
	var out []int
	for _, path := range paths {
		if path.Kind == BoundaryPathReturn && path.Index >= 0 {
			out = append(out, path.Index)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func boundaryReturnIndicesKey(indices []int) string {
	var out []byte
	for i, idx := range indices {
		if i > 0 {
			out = append(out, ',')
		}
		out = appendBoundaryIndexKey(out, idx)
	}
	return string(out)
}

func appendBoundaryIndexKey(out []byte, idx int) []byte {
	if idx == 0 {
		return append(out, '0')
	}
	var buf [20]byte
	i := len(buf)
	for idx > 0 {
		i--
		buf[i] = byte('0' + idx%10)
		idx /= 10
	}
	return append(out, buf[i:]...)
}

func compactBoundaryKeyPresence(xs []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyPresenceFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Table) || !validBoundaryPath(fact.Key) {
			continue
		}
		out = append(out, cloneBoundaryKeyPresence(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyPresence)
	return slices.CompactFunc(out, func(a, b BoundaryKeyPresenceFact) bool {
		return compareBoundaryKeyPresence(a, b) == 0
	})
}

func compactBoundaryKeyArrays(xs []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryKeyArray(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArray)
	return slices.CompactFunc(out, func(a, b BoundaryKeyArrayFact) bool {
		return compareBoundaryKeyArray(a, b) == 0
	})
}

func compactBoundaryKeyArrayValues(xs []BoundaryKeyArrayValueFact) []BoundaryKeyArrayValueFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryKeyArrayValueFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Table) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryKeyArrayValue(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryKeyArrayValue)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryKeyArrayValue(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryKeyArrayValueFact(nil), dst...)
}

func compactBoundaryAppendKeys(xs []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendKeyFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || !validBoundaryPath(fact.Key) {
			continue
		}
		if fact.HasTable && !validBoundaryPath(fact.Table) {
			continue
		}
		out = append(out, cloneBoundaryAppendKey(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendKey)
	return slices.CompactFunc(out, func(a, b BoundaryAppendKeyFact) bool {
		return compareBoundaryAppendKey(a, b) == 0
	})
}

func compactBoundaryAppendElementFieldOrigins(xs []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryAppendElementFieldOriginFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Array) || len(fact.Field) == 0 || !validBoundaryPath(fact.Source) {
			continue
		}
		out = append(out, cloneBoundaryAppendElementFieldOrigin(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryAppendElementFieldOrigin)
	return slices.CompactFunc(out, func(a, b BoundaryAppendElementFieldOriginFact) bool {
		return compareBoundaryAppendElementFieldOrigin(a, b) == 0
	})
}

func compactBoundaryLengthLower(xs []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryLengthLowerBound, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Target) || fact.Lower <= 0 {
			continue
		}
		out = append(out, cloneBoundaryLengthLower(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryLengthLower)
	return slices.CompactFunc(out, func(a, b BoundaryLengthLowerBound) bool {
		return compareBoundaryLengthLower(a, b) == 0
	})
}

func compactBoundaryIndexWrites(xs []BoundaryIndexWriteFact) []BoundaryIndexWriteFact {
	if len(xs) == 0 {
		return nil
	}
	out := make([]BoundaryIndexWriteFact, 0, len(xs))
	for _, fact := range xs {
		if !validBoundaryPath(fact.Table) || !validBoundaryPath(fact.Key) || fact.Value.IsZero() {
			continue
		}
		out = append(out, cloneBoundaryIndexWrite(fact))
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, compareBoundaryIndexWrite)
	dst := out[:0]
	for _, fact := range out {
		if len(dst) > 0 && compareBoundaryIndexWrite(dst[len(dst)-1], fact) == 0 {
			dst[len(dst)-1].Value = product.Domain.Join(dst[len(dst)-1].Value, fact.Value)
			continue
		}
		dst = append(dst, fact)
	}
	return append([]BoundaryIndexWriteFact(nil), dst...)
}

func validBoundaryPath(p BoundaryPath) bool {
	return (p.Kind == BoundaryPathParam || p.Kind == BoundaryPathReturn) && p.Index >= 0
}

func cloneBoundaryPath(p BoundaryPath) BoundaryPath {
	if len(p.Segments) == 0 {
		p.Segments = nil
		return p
	}
	p.Segments = append([]constraint.Segment(nil), p.Segments...)
	return p
}

func cloneBoundaryKeyPresence(f BoundaryKeyPresenceFact) BoundaryKeyPresenceFact {
	return BoundaryKeyPresenceFact{
		Table: cloneBoundaryPath(f.Table),
		Key:   cloneBoundaryPath(f.Key),
	}
}

func cloneBoundaryKeyArray(f BoundaryKeyArrayFact) BoundaryKeyArrayFact {
	return BoundaryKeyArrayFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
	}
}

func cloneBoundaryKeyArrayValue(f BoundaryKeyArrayValueFact) BoundaryKeyArrayValueFact {
	return BoundaryKeyArrayValueFact{
		Array: cloneBoundaryPath(f.Array),
		Table: cloneBoundaryPath(f.Table),
		Value: f.Value,
	}
}

func cloneBoundaryAppendKey(f BoundaryAppendKeyFact) BoundaryAppendKeyFact {
	return BoundaryAppendKeyFact{
		Array:    cloneBoundaryPath(f.Array),
		Key:      cloneBoundaryPath(f.Key),
		Table:    cloneBoundaryPath(f.Table),
		HasTable: f.HasTable,
	}
}

func cloneBoundaryAppendElementFieldOrigin(f BoundaryAppendElementFieldOriginFact) BoundaryAppendElementFieldOriginFact {
	return BoundaryAppendElementFieldOriginFact{
		Array:       cloneBoundaryPath(f.Array),
		Field:       append([]constraint.Segment(nil), f.Field...),
		Source:      cloneBoundaryPath(f.Source),
		SourceField: append([]constraint.Segment(nil), f.SourceField...),
	}
}

func cloneBoundaryLengthLower(f BoundaryLengthLowerBound) BoundaryLengthLowerBound {
	return BoundaryLengthLowerBound{
		Target: cloneBoundaryPath(f.Target),
		Lower:  f.Lower,
	}
}

func cloneBoundaryIndexWrite(f BoundaryIndexWriteFact) BoundaryIndexWriteFact {
	return BoundaryIndexWriteFact{
		Table: cloneBoundaryPath(f.Table),
		Key:   cloneBoundaryPath(f.Key),
		Value: f.Value,
	}
}

func compareBoundaryKeyPresence(a, b BoundaryKeyPresenceFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryKeyArray(a, b BoundaryKeyArrayFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryKeyArrayValue(a, b BoundaryKeyArrayValueFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendKey(a, b BoundaryAppendKeyFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Key, b.Key); c != 0 {
		return c
	}
	if a.HasTable != b.HasTable {
		if !a.HasTable {
			return -1
		}
		return 1
	}
	if !a.HasTable {
		return 0
	}
	return compareBoundaryPath(a.Table, b.Table)
}

func compareBoundaryAppendElementFieldOrigin(a, b BoundaryAppendElementFieldOriginFact) int {
	if c := compareBoundaryPath(a.Array, b.Array); c != 0 {
		return c
	}
	if c := compareConstraintSegments(a.Field, b.Field); c != 0 {
		return c
	}
	if c := compareBoundaryPath(a.Source, b.Source); c != 0 {
		return c
	}
	return compareConstraintSegments(a.SourceField, b.SourceField)
}

func compareBoundaryLengthLower(a, b BoundaryLengthLowerBound) int {
	if c := compareBoundaryPath(a.Target, b.Target); c != 0 {
		return c
	}
	return cmp.Compare(a.Lower, b.Lower)
}

func compareBoundaryIndexWrite(a, b BoundaryIndexWriteFact) int {
	if c := compareBoundaryPath(a.Table, b.Table); c != 0 {
		return c
	}
	return compareBoundaryPath(a.Key, b.Key)
}

func compareBoundaryPath(a, b BoundaryPath) int {
	if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Index, b.Index); c != 0 {
		return c
	}
	return compareConstraintSegments(a.Segments, b.Segments)
}

var (
	boundaryKeyPresenceRowIdentity = orderedRowIdentity[BoundaryKeyPresenceFact]{
		less: func(a, b BoundaryKeyPresenceFact) bool { return compareBoundaryKeyPresence(a, b) < 0 },
		same: func(a, b BoundaryKeyPresenceFact) bool {
			return compareBoundaryKeyPresence(a, b) == 0
		},
	}
	boundaryKeyArrayRowIdentity = orderedRowIdentity[BoundaryKeyArrayFact]{
		less: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayFact) bool { return compareBoundaryKeyArray(a, b) == 0 },
	}
	boundaryKeyArrayValueRowIdentity = orderedRowIdentity[BoundaryKeyArrayValueFact]{
		less: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) < 0 },
		same: func(a, b BoundaryKeyArrayValueFact) bool { return compareBoundaryKeyArrayValue(a, b) == 0 },
	}
	boundaryAppendKeyRowIdentity = orderedRowIdentity[BoundaryAppendKeyFact]{
		less: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) < 0 },
		same: func(a, b BoundaryAppendKeyFact) bool { return compareBoundaryAppendKey(a, b) == 0 },
	}
	boundaryAppendElementFieldOriginRowIdentity = orderedRowIdentity[BoundaryAppendElementFieldOriginFact]{
		less: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) < 0
		},
		same: func(a, b BoundaryAppendElementFieldOriginFact) bool {
			return compareBoundaryAppendElementFieldOrigin(a, b) == 0
		},
	}
	boundaryLengthLowerRowIdentity = orderedRowIdentity[BoundaryLengthLowerBound]{
		less: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) < 0 },
		same: func(a, b BoundaryLengthLowerBound) bool { return compareBoundaryLengthLower(a, b) == 0 },
	}
	boundaryIndexWriteRowIdentity = orderedRowIdentity[BoundaryIndexWriteFact]{
		less: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) < 0 },
		same: func(a, b BoundaryIndexWriteFact) bool { return compareBoundaryIndexWrite(a, b) == 0 },
	}
)

func boundaryKeyPresenceContainAll(have, want []BoundaryKeyPresenceFact) bool {
	return boundaryKeyPresenceRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArraysContainAll(have, want []BoundaryKeyArrayFact) bool {
	return boundaryKeyArrayRowIdentity.ContainsAll(have, want)
}

func boundaryKeyArrayValuesContainAll(have, want []BoundaryKeyArrayValueFact) bool {
	return boundaryKeyArrayValueRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryKeyArrayValueFact) bool {
		return compareBoundaryKeyArrayValue(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func boundaryAppendKeysContainAll(have, want []BoundaryAppendKeyFact) bool {
	return boundaryAppendKeyRowIdentity.ContainsAll(have, want)
}

func boundaryAppendElementFieldOriginsContainAll(have, want []BoundaryAppendElementFieldOriginFact) bool {
	return boundaryAppendElementFieldOriginRowIdentity.ContainsAll(have, want)
}

func boundaryLengthLowerContainAll(have, want []BoundaryLengthLowerBound) bool {
	return boundaryLengthLowerRowIdentity.ContainsAll(have, want)
}

func boundaryIndexWritesContainAll(have, want []BoundaryIndexWriteFact) bool {
	return boundaryIndexWriteRowIdentity.ContainsAllBy(have, want, func(have, want BoundaryIndexWriteFact) bool {
		return compareBoundaryIndexWrite(have, want) == 0 &&
			product.Domain.LessOrEq(have.Value, want.Value)
	})
}

func intersectBoundaryKeyPresence(a, b []BoundaryKeyPresenceFact) []BoundaryKeyPresenceFact {
	return boundaryKeyPresenceRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyPresenceFact) (BoundaryKeyPresenceFact, bool) {
		return cloneBoundaryKeyPresence(left), true
	})
}

func intersectBoundaryKeyArrays(a, b []BoundaryKeyArrayFact) []BoundaryKeyArrayFact {
	return boundaryKeyArrayRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryKeyArrayFact) (BoundaryKeyArrayFact, bool) {
		return cloneBoundaryKeyArray(left), true
	})
}

func intersectBoundaryKeyArrayValues(a, b []BoundaryKeyArrayValueFact, widenPayload bool) []BoundaryKeyArrayValueFact {
	out := boundaryKeyArrayValueRowIdentity.MergeIntersect(a, b, func(left, right BoundaryKeyArrayValueFact) (BoundaryKeyArrayValueFact, bool) {
		fact := cloneBoundaryKeyArrayValue(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryKeyArrayValues(out)
}

func intersectBoundaryAppendKeys(a, b []BoundaryAppendKeyFact) []BoundaryAppendKeyFact {
	return boundaryAppendKeyRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendKeyFact) (BoundaryAppendKeyFact, bool) {
		return cloneBoundaryAppendKey(left), true
	})
}

func intersectBoundaryAppendElementFieldOrigins(a, b []BoundaryAppendElementFieldOriginFact) []BoundaryAppendElementFieldOriginFact {
	return boundaryAppendElementFieldOriginRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryAppendElementFieldOriginFact) (BoundaryAppendElementFieldOriginFact, bool) {
		return cloneBoundaryAppendElementFieldOrigin(left), true
	})
}

func intersectBoundaryLengthLower(a, b []BoundaryLengthLowerBound) []BoundaryLengthLowerBound {
	return boundaryLengthLowerRowIdentity.MergeIntersect(a, b, func(left, _ BoundaryLengthLowerBound) (BoundaryLengthLowerBound, bool) {
		return cloneBoundaryLengthLower(left), true
	})
}

func intersectBoundaryIndexWrites(a, b []BoundaryIndexWriteFact, widenPayload bool) []BoundaryIndexWriteFact {
	out := boundaryIndexWriteRowIdentity.MergeIntersect(a, b, func(left, right BoundaryIndexWriteFact) (BoundaryIndexWriteFact, bool) {
		fact := cloneBoundaryIndexWrite(left)
		if widenPayload {
			fact.Value = product.Domain.Widen(fact.Value, right.Value)
		} else {
			fact.Value = product.Domain.Join(fact.Value, right.Value)
		}
		return fact, true
	})
	return compactBoundaryIndexWrites(out)
}
