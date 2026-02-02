package lua

import (
	"unsafe"
)

// iface is an internal representation of the go-interface.
type iface struct {
	itab unsafe.Pointer
	word unsafe.Pointer
}

const preloadLimit = 256
const intPreloadLimit = 65536 // Common loop bounds

var preloads [preloadLimit]LValue
var intPreloads [intPreloadLimit * 2]LValue // [-65536, 65536)

func init() {
	for i := 0; i < preloadLimit; i++ {
		preloads[i] = LNumber(i)
	}
	for i := -intPreloadLimit; i < intPreloadLimit; i++ {
		intPreloads[i+intPreloadLimit] = LInteger(i)
	}
}

// allocator is a fast bulk memory allocator for the LValue.
// Uses a single backing slice for both float64 and int64 since both are 8 bytes.
type allocator struct {
	size int
	ptrs []uint64 // shared backing store for float64 and int64

	scratchValue   LValue
	scratchValueP  *iface
	scratchIntVal  LValue
	scratchIntValP *iface
}

func newAllocator(size int) *allocator {
	al := &allocator{
		size: size,
		ptrs: nil, // lazy alloc on first use
	}
	al.scratchValue = LNumber(0)
	al.scratchValueP = (*iface)(unsafe.Pointer(&al.scratchValue))
	al.scratchIntVal = LInteger(0)
	al.scratchIntValP = (*iface)(unsafe.Pointer(&al.scratchIntVal))

	return al
}

// LNumber2I takes a number value and returns an interface LValue representing the same number.
// Converting an LNumber to a LValue naively, by doing:
// `var val LValue = myLNumber`
// will result in an individual heap alloc of 8 bytes for the float value. LNumber2I amortizes the cost and memory
// overhead of these allocs by allocating blocks instead.
// The downside of this is that all values on a given block have to become eligible for gc before the block
// as a whole can be gc-ed.
func (al *allocator) LNumber2I(v LNumber) LValue {
	// Fast path: check for shared preloaded integers [0, preloadLimit)
	iv := int(v)
	if iv >= 0 && iv < preloadLimit && LNumber(iv) == v {
		return preloads[iv]
	}

	// check if we need a new alloc page
	if cap(al.ptrs) == len(al.ptrs) {
		al.ptrs = make([]uint64, 0, al.size)
	}

	// alloc from shared pool, reinterpret as float64
	al.ptrs = append(al.ptrs, *(*uint64)(unsafe.Pointer(&v)))
	ptr := &al.ptrs[len(al.ptrs)-1]

	al.scratchValueP.word = unsafe.Pointer(ptr)
	return al.scratchValue
}

// LInteger2I converts an LInteger to LValue with zero-alloc for values in [-65536, 65536).
func (al *allocator) LInteger2I(v LInteger) LValue {
	iv := int(v)
	if iv >= -intPreloadLimit && iv < intPreloadLimit {
		return intPreloads[iv+intPreloadLimit]
	}

	if cap(al.ptrs) == len(al.ptrs) {
		al.ptrs = make([]uint64, 0, al.size)
	}

	// alloc from shared pool, reinterpret as int64
	al.ptrs = append(al.ptrs, uint64(v))
	ptr := &al.ptrs[len(al.ptrs)-1]

	al.scratchIntValP.word = unsafe.Pointer(ptr)
	return al.scratchIntVal
}
