// Package radix stores sealed sparse uint32 relations in a deterministic,
// collision-free radix form. It is deliberately a child of Link: its keys are
// opaque Link or Program identities, never semantic source identities of its
// own.
package radix

import (
	"errors"
	"math/bits"
)

// Index selects one sealed relation in a Store. Zero is invalid.
type Index uint32

// Pair is one exact uint32-to-uint32 relation row. Values, unlike Index, may
// be zero; presence is reported separately by Lookup.
type Pair struct {
	Key, Value uint32
}

var (
	errBuilderSealed = errors.New("radix: builder sealed")
	errPairsOrder    = errors.New("radix: pairs are not strictly ordered")
	errCapacity      = errors.New("radix: capacity overflow")
)

// Builder accumulates sorted, duplicate-free relations. Its zero value is
// ready for use. Once sealed it cannot mutate the Store it returned.
type Builder struct {
	draft *draft
}

// draft is the one construction lease. Builder values copied after their first
// successful mutation share this lease, so sealing through any copy closes all
// of them before the Store is published.
type draft struct {
	store  Store
	sealed bool
}

// Store is an immutable collection of sparse exact relations. It retains no
// maps or hashes. For a relation of m rows it stores m entries, at most m-1
// branch nodes, and at most 2m-2 child references; it never allocates a key
// universe or a relation cross-product. Its zero value has no valid Index.
type Store struct {
	tables   []table
	entries  []entry
	nodes    []node
	children []uint32
	leafBits []uint64
}

type table struct {
	root     uint32
	rootLeaf bool
	count    uint32
}

// node routes one 4-bit digit. Its live child references are packed in
// children[childStart : childStart+popcount(bitmap)]. A separate bitmap marks
// whether each reference selects an entry rather than another node.
type node struct {
	childStart uint32
	bitmap     uint16
	shift      uint8
}

type entry struct{ key, value uint32 }

type ref struct {
	index uint32
	leaf  bool
}

const (
	wordBits       = 64
	maxKeyBits     = 32
	radixBits      = 4
	maxLookupNodes = maxKeyBits / radixBits
)

// AddSorted adds one relation. Pairs must be strictly ascending by key; this
// keeps the retained layout canonical without a map, hash seed, or sort pass.
func (b *Builder) AddSorted(pairs []Pair) (Index, error) {
	d, err := b.acquire()
	if err != nil {
		return 0, err
	}
	if d.sealed {
		return 0, errBuilderSealed
	}
	if !strictlyAscending(pairs) {
		return 0, errPairsOrder
	}
	if !d.canAppend(len(pairs)) {
		return 0, errCapacity
	}

	index := Index(len(d.store.tables) + 1)
	base := len(d.store.entries)
	for _, pair := range pairs {
		d.store.entries = append(d.store.entries, entry{key: pair.Key, value: pair.Value})
	}

	row := table{count: uint32(len(pairs))}
	switch len(pairs) {
	case 0:
		// An empty, but valid, relation is useful for callers which have one
		// relation per outer object.
	case 1:
		row.root = uint32(base)
		row.rootLeaf = true
	default:
		root := d.build(base, base+len(pairs))
		row.root, row.rootLeaf = root.index, root.leaf
	}
	d.store.tables = append(d.store.tables, row)
	return index, nil
}

// Seal transfers an immutable Store by value. The returned Store shares no
// mutable future state: the Builder rejects every later AddSorted call.
func (b *Builder) Seal() (Store, error) {
	d, err := b.acquire()
	if err != nil {
		return Store{}, err
	}
	if d.sealed {
		return Store{}, errBuilderSealed
	}
	d.sealed = true
	return d.store, nil
}

// Lookup returns the exact value for key in index. It allocates nothing and
// follows at most eight radix nodes for the full uint32 universe.
func (s Store) Lookup(index Index, key uint32) (uint32, bool) {
	if index == 0 || uint64(index) > uint64(len(s.tables)) {
		return 0, false
	}
	row := s.tables[index-1]
	if row.count == 0 {
		return 0, false
	}
	if row.rootLeaf {
		return s.entry(row.root, key)
	}

	nodeIndex := row.root
	for step := 0; step < maxLookupNodes; step++ {
		if uint64(nodeIndex) >= uint64(len(s.nodes)) {
			return 0, false
		}
		n := s.nodes[nodeIndex]
		if n.shift > maxKeyBits-radixBits || n.shift%radixBits != 0 {
			return 0, false
		}
		digit := uint8(key >> n.shift & 0x0f)
		bit := uint16(1) << digit
		if n.bitmap&bit == 0 {
			return 0, false
		}
		offset := uint64(n.childStart) + uint64(bits.OnesCount16(n.bitmap&(bit-1)))
		if offset >= uint64(len(s.children)) {
			return 0, false
		}
		childAt := uint32(offset)
		child := s.children[childAt]
		if s.childLeaf(childAt) {
			return s.entry(child, key)
		}
		nodeIndex = child
	}
	// A valid compressed 32-bit radix tree cannot exceed eight branches. This
	// is a structural validation bound, never an analysis convergence budget.
	return 0, false
}

