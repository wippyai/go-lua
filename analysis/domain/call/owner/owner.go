// Package owner declares Call's finite dispatch-completeness relation to one
// cold engine Factor. Call keeps the Application/target algebra; this package
// merely preserves its coordinate-specific admission law at the engine edge.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/call"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
)

// coordinate is private dense layout for Call's sealed source sum. Link
// Applications, callback ports, and resume ports—not this ordinal—
// remain the semantic identities.
type coordinate uint32

// Owner holds Call's immutable algebra and the complete cold Rule capability
// surface. Its private lookup snapshot is not exported as a key plane; it
// deliberately contains no Unit, Target, or carrier authority.
type Owner struct {
	algebra *call.Algebra
	factor  *engine.Factor[coordinate, call.Value]
	locate  map[call.Key]coordinate

	output engine.Output[call.Value]
	read   engine.ReadForm[call.Value, engine.OrderedCells[call.Value]]
	write  engine.WriteForm[call.Value]
	carry  engine.CarryForm
}

// Declare converts the complete already-sealed Call source family into one
// Factor declaration. KeyCount zero is lawful; no direct Factor coordinate is
// exposed to Rules before Wave E binds Link Applications to Program points.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, algebra *call.Algebra) (*Owner, bool) {
	if composition == nil || !semantic.Available() || algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.KeyCount()) {
		return nil, false
	}
	lattice, ok := algebra.Lattice()
	if !ok {
		return nil, false
	}
	locations, located := locateCoordinates(algebra)
	if !located {
		return nil, false
	}
	owner := &Owner{algebra: algebra, locate: locations}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, call.Value]{
		Semantic:    semantic,
		KeyEnd:      uint64(algebra.KeyCount()),
		Lattice:     lattice,
		Default:     algebra.Default(),
		AdmitAt:     owner.admits,
		Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, call.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, func(factor *engine.Factor[coordinate, call.Value]) bool {
		read, readOK := engine.ExactReadForm(factor)
		write, writeOK := engine.ExactWriteForm(factor)
		carry, carryOK := engine.Carry(factor)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output = factor.Output()
		owner.read = read
		owner.write = write
		owner.carry = carry
		return true
	})
	if !ok || factor == nil {
		return nil, false
	}
	owner.factor = factor
	return owner, true
}

// Algebra exposes the immutable Call semantic vocabulary, including exact
// source-target support. Engine code must never recreate this relation.
func (owner *Owner) Algebra() *call.Algebra {
	if owner == nil {
		return nil
	}
	return owner.algebra
}

// Link returns the exact sealed Link that issued this Call owner's keys.
// It is intentionally only a provenance fence; Call keys and Values remain
// the sole Call vocabulary and are never reconstructed from the Link.
func (owner *Owner) Link() *link.Link {
	if owner == nil || owner.algebra == nil {
		return nil
	}
	return owner.algebra.Link()
}

// Output is the sole key-erased output capability for Call Rules.
func (owner *Owner) Output() engine.Output[call.Value] {
	if owner == nil {
		return engine.Output[call.Value]{}
	}
	return owner.output
}

// ExactRead is the owner-issued exact Call observation shape. The matching
// Call source is supplied only by the Wave-E Program/Link template binder.
func (owner *Owner) ExactRead() engine.ReadForm[call.Value, engine.OrderedCells[call.Value]] {
	if owner == nil {
		return engine.ReadForm[call.Value, engine.OrderedCells[call.Value]]{}
	}
	return owner.read
}

// ExactWrite is the owner-issued exact Call output shape. It contains no
// source ordinal or carrier target.
func (owner *Owner) ExactWrite() engine.WriteForm[call.Value] {
	if owner == nil {
		return engine.WriteForm[call.Value]{}
	}
	return owner.write
}

// Carry is the explicit whole-Call predecessor form, separate from exact
// Application reads.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Call's exact sealed coordinate capability for one canonical
// Algebra-local key. The reverse relation is snapshotted from Algebra.KeyAt
// when Declare closes the owner surface; it accepts no ordinals or substitute
// source identity.
func (owner *Owner) Locate(key call.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.algebra == nil || owner.factor == nil || !key.Valid() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.locate[key]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func locateCoordinates(algebra *call.Algebra) (map[call.Key]coordinate, bool) {
	if algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.KeyCount()) {
		return nil, false
	}
	locations := make(map[call.Key]coordinate, algebra.KeyCount())
	for index := 0; index < algebra.KeyCount(); index++ {
		key, ok := algebra.KeyAt(index)
		if !ok || !key.Valid() || uint64(index) > uint64(^uint32(0)) {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := locations[key]; duplicate {
			return nil, false
		}
		locations[key] = coordinate
	}
	return locations, len(locations) == algebra.KeyCount()
}

func (owner *Owner) keyAt(index coordinate) (call.Key, bool) {
	if owner == nil || owner.algebra == nil || uint64(index) >= uint64(owner.algebra.KeyCount()) {
		return call.Key{}, false
	}
	return owner.algebra.KeyAt(int(index))
}

// admits delegates every key/value fence to Call's proven algebra.
func (owner *Owner) admits(index coordinate, value call.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.algebra.Admits(key, value)
}

func (owner *Owner) widenRank(index coordinate, value call.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	rank, ok := owner.algebra.WidenRank(key)
	if !ok {
		return 0
	}
	measure, ok := rank.At(value, component)
	if !ok {
		return 0
	}
	return measure
}

// Call's dense source range can include all uint32 values because
// coordinate zero is a private engine layout value, not a Call Key sentinel.
func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
