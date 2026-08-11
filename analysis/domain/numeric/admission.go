package numeric

import (
	"math"
	"math/bits"

	"github.com/wippyai/go-lua/program/keyspace"
)

// Ref returns the cold replay identity of this exact Numeric root.
func (key Key) Ref() (RootRef, bool) {
	if !key.Valid() {
		return RootRef{}, false
	}
	return key.owner.keys[key.slot-1].ref, true
}

// Root returns the exact Program body coordinate owned by this key.
func (key Key) Root() (Root, bool) {
	if !key.Valid() {
		return Root{}, false
	}
	return key.owner.keys[key.slot-1].root, true
}

// Scalar returns the exact Program occurrence for an ordinary atom.
// The algebra's mathematical zero deliberately has no fabricated occurrence.
func (atom Atom) Scalar() (Scalar, bool) {
	if !atom.Valid() || atom.slot == 1 || atom.owner.atomScalars[atom.slot-1] == (Scalar{}) {
		return Scalar{}, false
	}
	return atom.owner.atomScalars[atom.slot-1], true
}

func (pair Pair) Atoms() (Atom, Atom, bool) {
	if !pair.Valid() {
		return Atom{}, Atom{}, false
	}
	row := pair.owner.pairs[pair.slot-1]
	return row.left, row.right, true
}

// FindKey rebinds one exact root identity against this Algebra.
func (algebra *Algebra) FindKey(ref RootRef) (Key, bool) {
	if !algebra.Valid() || ref.LinkID() != algebra.linkID {
		return Key{}, false
	}
	project := algebra.source.Project()
	if project == nil || ref.ShardOrdinal() == 0 {
		return Key{}, false
	}
	shard, ok := project.Mounts().At(int(ref.ShardOrdinal()) - 1)
	if !ok {
		return Key{}, false
	}
	if index, indexOK := project.Mounts().Index(shard); !indexOK || uint64(index+1) != uint64(ref.ShardOrdinal()) {
		return Key{}, false
	}
	key := Key{owner: algebra, slot: algebra.keyIndex[ref]}
	return key, key.Valid()
}

// AtomCount/AtomAt enumerate one key's exact sparse atom support. Zero is
// always first. The support is a sorted selector list, never a Key×Atom bit
// matrix.
func (algebra *Algebra) AtomCount(key Key) int {
	if !algebra.validKey(key) {
		return 0
	}
	return len(algebra.keyAtoms[key.slot-1])
}

func (algebra *Algebra) AtomAt(key Key, index int) (Atom, bool) {
	if !algebra.validKey(key) || index < 0 || index >= len(algebra.keyAtoms[key.slot-1]) {
		return Atom{}, false
	}
	return algebra.atoms[algebra.keyAtoms[key.slot-1][index]-1], true
}

func (algebra *Algebra) PairCount(key Key) int {
	if !algebra.validKey(key) {
		return 0
	}
	return len(algebra.keyPairs[key.slot-1])
}

func (algebra *Algebra) PairAt(key Key, index int) (Pair, bool) {
	if !algebra.validKey(key) || index < 0 || index >= len(algebra.keyPairs[key.slot-1]) {
		return Pair{}, false
	}
	return Pair{owner: algebra, slot: algebra.keyPairs[key.slot-1][index]}, true
}

// Admits proves that a keyless relation mentions only coordinates observable
// at one exact Numeric root. Bottom and the single Default are valid at every
// key; no per-key default image or adapter is constructed.
func (algebra *Algebra) Admits(key Key, value Value) bool {
	if !algebra.validKey(key) || !algebra.owns(value) || value.bottom {
		return algebra.validKey(key) && algebra.owns(value)
	}
	atoms := algebra.keyAtoms[key.slot-1]
	for _, fact := range value.masks {
		if !containsUint32(atoms, fact.slot) {
			return false
		}
	}
	pairs := algebra.keyPairs[key.slot-1]
	for _, slot := range value.equal {
		if !containsUint32(pairs, slot) {
			return false
		}
	}
	for _, slot := range value.unequal {
		if !containsUint32(pairs, slot) {
			return false
		}
	}
	for _, slot := range value.integral {
		if !containsUint32(pairs, slot) {
			return false
		}
	}
	for _, fact := range value.bounds {
		if !containsUint32(pairs, fact.slot) {
			return false
		}
	}
	return true
}

