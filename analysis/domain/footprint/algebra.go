package footprint

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/keyspace"
)

type Schema struct{ universe *universe }
type universe struct {
	heap      heap.Schema
	id        keyspace.ContentID
	roots     []heap.Key
	rootIndex map[heap.Key]uint32
	potential uint64
}

// NewSchema derives Footprint's complete immutable authority from Heap's
// canonical allocation-key range. Transfer topology is owned by Transfer;
// Footprint never materializes an Application×Target-operation key plane.
func NewSchema(source heap.Schema) (Schema, bool) {
	if !source.Valid() || !source.ContentID().Available() {
		return Schema{}, false
	}
	rootCount := 0
	for index := 0; index < source.KeyCount(); index++ {
		root, ok := source.KeyAt(index)
		if !ok {
			return Schema{}, false
		}
		if root.Kind() == heap.RootAllocation {
			rootCount++
		}
	}
	if rootCount < 0 || uint64(rootCount) > uint64(^uint32(0)) {
		return Schema{}, false
	}
	universe := &universe{
		heap:  source,
		roots: make([]heap.Key, 0, rootCount), rootIndex: make(map[heap.Key]uint32, rootCount),
	}
	for index := 0; index < source.KeyCount(); index++ {
		root, ok := source.KeyAt(index)
		if !ok {
			return Schema{}, false
		}
		if root.Kind() != heap.RootAllocation {
			continue
		}
		if universe.rootIndex[root] != 0 {
			return Schema{}, false
		}
		if _, ok := root.ContentID(); !ok {
			return Schema{}, false
		}
		universe.roots = append(universe.roots, root)
		universe.rootIndex[root] = uint32(len(universe.roots))
	}
	roots := uint64(rootCount)
	if roots != 0 && roots > ^uint64(0)/roots {
		return Schema{}, false
	}
	edges := roots * roots
	if edges == ^uint64(0) || roots > (^uint64(0)-edges-1)/10 {
		return Schema{}, false
	}
	universe.potential = 1 + edges + roots*10
	id := footprintSchemaID(source.ContentID())
	if !id.Available() {
		return Schema{}, false
	}
	universe.id = id
	return Schema{universe: universe}, true
}

func (universe *universe) ownsKey(key Key) bool {
	if universe == nil || key.universe != universe {
		return false
	}
	return key.slot != 0 && uint64(key.slot) <= uint64(len(universe.roots))
}

func (universe *universe) rootAt(coordinate uint32) (heap.Key, bool) {
	if universe == nil || coordinate == 0 || uint64(coordinate) > uint64(len(universe.roots)) {
		return heap.Key{}, false
	}
	return universe.roots[coordinate-1], true
}

func (s Schema) Valid() bool {
	return s.universe != nil && s.universe.id.Available() && s.universe.heap.Valid()
}
func (s Schema) ContentID() keyspace.ContentID {
	if !s.Valid() {
		return keyspace.ContentID{}
	}
	return s.universe.id
}
func (s Schema) LinkContentID() keyspace.ContentID {
	if !s.Valid() {
		return keyspace.ContentID{}
	}
	return s.universe.heap.LinkContentID()
}

// KeyCount is the complete finite structural Footprint observation range.
// The Heap-derived order is exposed only so the owner child can assign
// private carrier selectors during Factor declaration.
func (s Schema) KeyCount() int {
	if !s.Valid() {
		return 0
	}
	return len(s.universe.roots)
}

// KeyAt returns one universe-owned dense handle for an exact structural root.
func (s Schema) KeyAt(index int) (Key, bool) {
	if !s.Valid() || index < 0 || index >= len(s.universe.roots) || uint64(index) >= uint64(^uint32(0)) {
		return Key{}, false
	}
	return Key{universe: s.universe, slot: uint32(index + 1)}, true
}

// Rebind reconstructs only the cold family declaration. Existing Values stay
// fenced to their original owner.
func (s Schema) Rebind(source heap.Schema) (Schema, bool) {
	if !s.Valid() || !source.Valid() || source.ContentID() != s.universe.heap.ContentID() {
		return Schema{}, false
	}
	rebound, ok := NewSchema(source)
	return rebound, ok && rebound.ContentID() == s.ContentID()
}

// Algebra is the one homogeneous lattice instance for every Footprint key.
type Algebra struct{ schema Schema }

func NewAlgebra(schema Schema) (Algebra, bool) {
	if !schema.Valid() {
		return Algebra{}, false
	}
	return Algebra{schema: schema}, true
}
func (a Algebra) Default() Value {
	if !a.valid() {
		return Value{}
	}
	return bottom(a.schema.universe)
}
func (a Algebra) Top() Value {
	if !a.valid() {
		return Value{}
	}
	return top(a.schema.universe)
}

