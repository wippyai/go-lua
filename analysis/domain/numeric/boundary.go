package numeric

import (
	"cmp"
	"slices"
)

// EligibilityAt constructs one exact key-local eligibility contribution. It
// accepts only a subset of the atom's already sealed base eligibility: a
// boundary cannot invent a runtime kind or silently clip an unavailable one.
// No pair is created by this contribution.
func (algebra *Algebra) EligibilityAt(key Key, atom Atom, mask Eligibility) (Value, bool) {
	if !algebra.validKey(key) || atom.owner != algebra || !mask.Valid() {
		return Value{}, false
	}
	index, ok := algebra.atomIndex[atom]
	if !ok || mask&^algebra.baseEligibility(index) != 0 {
		return Value{}, false
	}
	return algebra.admitImage(key, Value{algebra: algebra, masks: []atomFact{{slot: atom.slot, mask: mask}}})
}

const invalidBoundImage = ^uint16(0)

// Substitution is a cold, precompiled Numeric boundary transport. Rules may
// apply it repeatedly without rebuilding coordinate maps or consulting the
// Algebra's hash indexes. Nil key/atom images denote identity; non-nil images
// are dense owner-local slot tables. Pair and exact threshold images are
// resolved once from the atom image during construction.
type Substitution struct {
	algebra      *Algebra
	keys         []uint32
	atoms        []uint32
	pairs        []uint32
	boundOffsets []int
	boundLevels  []uint16
}

// IdentitySubstitution returns the total identity transport for one sealed
// Algebra. It intentionally allocates no lookup table.
func IdentitySubstitution(algebra *Algebra) (Substitution, bool) {
	if algebra == nil || !algebra.Valid() {
		return Substitution{}, false
	}
	return Substitution{algebra: algebra}, true
}

// NewSubstitution compiles one finite key/atom transport in an existing
// Algebra. Source coordinates may have one image each; non-injective images
// are valid and use the carrier's join abstraction during Substitute.
//
// Pair identities are never accepted from callers. The constructor derives
// every possible pair and exact threshold image from the sealed Algebra once.
// A missing pair or threshold remains an explicit invalid image and rejects a
// Value only when that Value actually mentions the unavailable relation.
func NewSubstitution(algebra *Algebra, keyPairs [][2]Key, atomPairs [][2]Atom) (Substitution, bool) {
	identity, ok := IdentitySubstitution(algebra)
	if !ok {
		return Substitution{}, false
	}
	if len(keyPairs) == 0 && len(atomPairs) == 0 {
		return identity, true
	}

	var keys []uint32
	if len(keyPairs) != 0 {
		keys = make([]uint32, len(algebra.keys))
		for _, pair := range keyPairs {
			if !algebra.validKey(pair[0]) || !algebra.validKey(pair[1]) || keys[pair[0].slot-1] != 0 {
				return Substitution{}, false
			}
			keys[pair[0].slot-1] = pair[1].slot
		}
		for index := range keys {
			if keys[index] == 0 {
				keys[index] = uint32(index + 1)
			}
		}
	}

	var atoms []uint32
	if len(atomPairs) != 0 {
		atoms = make([]uint32, len(algebra.atoms))
		for _, pair := range atomPairs {
			if pair[0].owner != algebra || pair[1].owner != algebra || !pair[0].Valid() || !pair[1].Valid() || atoms[pair[0].slot-1] != 0 {
				return Substitution{}, false
			}
			atoms[pair[0].slot-1] = pair[1].slot
		}
		for index := range atoms {
			if atoms[index] == 0 {
				atoms[index] = uint32(index + 1)
			}
		}
	}
	return compileSubstitution(algebra, keys, atoms)
}