func (s Store) entry(index, key uint32) (uint32, bool) {
	if uint64(index) >= uint64(len(s.entries)) {
		return 0, false
	}
	row := s.entries[index]
	if row.key != key {
		return 0, false
	}
	return row.value, true
}

func (s Store) childLeaf(index uint32) bool {
	word := uint64(index) / wordBits
	if word >= uint64(len(s.leafBits)) {
		return false
	}
	bit := uint(index % wordBits)
	return s.leafBits[word]&(uint64(1)<<bit) != 0
}

func strictlyAscending(pairs []Pair) bool {
	for i := 1; i < len(pairs); i++ {
		if pairs[i-1].Key >= pairs[i].Key {
			return false
		}
	}
	return true
}

func (b *Builder) acquire() (*draft, error) {
	if b == nil {
		return nil, errBuilderSealed
	}
	if b.draft == nil {
		b.draft = &draft{}
	}
	return b.draft, nil
}

func (d *draft) canAppend(entryCount int) bool {
	// table.count is uint32 rather than a sentinel-bearing handle: its largest
	// representable nonempty cardinality is MaxUint32, never 2^32.
	if entryCount < 0 || uint64(entryCount) > maxIndexCount || uint64(len(d.store.tables))+1 > maxIndexCount {
		return false
	}
	entries := uint64(entryCount)
	var nodes, children uint64
	if entries > 1 {
		nodes = entries - 1
		children = 2*entries - 2
	}
	if !withinAddressable(len(d.store.entries), entries) ||
		!withinAddressable(len(d.store.nodes), nodes) ||
		!withinAddressable(len(d.store.children), children) {
		return false
	}
	newChildren := uint64(len(d.store.children)) + children
	_, ok := childWordCount(newChildren)
	return ok
}

// build emits a compressed radix subtree over the already-copied sorted
// entries [start,end). The range is nonempty. Its recursion depth is at most
// eight because each call consumes one strictly lower nibble.
func (d *draft) build(start, end int) ref {
	if end-start == 1 {
		return ref{index: uint32(start), leaf: true}
	}
	first, last := d.store.entries[start].key, d.store.entries[end-1].key
	shift := uint8((bits.Len32(first^last) - 1) &^ (radixBits - 1))

	var starts [1 << radixBits]int
	var ends [1 << radixBits]int
	var digits [1 << radixBits]uint8
	groups := 0
	for at := start; at < end; {
		digit := uint8(d.store.entries[at].key>>shift) & 0x0f
		next := at + 1
		for next < end && uint8(d.store.entries[next].key>>shift)&0x0f == digit {
			next++
		}
		starts[groups], ends[groups], digits[groups] = at, next, digit
		groups++
		at = next
	}

	childStart := d.reserveChildren(groups)
	nodeIndex := uint32(len(d.store.nodes))
	n := node{childStart: childStart, shift: shift}
	for i := 0; i < groups; i++ {
		n.bitmap |= uint16(1) << digits[i]
	}
	d.store.nodes = append(d.store.nodes, n)
	for i := 0; i < groups; i++ {
		d.setChild(childStart+uint32(i), d.build(starts[i], ends[i]))
	}
	return ref{index: nodeIndex}
}

func (d *draft) reserveChildren(count int) uint32 {
	start := uint32(len(d.store.children))
	for i := 0; i < count; i++ {
		d.store.children = append(d.store.children, 0)
	}
	words, ok := childWordCount(uint64(len(d.store.children)))
	if !ok {
		// AddSorted proved this before publishing any table. Reaching this
		// branch means an internal invariant was violated; it must not silently
		// under-allocate the leaf bitmap.
		panic("radix: child word capacity invariant")
	}
	for len(d.store.leafBits) < words {
		d.store.leafBits = append(d.store.leafBits, 0)
	}
	return start
}

func (d *draft) setChild(index uint32, child ref) {
	d.store.children[index] = child.index
	word, bit := uint64(index)/wordBits, uint(index%wordBits)
	mask := uint64(1) << bit
	if child.leaf {
		d.store.leafBits[word] |= mask
		return
	}
	d.store.leafBits[word] &^= mask
}

const (
	maxUint32Count = uint64(^uint32(0)) + 1
	maxIndexCount  = uint64(^uint32(0))
	maxInt         = int(^uint(0) >> 1)
)

func withinAddressable(current int, added uint64) bool {
	if current < 0 {
		return false
	}
	total := uint64(current) + added
	return total <= maxUint32Count && total <= uint64(maxInt)
}

// childWordCount returns ceil(children/64) without an overflow-prone
// children+63 intermediate. It proves the result fits int before conversion,
// so the builder never narrows native arithmetic into its retained slices.
func childWordCount(children uint64) (int, bool) {
	words := children / wordBits
	if children%wordBits != 0 {
		words++
	}
	if words > uint64(maxInt) {
		return 0, false
	}
	return int(words), true
}