func (a Algebra) Of(key Key, nodes []Node, edges []Edge) (Value, bool) {
	if !a.valid() || !a.schema.universe.ownsKey(key) {
		return Value{}, false
	}
	value, ok := normalize(a.schema.universe, nodes, edges)
	return value, ok && a.accepts(value)
}

// Admits is the O(1) State-family fence used at the Factor boundary. Every
// nonzero Value can only be constructed by this package after normalization;
// recurrent operations preserve that representation invariant.
func (a Algebra) Admits(value Value) bool { return a.accepts(value) }

func (a Algebra) Equal(left, right Value) bool {
	return a.accepts(left) && a.accepts(right) && equalValue(left, right)
}
func (a Algebra) Same(left, right Value) bool { return a.Equal(left, right) }
func (a Algebra) LessOrEq(left, right Value) bool {
	return a.accepts(left) && a.accepts(right) && lessValue(left, right)
}
func (a Algebra) Join(left, right Value) Value {
	if !a.accepts(left) || !a.accepts(right) {
		return Value{}
	}
	return joinValue(left, right)
}
func (a Algebra) Meet(left, right Value) Value {
	if !a.accepts(left) || !a.accepts(right) {
		return Value{}
	}
	return meetValue(left, right)
}

func (a Algebra) Widen(previous, next Value) Value {
	if !a.accepts(previous) || !a.accepts(next) {
		return Value{}
	}
	joined := joinValue(previous, next)
	if equalValue(previous, joined) || previous.IsBottom() || previous.IsTop() || joined.IsTop() {
		return joined
	}
	result := joined
	for i := range result.nodes {
		prior, ok := nodeAt(previous.nodes, result.nodes[i].root)
		if !ok {
			continue
		}
		result.nodes[i].Objects = widenBound(prior.Objects, result.nodes[i].Objects)
		result.nodes[i].Elements = widenBound(prior.Elements, result.nodes[i].Elements)
		result.nodes[i].Capacity = widenBound(prior.Capacity, result.nodes[i].Capacity)
	}
	return result
}

func (a Algebra) WidenRank(key Key, value Value, component int) uint64 {
	if component != 0 || !a.schema.universe.ownsKey(key) || !a.accepts(value) || value.IsTop() {
		return 0
	}
	rank := a.schema.universe.potential - uint64(len(value.edges)) - uint64(len(value.nodes))*10
	for _, stored := range value.nodes {
		rank += boundPrecision(stored.Objects) + boundPrecision(stored.Elements) + boundPrecision(stored.Capacity)
	}
	return rank
}

func boundPrecision(bound Bound) uint64 {
	switch bound.kind {
	case BoundExact, BoundRange:
		return 3
	case BoundOverflow:
		return 2
	case BoundUnbounded:
		return 1
	default:
		return 0
	}
}

func (a Algebra) Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{Bottom: a.Default, Top: a.Top, Equal: a.Equal, Same: a.Same, LessOrEq: a.LessOrEq, Join: a.Join, Meet: a.Meet, Widen: a.Widen}
}

func (a Algebra) Fingerprint(value Value) uint64 {
	if !a.accepts(value) {
		return 0
	}
	hash := uint64(0x46_4f_4f_54)
	for _, word := range a.schema.universe.id {
		hash = internal.MixHash(hash, uint64(word))
	}
	if value.unknown {
		return internal.MixHash(hash, 1)
	}
	for _, stored := range value.nodes {
		hash = internal.MixHash(hash, uint64(stored.root))
		hash = fingerprintBound(hash, stored.Objects)
		hash = fingerprintBound(hash, stored.Elements)
		hash = fingerprintBound(hash, stored.Capacity)
	}
	for _, stored := range value.edges {
		hash = internal.MixHash(hash, uint64(stored.from))
		hash = internal.MixHash(hash, uint64(stored.to))
	}
	return hash
}

func fingerprintBound(hash uint64, bound Bound) uint64 {
	hash = internal.MixHash(hash, uint64(bound.kind))
	hash = internal.MixHash(hash, bound.lower)
	return internal.MixHash(hash, bound.upper)
}
func (a Algebra) valid() bool { return a.schema.Valid() }

func (a Algebra) accepts(value Value) bool {
	return a.valid() && value.universe == a.schema.universe
}

func footprintSchemaID(linkID keyspace.ContentID) keyspace.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte("footprint-schema-v3"))
	_, _ = hash.Write(linkID[:])
	writeFootprintWord(hash, 1)
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func writeFootprintWord(dst interface{ Write([]byte) (int, error) }, value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	_, _ = dst.Write(data[:])
}