// compileSubstitution takes ownership of dense, already owner-validated key
// and atom images. It canonicalizes identity and seals all derived images.
func compileSubstitution(algebra *Algebra, keys, atoms []uint32) (Substitution, bool) {
	if algebra == nil || !algebra.Valid() || len(keys) != 0 && len(keys) != len(algebra.keys) || len(atoms) != 0 && len(atoms) != len(algebra.atoms) {
		return Substitution{}, false
	}
	keys = canonicalIdentityImage(keys)
	atoms = canonicalIdentityImage(atoms)
	result := Substitution{algebra: algebra, keys: keys, atoms: atoms}
	if atoms == nil {
		return result, true
	}

	result.pairs = make([]uint32, len(algebra.pairs))
	result.boundOffsets = make([]int, len(algebra.pairs)+1)
	totalLevels := 0
	for index, thresholds := range algebra.thresholds {
		totalLevels += len(thresholds)
		result.boundOffsets[index+1] = totalLevels
	}
	result.boundLevels = make([]uint16, totalLevels)
	for pairOrdinal, row := range algebra.pairs {
		left := algebra.atoms[atoms[row.left.slot-1]-1]
		right := algebra.atoms[atoms[row.right.slot-1]-1]
		image, ok := algebra.pairIndex[pairIndex{left: left, right: right}]
		if !ok {
			continue
		}
		result.pairs[pairOrdinal] = uint32(image + 1)
		for level, threshold := range algebra.thresholds[pairOrdinal] {
			targetLevel, exact := algebra.exactLevel(image, threshold)
			if !exact {
				result.boundLevels[result.boundOffsets[pairOrdinal]+level] = invalidBoundImage
				continue
			}
			result.boundLevels[result.boundOffsets[pairOrdinal]+level] = targetLevel
		}
	}
	return result, true
}

func canonicalIdentityImage(image []uint32) []uint32 {
	for index, slot := range image {
		if slot != uint32(index+1) {
			return image
		}
	}
	return nil
}

func (substitution Substitution) valid() bool {
	if substitution.algebra == nil || !substitution.algebra.Valid() ||
		len(substitution.keys) != 0 && len(substitution.keys) != len(substitution.algebra.keys) ||
		len(substitution.atoms) != 0 && len(substitution.atoms) != len(substitution.algebra.atoms) {
		return false
	}
	if substitution.atoms == nil {
		return substitution.pairs == nil && substitution.boundOffsets == nil && substitution.boundLevels == nil
	}
	return len(substitution.pairs) == len(substitution.algebra.pairs) &&
		len(substitution.boundOffsets) == len(substitution.algebra.pairs)+1
}

// Key returns the exact mapped key, or the original valid key when it is not
// named by this transport. The compiled slot lookup is allocation-free.
func (substitution Substitution) Key(key Key) (Key, bool) {
	if !substitution.valid() || !substitution.algebra.validKey(key) {
		return Key{}, false
	}
	if substitution.keys == nil {
		return key, true
	}
	return Key{owner: substitution.algebra, slot: substitution.keys[key.slot-1]}, true
}

// Atom returns the exact mapped atom, or the original valid atom when it is
// not named by this transport. It cannot cross Algebra ownership.
func (substitution Substitution) Atom(atom Atom) (Atom, bool) {
	if !substitution.valid() || atom.owner != substitution.algebra || !atom.Valid() {
		return Atom{}, false
	}
	if substitution.atoms == nil {
		return atom, true
	}
	return substitution.algebra.atoms[substitution.atoms[atom.slot-1]-1], true
}

// Compose returns next after substitution. Composition is a cold image-build
// operation; the result is compiled once before any Rule can apply it.
func (substitution Substitution) Compose(next Substitution) (Substitution, bool) {
	if !substitution.valid() || !next.valid() || substitution.algebra != next.algebra {
		return Substitution{}, false
	}
	algebra := substitution.algebra
	var keys []uint32
	if substitution.keys != nil || next.keys != nil {
		keys = make([]uint32, len(algebra.keys))
		for index := range keys {
			middle := uint32(index + 1)
			if substitution.keys != nil {
				middle = substitution.keys[index]
			}
			if next.keys != nil {
				middle = next.keys[middle-1]
			}
			keys[index] = middle
		}
	}
	var atoms []uint32
	if substitution.atoms != nil || next.atoms != nil {
		atoms = make([]uint32, len(algebra.atoms))
		for index := range atoms {
			middle := uint32(index + 1)
			if substitution.atoms != nil {
				middle = substitution.atoms[index]
			}
			if next.atoms != nil {
				middle = next.atoms[middle-1]
			}
			atoms[index] = middle
		}
	}
	return compileSubstitution(algebra, keys, atoms)
}

// Fact is one exact Numeric relation at one Algebra key.
type Fact struct {
	Key   Key
	Value Value
}

