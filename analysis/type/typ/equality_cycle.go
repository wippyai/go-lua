package typ

import (
	"math/bits"
	"sync"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func needsCycleCheck(k kind.Kind) bool {
	switch k {
	case kind.Union, kind.Intersection, kind.Record, kind.Function,
		kind.Generic, kind.Instantiated, kind.Interface, kind.Recursive,
		kind.TypeParam:
		return true
	}

	return false
}

type typePair struct {
	// Cycle pairs are created only when both typePointer calls returned nonzero;
	// the all-zero value is therefore available as the table's empty sentinel.
	a uintptr
	b uintptr
}

const (
	typePairInlineCapacity  = 32
	typePairMinimumTable    = 64
	typePairLoadNumerator   = 3
	typePairLoadDenominator = 4
)

// typePairSet tracks pairs already visited by recursive structural equality.
// Most type comparisons are shallow and acyclic, so their pairs remain inline.
// Deeper comparisons promote the complete set into an exact open-addressed
// table; after promotion the inline prefix is never scanned again.
type typePairSet struct {
	inline  [typePairInlineCapacity]typePair
	inlineN uint8
	table   *typePairTable
	count   int
}

type typePairTable struct {
	slots []typePair
}

// Only the common bounded scratch sizes are pooled. Larger recursive products
// remain exact but their backing storage is released to the garbage collector.
var typePairTablePools [3]sync.Pool

func typePairPoolIndex(size int) int {
	switch size {
	case 64:
		return 0
	case 128:
		return 1
	case 256:
		return 2
	default:
		return -1
	}
}

func acquireTypePairTable(size int) *typePairTable {
	if index := typePairPoolIndex(size); index >= 0 {
		if pooled := typePairTablePools[index].Get(); pooled != nil {
			return pooled.(*typePairTable)
		}
	}
	return &typePairTable{slots: make([]typePair, size)}
}

func releaseTypePairTable(table *typePairTable) {
	if table == nil {
		return
	}
	if index := typePairPoolIndex(len(table.slots)); index >= 0 {
		clear(table.slots)
		typePairTablePools[index].Put(table)
	}
}

// hashTypePair selects probes only. Membership is always decided by comparing
// the complete ordered pair, so collisions cannot affect equality.
func hashTypePair(pair typePair) uint64 {
	x := uint64(pair.a) ^ bits.RotateLeft64(uint64(pair.b), 31)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// seenOrAdd reports whether pair was already present and otherwise records it.
func (s *typePairSet) seenOrAdd(pair typePair) bool {
	if s.table != nil {
		return s.tableSeenOrAdd(pair)
	}
	for i := range s.inlineN {
		if s.inline[i] == pair {
			return true
		}
	}
	if s.inlineN < uint8(len(s.inline)) {
		s.inline[s.inlineN] = pair
		s.inlineN++
		return false
	}
	s.promote(typePairMinimumTable)
	return s.tableSeenOrAdd(pair)
}

func (s *typePairSet) promote(size int) {
	table := acquireTypePairTable(size)
	for i := range s.inlineN {
		insertTypePair(table, s.inline[i])
	}
	s.table = table
	s.count = int(s.inlineN)
}

func insertTypePair(table *typePairTable, pair typePair) {
	mask := uint64(len(table.slots) - 1)
	for slot := hashTypePair(pair) & mask; ; slot = (slot + 1) & mask {
		if table.slots[slot] == (typePair{}) {
			table.slots[slot] = pair
			return
		}
	}
}

func (s *typePairSet) tableSeenOrAdd(pair typePair) bool {
	for {
		slots := s.table.slots
		mask := uint64(len(slots) - 1)
		for slot := hashTypePair(pair) & mask; ; slot = (slot + 1) & mask {
			existing := slots[slot]
			if existing == pair {
				return true
			}
			if existing != (typePair{}) {
				continue
			}
			if (s.count+1)*typePairLoadDenominator > len(slots)*typePairLoadNumerator {
				s.grow()
				break
			}
			slots[slot] = pair
			s.count++
			return false
		}
	}
}

func (s *typePairSet) grow() {
	old := s.table
	next := acquireTypePairTable(len(old.slots) * 2)
	for _, pair := range old.slots {
		if pair != (typePair{}) {
			insertTypePair(next, pair)
		}
	}
	s.table = next
	releaseTypePairTable(old)
}

func (s *typePairSet) release() {
	releaseTypePairTable(s.table)
	s.table = nil
	s.count = 0
	s.inlineN = 0
}
