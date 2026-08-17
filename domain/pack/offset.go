package pack

import "sort"

// Offset is one schema-issued arbitrary-precision natural. Recurrent Pack
// values retain only this owner-fenced dense handle; canonical limbs remain
// cold schema data. Dynamic or unbounded selection does not intern offsets at
// runtime: it is represented by AnyScalar/AnyTail instead.
type Offset struct {
	owner *algebra
	index uint32
}

func (offset Offset) valid() bool {
	return offset.owner != nil && uint64(offset.index) < uint64(len(offset.owner.offsets))
}

func sameOffset(left, right Offset) bool {
	return left.owner == right.owner && left.valid() && right.valid() && left.index == right.index
}

func compareOffset(left, right Offset) int {
	if sameOffset(left, right) {
		return 0
	}
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return 0
	}
	// freezeOffsets sorts the immutable table by numeric value and removes
	// duplicates. Thus its dense index is the numeric order once the owner
	// fence is established; recurrent comparison must not read cold limbs.
	if left.index < right.index {
		return -1
	}
	return 1
}

// nat is the cold canonical limb representation. Limbs are big-endian,
// zero-free except that zero itself is the one empty sequence. It never enters
// recurrent state or a public Pack term.
type nat struct{ limbs []uint32 }

func natFromUint64(value uint64) nat {
	if value == 0 {
		return nat{}
	}
	limbs := make([]uint32, 0, 2)
	for value != 0 {
		limbs = append(limbs, uint32(value))
		value >>= 32
	}
	for left, right := 0, len(limbs)-1; left < right; left, right = left+1, right-1 {
		limbs[left], limbs[right] = limbs[right], limbs[left]
	}
	return nat{limbs: limbs}
}

func canonicalNat(value nat) (nat, bool) {
	first := 0
	for first < len(value.limbs) && value.limbs[first] == 0 {
		first++
	}
	if first == len(value.limbs) {
		return nat{}, true
	}
	return nat{limbs: append([]uint32(nil), value.limbs[first:]...)}, true
}

func compareNat(left, right nat) int {
	if len(left.limbs) < len(right.limbs) {
		return -1
	}
	if len(left.limbs) > len(right.limbs) {
		return 1
	}
	for index := range left.limbs {
		if left.limbs[index] < right.limbs[index] {
			return -1
		}
		if left.limbs[index] > right.limbs[index] {
			return 1
		}
	}
	return 0
}

func equalNat(left, right nat) bool { return compareNat(left, right) == 0 }

func freezeOffsets(input []nat) ([]nat, bool) {
	entries := make([]nat, 0, len(input)+1)
	entries = append(entries, nat{}) // canonical zero is always needed.
	for _, raw := range input {
		value, ok := canonicalNat(raw)
		if !ok {
			return nil, false
		}
		entries = append(entries, value)
	}
	sort.Slice(entries, func(left, right int) bool { return compareNat(entries[left], entries[right]) < 0 })
	kept := entries[:0]
	for _, value := range entries {
		if len(kept) == 0 || !equalNat(kept[len(kept)-1], value) {
			kept = append(kept, value)
		}
	}
	if len(kept) == 0 || !equalNat(kept[0], nat{}) || uint64(len(kept)) > uint64(^uint32(0))+1 {
		return nil, false
	}
	return append([]nat(nil), kept...), true
}

func offsetAt(owner *algebra, index uint32) (Offset, bool) {
	offset := Offset{owner: owner, index: index}
	return offset, offset.valid()
}

func zeroOffset(owner *algebra) (Offset, bool) { return offsetAt(owner, 0) }

// offsetForUint64 returns one already-sealed schema offset.  It never interns
// a dynamic integer: callers whose exact displacement was not declared by
// Schema must fail closed rather than fabricate a recurrent handle.
func offsetForUint64(owner *algebra, value uint64) (Offset, bool) {
	if owner == nil || !owner.valid() {
		return Offset{}, false
	}
	// Schema-generated offsets are normally dense from zero.  Keep that
	// checked direct path hot, while the binary search below preserves exact
	// behavior for a sparse sealed offset table without allocating a nat or
	// retaining a second lookup authority.
	if value < uint64(len(owner.offsets)) {
		index := uint32(value)
		if compareNatUint64(owner.offsets[index], value) == 0 {
			return offsetAt(owner, index)
		}
	}
	low, high := 0, len(owner.offsets)
	for low < high {
		middle := low + (high-low)/2
		if compareNatUint64(owner.offsets[middle], value) < 0 {
			low = middle + 1
			continue
		}
		high = middle
	}
	if low < len(owner.offsets) && compareNatUint64(owner.offsets[low], value) == 0 {
		return offsetAt(owner, uint32(low))
	}
	return Offset{}, false
}

