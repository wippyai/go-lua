// Package owner declares Transfer's cold Factor schema. Transfer remains the
// engine-free authority for its sealed correlated arm algebra.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/transfer"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

// Owner stores only the immutable algebra and Factor-issued cold forms.
// transfer.Key is not leaked as a concrete engine coordinate.
type Owner struct {
	algebra  transfer.Algebra
	rank     transfer.WidenRank
	keys     []transfer.Key
	armIndex map[transfer.Arm]coordinate
	factor   *engine.Factor[coordinate, transfer.Value]
	output   engine.Output[transfer.Value]
	read     engine.ReadForm[transfer.Value, engine.OrderedCells[transfer.Value]]
	write    engine.WriteForm[transfer.Value]
	carry    engine.CarryForm
}

// Declare flattens Transfer's already-sealed arm family in canonical order.
// It creates no Link products, units, targets, or runtime binding.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, algebra transfer.Algebra) (*Owner, bool) {
	if composition == nil || !semantic.Available() {
		return nil, false
	}
	keys, armIndex, ok := allKeys(algebra)
	if !ok || !validCoordinateCount(len(keys)) {
		return nil, false
	}
	lattice, latticeOK := algebra.Lattice()
	defaultValue, defaultOK := algebra.Default()
	rank, rankOK := transfer.NewWidenRank(algebra)
	if !latticeOK || !defaultOK || !rankOK {
		return nil, false
	}
	owner := &Owner{algebra: algebra, rank: rank, keys: keys, armIndex: armIndex}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, transfer.Value]{
		Semantic: semantic, KeyEnd: uint64(len(keys)), Lattice: lattice, Default: defaultValue,
		AdmitAt: owner.admits, Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, transfer.Value]{Width: rank.Width(), At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, transfer.Value]) bool {
		read, readOK := engine.ExactReadForm(factor)
		write, writeOK := engine.ExactWriteForm(factor)
		carry, carryOK := engine.Carry(factor)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output, owner.read, owner.write, owner.carry = factor.Output(), read, write, carry
		return true
	})
	if !ok || factor == nil {
		return nil, false
	}
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Algebra() transfer.Algebra {
	if owner == nil {
		return transfer.Algebra{}
	}
	return owner.algebra
}

// Classify issues Transfer's complete typed structural view of an exact arm.
// It is a pure projection over the already sealed algebra; it neither binds a
// Program point nor allocates a runtime carrier.
func (owner *Owner) Classify(arm transfer.Arm) (transfer.Classification, bool) {
	if owner == nil {
		return transfer.Classification{}, false
	}
	return transfer.Classify(owner.algebra, arm)
}

func (owner *Owner) Output() engine.Output[transfer.Value] {
	if owner == nil {
		return engine.Output[transfer.Value]{}
	}
	return owner.output
}

func (owner *Owner) Read() engine.ReadForm[transfer.Value, engine.OrderedCells[transfer.Value]] {
	if owner == nil {
		return engine.ReadForm[transfer.Value, engine.OrderedCells[transfer.Value]]{}
	}
	return owner.read
}

func (owner *Owner) Write() engine.WriteForm[transfer.Value] {
	if owner == nil {
		return engine.WriteForm[transfer.Value]{}
	}
	return owner.write
}

func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues this Owner's exact Factor coordinate for one sealed Transfer
// Arm. The ArmAt→Key declaration sequence is retained privately; callers
// cannot supply a dense ordinal or reconstruct a transfer identity.
func (owner *Owner) Locate(arm transfer.Arm) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.factor == nil || !owner.algebra.ContentID().Available() {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.armIndex[arm]
	if !ok {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(index)
}

func (owner *Owner) keyAt(index coordinate) (transfer.Key, bool) {
	if owner == nil || uint64(index) >= uint64(len(owner.keys)) {
		return transfer.Key{}, false
	}
	return owner.keys[int(index)], true
}

func (owner *Owner) admits(index coordinate, value transfer.Value) bool {
	key, ok := owner.keyAt(index)
	return ok && owner.algebra.Admits(key, value)
}

func (owner *Owner) widenRank(index coordinate, value transfer.Value, component int) uint64 {
	key, ok := owner.keyAt(index)
	if !ok {
		return 0
	}
	return owner.rank.At(key, value, component)
}

func allKeys(algebra transfer.Algebra) ([]transfer.Key, map[transfer.Arm]coordinate, bool) {
	keys := make([]transfer.Key, 0, algebra.ArmCount())
	indices := make(map[transfer.Arm]coordinate, algebra.ArmCount())
	for armIndex := 0; armIndex < algebra.ArmCount(); armIndex++ {
		arm, ok := algebra.ArmAt(armIndex)
		if !ok {
			return nil, nil, false
		}
		key, ok := algebra.Key(arm)
		if !ok {
			return nil, nil, false
		}
		if _, duplicate := indices[arm]; duplicate {
			return nil, nil, false
		}
		keys = append(keys, key)
		indices[arm] = coordinate(armIndex)
	}
	return keys, indices, true
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