// Substitute is the hot application of a cold compiled transport. Atom
// collisions use eligibility union; pair-set collisions deduplicate; bound
// collisions retain the less precise maximum exact threshold level. The only
// per-call slices are the immutable facts owned by the result Value.
func (algebra *Algebra) Substitute(fact Fact, substitution Substitution) (Fact, bool) {
	if algebra == nil || substitution.algebra != algebra || !substitution.valid() || !algebra.validKey(fact.Key) || !algebra.owns(fact.Value) {
		return Fact{}, false
	}
	destination, ok := substitution.Key(fact.Key)
	if !ok {
		return Fact{}, false
	}
	if fact.Value.bottom {
		return Fact{Key: destination, Value: algebra.Bottom()}, true
	}
	if fact.Value.isDefault() {
		return Fact{Key: destination, Value: algebra.Default()}, true
	}
	if substitution.atoms == nil {
		if !algebra.Admits(destination, fact.Value) {
			return Fact{}, false
		}
		return Fact{Key: destination, Value: fact.Value}, true
	}

	value := Value{algebra: algebra}
	if value.masks, ok = algebra.substituteMasks(fact.Value.masks, substitution); !ok {
		return Fact{}, false
	}
	if value.equal, ok = substitutePairSet(fact.Value.equal, substitution); !ok {
		return Fact{}, false
	}
	if value.unequal, ok = substitutePairSet(fact.Value.unequal, substitution); !ok {
		return Fact{}, false
	}
	if value.integral, ok = substitutePairSet(fact.Value.integral, substitution); !ok {
		return Fact{}, false
	}
	if value.bounds, ok = substituteBounds(fact.Value.bounds, substitution); !ok {
		return Fact{}, false
	}
	image, ok := algebra.admitImage(destination, value)
	if !ok {
		return Fact{}, false
	}
	return Fact{Key: destination, Value: image}, true
}

func (algebra *Algebra) substituteMasks(source []atomFact, substitution Substitution) ([]atomFact, bool) {
	if len(source) == 0 {
		return nil, true
	}
	result := make([]atomFact, len(source))
	for index, fact := range source {
		result[index] = atomFact{slot: substitution.atoms[fact.slot-1], mask: fact.mask}
	}
	slices.SortFunc(result, func(left, right atomFact) int { return cmp.Compare(left.slot, right.slot) })
	write := 0
	for _, fact := range result {
		if write != 0 && result[write-1].slot == fact.slot {
			result[write-1].mask |= fact.mask
			continue
		}
		result[write] = fact
		write++
	}
	result = result[:write]
	write = 0
	for _, fact := range result {
		base := algebra.baseEligibility(int(fact.slot - 1))
		if !fact.mask.Valid() || fact.mask&^base != 0 {
			return nil, false
		}
		if fact.mask != base {
			result[write] = fact
			write++
		}
	}
	return result[:write], true
}

func substitutePairSet(source []uint32, substitution Substitution) ([]uint32, bool) {
	if len(source) == 0 {
		return nil, true
	}
	result := make([]uint32, len(source))
	for index, slot := range source {
		image := substitution.pairs[slot-1]
		if image == 0 {
			return nil, false
		}
		result[index] = image
	}
	slices.Sort(result)
	return uniqueUint32(result), true
}

func substituteBounds(source []boundFact, substitution Substitution) ([]boundFact, bool) {
	if len(source) == 0 {
		return nil, true
	}
	result := make([]boundFact, len(source))
	for index, fact := range source {
		pairIndex := int(fact.slot - 1)
		image := substitution.pairs[pairIndex]
		levelIndex := substitution.boundOffsets[pairIndex] + int(fact.level)
		if image == 0 || levelIndex < substitution.boundOffsets[pairIndex] || levelIndex >= substitution.boundOffsets[pairIndex+1] {
			return nil, false
		}
		level := substitution.boundLevels[levelIndex]
		if level == invalidBoundImage {
			return nil, false
		}
		result[index] = boundFact{slot: image, level: level}
	}
	slices.SortFunc(result, func(left, right boundFact) int { return cmp.Compare(left.slot, right.slot) })
	write := 0
	for _, fact := range result {
		if write != 0 && result[write-1].slot == fact.slot {
			if fact.level > result[write-1].level {
				result[write-1].level = fact.level
			}
			continue
		}
		result[write] = fact
		write++
	}
	return result[:write], true
}
