// Package owner declares Numeric's cold Factor schema.  Numeric's relation
// algebra remains engine-free; no concrete units or targets are retained.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/numeric"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
)

type coordinate uint32

type Owner struct {
	algebra *numeric.Algebra
	factor  *engine.Factor[coordinate, numeric.Value]
	locator map[numeric.Key]coordinate
	output  engine.Output[numeric.Value]
	read    engine.ReadForm[numeric.Value, engine.OrderedCells[numeric.Value]]
	write   engine.WriteForm[numeric.Value]
	carry   engine.CarryForm
	width   int
}

func Declare(composition *engine.Composition, semantic engine.SemanticKey, algebra *numeric.Algebra) (*Owner, bool) {
	if composition == nil || !semantic.Available() || algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.KeyCount()) {
		return nil, false
	}
	lattice := algebra.Lattice()
	defaultValue := algebra.Default()
	if !lattice.Equal(defaultValue, defaultValue) {
		return nil, false
	}
	width, ok := widestRank(algebra)
	if !ok {
		return nil, false
	}
	locator, ok := locateKeys(algebra)
	if !ok {
		return nil, false
	}
	owner := &Owner{algebra: algebra, locator: locator, width: width}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, numeric.Value]{
		Semantic: semantic, KeyEnd: uint64(algebra.KeyCount()), Lattice: lattice, Default: defaultValue,
		AdmitAt: owner.admits, Fingerprint: numeric.Value.Hash,
		WidenRank: engine.Measure[coordinate, numeric.Value]{Width: width, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, numeric.Value]) bool {
		var valid bool
		owner.output = factor.Output()
		owner.read, valid = engine.ExactReadForm(factor)
		if !valid {
			return false
		}
		owner.write, valid = engine.ExactWriteForm(factor)
		if !valid {
			return false
		}
		owner.carry, valid = engine.Carry(factor)
		return valid
	})
	if !ok {
		return nil, false
	}
	// Keep the typed issuer only after its declaration callback has closed.
	// Locate still cannot issue a capability until the Composition seals.
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Algebra() *numeric.Algebra {
	if owner == nil {
		return nil
	}
	return owner.algebra
}

// Link returns the exact sealed Link that issued this Numeric owner's
// coordinates. It is a live ownership fence; replay identities remain on the
// Algebra and continue to use LinkID.
func (owner *Owner) Link() *link.Link {
	if owner == nil || owner.algebra == nil {
		return nil
	}
	return owner.algebra.Link()
}

func (owner *Owner) Output() engine.Output[numeric.Value] {
	if owner == nil {
		return engine.Output[numeric.Value]{}
	}
	return owner.output
}
func (owner *Owner) ExactRead() engine.ReadForm[numeric.Value, engine.OrderedCells[numeric.Value]] {
	if owner == nil {
		return engine.ReadForm[numeric.Value, engine.OrderedCells[numeric.Value]]{}
	}
	return owner.read
}
func (owner *Owner) ExactWrite() engine.WriteForm[numeric.Value] {
	if owner == nil {
		return engine.WriteForm[numeric.Value]{}
	}
	return owner.write
}
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Numeric's exact composition-local coordinate capability for
// one canonical Algebra key. The declaration-time table is intentionally the
// only reverse lookup: it neither exposes ordinals nor searches algebra state
// on the hot path.
func (owner *Owner) Locate(key numeric.Key) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.algebra == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.locator[key]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func locateKeys(algebra *numeric.Algebra) (map[numeric.Key]coordinate, bool) {
	if algebra == nil || !algebra.Valid() {
		return nil, false
	}
	keys := make(map[numeric.Key]coordinate, algebra.KeyCount())
	for index := 0; index < algebra.KeyCount(); index++ {
		key, ok := algebra.KeyAt(index)
		if !ok || !key.Valid() {
			return nil, false
		}
		coordinate := coordinate(index)
		if _, duplicate := keys[key]; duplicate {
			return nil, false
		}
		keys[key] = coordinate
	}
	return keys, true
}

func (owner *Owner) keyAt(index coordinate) (numeric.Key, bool) {
	if owner == nil || owner.algebra == nil || uint64(index) >= uint64(owner.algebra.KeyCount()) {
		return numeric.Key{}, false
	}
	return owner.algebra.KeyAt(int(index))
}
func (owner *Owner) admits(index coordinate, fact numeric.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.algebra.Admits(key, fact)
}
func (owner *Owner) widenRank(index coordinate, fact numeric.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok || component < 0 || component >= owner.width {
		return 0
	}
	rank, ok := owner.algebra.WidenRank(key)
	if !ok || component >= rank.Width() {
		return 0
	}
	measure, ok := rank.At(fact, component)
	if !ok {
		return 0
	}
	return measure
}
func widestRank(algebra *numeric.Algebra) (int, bool) {
	width := 1
	for index := 0; index < algebra.KeyCount(); index++ {
		key, ok := algebra.KeyAt(index)
		if !ok {
			return 0, false
		}
		rank, ok := algebra.WidenRank(key)
		if !ok || rank.Width() <= 0 {
			return 0, false
		}
		if rank.Width() > width {
			width = rank.Width()
		}
	}
	return width, true
}
func validCoordinateCount(count int) bool { return count >= 0 && uint64(count) <= uint64(^uint32(0))+1 }