// compareNatUint64 compares one canonical finite offset against a uint64
// without materializing the integer as a temporary nat.
func compareNatUint64(current nat, value uint64) int {
	if len(current.limbs) > 2 {
		return 1
	}
	if value == 0 {
		if len(current.limbs) == 0 {
			return 0
		}
		return 1
	}
	wantLen := 1
	if value > uint64(^uint32(0)) {
		wantLen = 2
	}
	if len(current.limbs) < wantLen {
		return -1
	}
	if len(current.limbs) > wantLen {
		return 1
	}
	if wantLen == 1 {
		currentValue := uint64(current.limbs[0])
		if currentValue < value {
			return -1
		}
		if currentValue > value {
			return 1
		}
		return 0
	}
	currentValue := uint64(current.limbs[0])<<32 | uint64(current.limbs[1])
	if currentValue < value {
		return -1
	}
	if currentValue > value {
		return 1
	}
	return 0
}

// offsetUint64 decodes a sealed offset only while constructing a cold
// selector.  Offsets too large for Program's index space have no TableIndex;
// Pack operations themselves continue to use their dense owner-issued handle.
func offsetUint64(offset Offset) (uint64, bool) {
	if !offset.valid() {
		return 0, false
	}
	limbs := offset.owner.offsets[offset.index].limbs
	if len(limbs) > 2 {
		return 0, false
	}
	var value uint64
	for _, limb := range limbs {
		if value > (^uint64(0) >> 32) {
			return 0, false
		}
		value = value<<32 | uint64(limb)
	}
	return value, true
}

// addOffsets combines two already-sealed offsets only when the exact sum was
// predeclared by Schema.  The cold limb arithmetic performs no interning and
// the returned dense handle remains owner-fenced.
func addOffsets(left, right Offset) (Offset, bool) {
	if !left.valid() || !right.valid() || left.owner != right.owner {
		return Offset{}, false
	}
	if left.index == 0 {
		return right, true
	}
	if right.index == 0 {
		return left, true
	}
	leftNat := left.owner.offsets[left.index]
	rightNat := left.owner.offsets[right.index]
	if leftValue, leftOK := offsetUint64(left); leftOK {
		if rightValue, rightOK := offsetUint64(right); rightOK {
			if rightValue > ^uint64(0)-leftValue {
				return Offset{}, false
			}
			return offsetForUint64(left.owner, leftValue+rightValue)
		}
	}
	low, high := 0, len(left.owner.offsets)
	for low < high {
		middle := low + (high-low)/2
		if compareNatSum(leftNat, rightNat, left.owner.offsets[middle]) > 0 {
			low = middle + 1
			continue
		}
		high = middle
	}
	if low < len(left.owner.offsets) && compareNatSum(leftNat, rightNat, left.owner.offsets[low]) == 0 {
		return offsetAt(left.owner, uint32(low))
	}
	return Offset{}, false
}

func compareNatSum(left, right, target nat) int {
	limit := len(left.limbs)
	if len(right.limbs) > limit {
		limit = len(right.limbs)
	}
	carry := uint64(0)
	for index := 0; index < limit; index++ {
		carry = (natLimbFromRight(left, index) + natLimbFromRight(right, index) + carry) >> 32
	}
	sumLength := limit
	if carry != 0 {
		sumLength++
	}
	if len(target.limbs) < sumLength {
		return 1
	}
	if len(target.limbs) > sumLength {
		return -1
	}
	carry = 0
	comparison := 0
	for index := 0; index < sumLength; index++ {
		var digit uint32
		if index == limit {
			digit = uint32(carry)
		} else {
			sum := natLimbFromRight(left, index) + natLimbFromRight(right, index) + carry
			digit = uint32(sum)
			carry = sum >> 32
		}
		targetDigit := target.limbs[len(target.limbs)-1-index]
		if digit < targetDigit {
			comparison = -1
		} else if digit > targetDigit {
			comparison = 1
		}
	}
	return comparison
}

func natLimbFromRight(value nat, index int) uint64 {
	if index < 0 || index >= len(value.limbs) {
		return 0
	}
	return uint64(value.limbs[len(value.limbs)-1-index])
}

// offsetBytes is cold-only schema/artifact support. It never backs a map key
// or Fact payload; callers receive a copy so the immutable table cannot be
// mutated through an external slice.
func offsetBytes(owner *algebra, offset Offset) ([]byte, bool) {
	if owner == nil || !offset.valid() || offset.owner != owner {
		return nil, false
	}
	limbs := owner.offsets[offset.index].limbs
	bytesValue := make([]byte, len(limbs)*4)
	for index, limb := range limbs {
		at := index * 4
		bytesValue[at] = byte(limb >> 24)
		bytesValue[at+1] = byte(limb >> 16)
		bytesValue[at+2] = byte(limb >> 8)
		bytesValue[at+3] = byte(limb)
	}
	return bytesValue, true
}