// WidenRank is the key-specific finite lexicographic descent witness. Its
// width is exactly the observable support at the key, not the global Link
// universe. Every strict Join expands one mask, removes one must fact, or
// raises one finite threshold level, hence decreases its first changed rank.
type WidenRank struct {
	algebra *Algebra
	key     Key
}

func (algebra *Algebra) WidenRank(key Key) (WidenRank, bool) {
	if !algebra.validKey(key) {
		return WidenRank{}, false
	}
	return WidenRank{algebra: algebra, key: key}, true
}

func (rank WidenRank) Width() int {
	if rank.algebra == nil || !rank.algebra.validKey(rank.key) {
		return 0
	}
	return len(rank.algebra.keyAtoms[rank.key.slot-1]) + 4*len(rank.algebra.keyPairs[rank.key.slot-1])
}

func (rank WidenRank) At(value Value, component int) (uint64, bool) {
	if rank.algebra == nil || component < 0 || component >= rank.Width() || !rank.algebra.Admits(rank.key, value) {
		return 0, false
	}
	if value.bottom {
		return math.MaxUint64, true
	}
	atoms := rank.algebra.keyAtoms[rank.key.slot-1]
	if component < len(atoms) {
		mask := rank.algebra.mask(value, atoms[component])
		base := rank.algebra.baseEligibility(int(atoms[component] - 1))
		return uint64(bits.OnesCount8(uint8(base &^ mask))), true
	}
	component -= len(atoms)
	pairs := rank.algebra.keyPairs[rank.key.slot-1]
	section, offset := component/len(pairs), component%len(pairs)
	pairIndex := int(pairs[offset] - 1)
	switch section {
	case 0:
		if containsUint32(value.equal, uint32(pairIndex+1)) {
			return 1, true
		}
		return 0, true
	case 1:
		if containsUint32(value.unequal, uint32(pairIndex+1)) {
			return 1, true
		}
		return 0, true
	case 2:
		if containsUint32(value.integral, uint32(pairIndex+1)) {
			return 1, true
		}
		return 0, true
	case 3:
		level := len(rank.algebra.thresholds[pairIndex])
		if finite, present := boundLevel(value.bounds, uint32(pairIndex+1)); present {
			level = int(finite)
		}
		return uint64(len(rank.algebra.thresholds[pairIndex]) - level), true
	default:
		return 0, false
	}
}

// Equivalent is the cold exact replay check. Content identity is only a
// prefilter; all dense tables and sparse key supports are compared.
func (algebra *Algebra) Equivalent(other *Algebra) bool {
	if !algebra.Valid() || !other.Valid() || algebra.content != other.content || algebra.linkID != other.linkID ||
		len(algebra.keys) != len(other.keys) || len(algebra.atomScalars) != len(other.atomScalars) || len(algebra.pairs) != len(other.pairs) {
		return false
	}
	for index := range algebra.keys {
		if algebra.keys[index].ref != other.keys[index].ref ||
			!equalUint32(algebra.keyAtoms[index], other.keyAtoms[index]) ||
			!equalUint32(algebra.keyPairs[index], other.keyPairs[index]) {
			return false
		}
	}
	for index := range algebra.atomScalars {
		if algebra.atomScalarRefs[index] != other.atomScalarRefs[index] || algebra.atomLiterals[index] != other.atomLiterals[index] {
			return false
		}
	}
	for index := range algebra.pairs {
		left, right := algebra.pairs[index], other.pairs[index]
		if left.left.slot != right.left.slot || left.right.slot != right.right.slot || !equalInt64(algebra.thresholds[index], other.thresholds[index]) {
			return false
		}
	}
	return true
}

// Rebind transfers an immutable relation only after exact replay equivalence.
func (algebra *Algebra) Rebind(value Value) (Value, bool) {
	if !value.valid() || !algebra.Equivalent(value.algebra) {
		return Value{}, false
	}
	if value.bottom {
		return algebra.bottom, true
	}
	return Value{
		algebra:  algebra,
		masks:    append([]atomFact(nil), value.masks...),
		equal:    append([]uint32(nil), value.equal...),
		unequal:  append([]uint32(nil), value.unequal...),
		integral: append([]uint32(nil), value.integral...),
		bounds:   append([]boundFact(nil), value.bounds...),
	}, true
}

func (algebra *Algebra) LinkID() keyspace.ContentID {
	if !algebra.Valid() {
		return keyspace.ContentID{}
	}
	return algebra.linkID
}

func equalUint32(left, right []uint32) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalInt64(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
