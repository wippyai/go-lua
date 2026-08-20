package snapshot

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"reflect"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/identity"
)

// keySeed is the process-wide hashing seed of every published trie. A
// snapshot is an in-memory value that never leaves the process, so the seed
// is chosen once per process: hashes are comparable across every snapshot a
// process holds and are not a durable encoding anyone can depend on.
var keySeed = maphash.MakeSeed()

// keyPlan is the hashing schedule of one key type. It is derived once, from
// the type alone, and every column and denominator keyed by that type shares
// the schedule, which is what lets a denominator sealed by one column answer
// under the hash another column computed.
//
// A schedule exists because a key is hashed by what makes two keys equal.
// Padding bytes inside a struct are not part of equality and are never
// hashed, a string is hashed by its contents rather than by its header, and
// a float is normalized so the two zeros hash alike. Adjacent equality-
// relevant bytes are coalesced into one step, so the common key shapes -- an
// integer, a content identity, a flat struct -- hash in a single memory pass.
type keyPlan struct {
	steps []keyStep
	// ordinal is how this key type converts to and from its own position. A
	// key type that has positions can address a dense universe by index; one
	// that has none is only ever addressed by hash.
	ordinal ordinalPlan
}

// keyStepKind names how one region of a key contributes to its hash.
type keyStepKind uint8

const (
	// keyBytes hashes size bytes at offset directly.
	keyBytes keyStepKind = iota
	// keyString hashes the contents of the string header at offset.
	keyString
	// keyFloat32 and keyFloat64 hash a normalized floating point value, so
	// the negative and positive zeros that compare equal hash equal.
	keyFloat32
	keyFloat64
)

// keyStep is one region of a key and how it is hashed.
type keyStep struct {
	kind   keyStepKind
	offset uintptr
	size   uintptr
}

// planFor derives the hashing schedule of K. It reports false for a key type
// whose equality is dynamic rather than structural: an interface key compares
// by a dynamic type this package cannot see, so a column can never be keyed
// by one.
func planFor[K comparable]() (*keyPlan, bool) {
	keyType := reflect.TypeOf((*K)(nil)).Elem()
	steps, derived := planSteps(keyType, 0, nil)
	if !derived {
		return nil, false
	}
	return &keyPlan{steps: steps, ordinal: ordinalPlanFor(keyType)}, true
}

// planSteps appends the schedule of one type at offset to steps.
func planSteps(keyType reflect.Type, offset uintptr, steps []keyStep) ([]keyStep, bool) {
	switch keyType.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Pointer, reflect.UnsafePointer, reflect.Chan:
		return appendStep(steps, keyStep{kind: keyBytes, offset: offset, size: keyType.Size()}), true
	case reflect.Float32:
		return appendStep(steps, keyStep{kind: keyFloat32, offset: offset, size: 4}), true
	case reflect.Float64:
		return appendStep(steps, keyStep{kind: keyFloat64, offset: offset, size: 8}), true
	case reflect.Complex64:
		steps = appendStep(steps, keyStep{kind: keyFloat32, offset: offset, size: 4})
		return appendStep(steps, keyStep{kind: keyFloat32, offset: offset + 4, size: 4}), true
	case reflect.Complex128:
		steps = appendStep(steps, keyStep{kind: keyFloat64, offset: offset, size: 8})
		return appendStep(steps, keyStep{kind: keyFloat64, offset: offset + 8, size: 8}), true
	case reflect.String:
		return appendStep(steps, keyStep{kind: keyString, offset: offset, size: keyType.Size()}), true
	case reflect.Array:
		element := keyType.Elem()
		for index := 0; index < keyType.Len(); index++ {
			var derived bool
			steps, derived = planSteps(element, offset+uintptr(index)*element.Size(), steps)
			if !derived {
				return nil, false
			}
		}
		return steps, true
	case reflect.Struct:
		for index := 0; index < keyType.NumField(); index++ {
			field := keyType.Field(index)
			var derived bool
			steps, derived = planSteps(field.Type, offset+field.Offset, steps)
			if !derived {
				return nil, false
			}
		}
		return steps, true
	default:
		return nil, false
	}
}

// appendStep adds step to the schedule, merging it into the previous step
// when both hash raw bytes that are adjacent in the key. The merge is what
// collapses a byte array, a flat struct, or a nested struct of scalars into a
// single memory pass.
func appendStep(steps []keyStep, step keyStep) []keyStep {
	if step.kind == keyBytes && len(steps) > 0 {
		previous := &steps[len(steps)-1]
		if previous.kind == keyBytes && previous.offset+previous.size == step.offset {
			previous.size += step.size
			return steps
		}
	}
	return append(steps, step)
}

// hashKey hashes key under plan. It reads the key through its own address
// and never retains it, so a hash costs no allocation on any key shape.
func hashKey[K comparable](plan *keyPlan, key K) uint64 {
	base := unsafe.Pointer(&key)
	if len(plan.steps) == 1 {
		step := plan.steps[0]
		switch step.kind {
		case keyBytes:
			return maphash.Bytes(keySeed, unsafe.Slice((*byte)(unsafe.Add(base, step.offset)), step.size))
		case keyString:
			return maphash.String(keySeed, *(*string)(unsafe.Add(base, step.offset)))
		}
	}
	var digest maphash.Hash
	digest.SetSeed(keySeed)
	var scratch [8]byte
	for _, step := range plan.steps {
		switch step.kind {
		case keyBytes:
			digest.Write(unsafe.Slice((*byte)(unsafe.Add(base, step.offset)), step.size))
		case keyString:
			// A variable-length field writes its length before its bytes, so two
			// keys whose strings concatenate identically still hash apart: the
			// boundary between fields is part of the hashed content.
			value := *(*string)(unsafe.Add(base, step.offset))
			binary.LittleEndian.PutUint64(scratch[:8], uint64(len(value)))
			digest.Write(scratch[:8])
			digest.WriteString(value)
		case keyFloat32:
			value := *(*float32)(unsafe.Add(base, step.offset))
			if value == 0 {
				value = 0
			}
			binary.LittleEndian.PutUint32(scratch[:4], math.Float32bits(value))
			digest.Write(scratch[:4])
		case keyFloat64:
			value := *(*float64)(unsafe.Add(base, step.offset))
			if value == 0 {
				value = 0
			}
			binary.LittleEndian.PutUint64(scratch[:], math.Float64bits(value))
			digest.Write(scratch[:])
		}
	}
	return digest.Sum64()
}

// identityPlan is the schedule of the one identity key type this package
// stores itself: the directory and query publication are keyed by a content
// identity.
var identityPlan = mustPlan[identity.ContentID]()

// mustPlan derives the schedule of a key type this package itself stores.
// Those types are fixed at compile time, so a failure is a programming error
// in this package rather than a caller's input.
func mustPlan[K comparable]() *keyPlan {
	plan, derived := planFor[K]()
	if !derived {
		panic("snapshot: internal key type is not hashable")
	}
	return plan
}
