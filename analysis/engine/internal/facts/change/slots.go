package change

import "math/bits"

// Slots is a word bitset over one sealed slot plane. Union is the operation
// the fold needs, so it is word-parallel over a caller-owned backing that
// grows only as far as the highest slot the owner actually touches.
type Slots struct{ words []uint64 }

// Test reports membership.
func (s Slots) Test(i int) bool {
	if i < 0 || i>>6 >= len(s.words) {
		return false
	}
	return s.words[i>>6]&(uint64(1)<<uint(i&63)) != 0
}

// Set adds one slot, growing the backing to reach it.
func (s *Slots) Set(i int) bool {
	if s == nil || i < 0 {
		return false
	}
	word := i >> 6
	if word >= len(s.words) {
		s.words = append(s.words, make([]uint64, word+1-len(s.words))...)
	}
	s.words[word] |= uint64(1) << uint(i&63)
	return true
}

// UnionInto adds every slot of s to dst.
func (s Slots) UnionInto(dst *Slots) bool {
	if dst == nil {
		return false
	}
	if len(s.words) > len(dst.words) {
		dst.words = append(dst.words, make([]uint64, len(s.words)-len(dst.words))...)
	}
	for index, word := range s.words {
		dst.words[index] |= word
	}
	return true
}

// Next returns the lowest member at or above from.
func (s Slots) Next(from int) (int, bool) {
	if from < 0 {
		from = 0
	}
	for word := from >> 6; word < len(s.words); word++ {
		masked := s.words[word]
		if word == from>>6 {
			masked &^= uint64(1)<<uint(from&63) - 1
		}
		if masked != 0 {
			return word<<6 + bits.TrailingZeros64(masked), true
		}
	}
	return 0, false
}

// Empty reports that no slot is a member.
func (s Slots) Empty() bool {
	for _, word := range s.words {
		if word != 0 {
			return false
		}
	}
	return true
}

// Count reports the number of members.
func (s Slots) Count() int {
	total := 0
	for _, word := range s.words {
		total += bits.OnesCount64(word)
	}
	return total
}

// Clear drops every member while keeping the backing for reuse.
func (s *Slots) Clear() {
	if s != nil {
		clear(s.words)
	}
}
