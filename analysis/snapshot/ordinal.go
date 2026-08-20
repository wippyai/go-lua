package snapshot

import (
	"math"
	"reflect"
	"unsafe"
)

// An ordinal key is a key whose value is its own position: the key universe
// is the dense range 0..width-1 and nothing else. A writer that publishes an
// emitted sequence has that discipline by construction, and stating it is
// what lets a column store its rows where their position already puts them
// instead of where a hash would.
//
// The discipline is a property of the key type and of the content, never of
// the storage: a column states the same rows whichever way it stores them,
// and every read answers the same. What the ordinal form changes is the cost
// of finding a row, not which rows exist.

// ordinalPlan is how a key type converts to and from its own index. It is
// derived once per key type alongside the hashing schedule, so a column that
// publishes an ordinal universe pays no per-row type inspection.
//
// The zero value describes a key type that is not an ordinal: a string, a
// struct, a content identity. Those key universes are named by their members
// and are stored by hash.
type ordinalPlan struct {
	// size is the width of the integer key in bytes, and signed reports
	// whether its high bit is a sign rather than a magnitude. A negative key
	// is no position, so it converts to no index.
	size   uintptr
	signed bool
	// ordinal reports whether this key type has positions at all.
	ordinal bool
}

// ordinalPlanFor derives the index conversion of keyType. A key is a position
// only when the whole key is one integer: a struct that happens to hold one
// is a compound key whose universe its members name.
func ordinalPlanFor(keyType reflect.Type) ordinalPlan {
	switch keyType.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return ordinalPlan{size: keyType.Size(), ordinal: true}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return ordinalPlan{size: keyType.Size(), signed: true, ordinal: true}
	default:
		return ordinalPlan{}
	}
}

// holds reports whether index is a position this key type can name. A width
// the key type cannot count to is not a dense universe of that key.
func (plan ordinalPlan) holds(index int) bool {
	if !plan.ordinal || index < 0 {
		return false
	}
	bits := uint(plan.size) * 8
	if plan.signed {
		bits--
	}
	if bits >= 63 {
		return true
	}
	return uint64(index) <= 1<<bits-1
}

// keyOrdinal returns the position key names, and reports false for a key type
// that has no positions and for a key that names none: a negative index, and
// a magnitude beyond what a position can be.
func keyOrdinal[K comparable](plan *keyPlan, key K) (int, bool) {
	if !plan.ordinal.ordinal {
		return 0, false
	}
	base := unsafe.Pointer(&key)
	var magnitude uint64
	if plan.ordinal.signed {
		var value int64
		switch plan.ordinal.size {
		case 1:
			value = int64(*(*int8)(base))
		case 2:
			value = int64(*(*int16)(base))
		case 4:
			value = int64(*(*int32)(base))
		default:
			value = *(*int64)(base)
		}
		if value < 0 {
			return 0, false
		}
		magnitude = uint64(value)
	} else {
		switch plan.ordinal.size {
		case 1:
			magnitude = uint64(*(*uint8)(base))
		case 2:
			magnitude = uint64(*(*uint16)(base))
		case 4:
			magnitude = uint64(*(*uint32)(base))
		default:
			magnitude = *(*uint64)(base)
		}
	}
	if magnitude > uint64(math.MaxInt) {
		return 0, false
	}
	return int(magnitude), true
}

// ordinalKey returns the key that names position index. It is the inverse of
// keyOrdinal and is what a publication that stores an ordinal sequence by
// hash uses to name each row.
func ordinalKey[K comparable](plan *keyPlan, index int) K {
	var key K
	base := unsafe.Pointer(&key)
	if plan.ordinal.signed {
		switch plan.ordinal.size {
		case 1:
			*(*int8)(base) = int8(index)
		case 2:
			*(*int16)(base) = int16(index)
		case 4:
			*(*int32)(base) = int32(index)
		default:
			*(*int64)(base) = int64(index)
		}
		return key
	}
	switch plan.ordinal.size {
	case 1:
		*(*uint8)(base) = uint8(index)
	case 2:
		*(*uint16)(base) = uint16(index)
	case 4:
		*(*uint32)(base) = uint32(index)
	default:
		*(*uint64)(base) = uint64(index)
	}
	return key
}
