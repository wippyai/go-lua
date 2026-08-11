// Package owner declares TypeValue's cold Factor schema.  Its semantic
// authority stays engine-free; this package retains only typed Forms.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/typevalue"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

type Owner struct {
	authority *typevalue.Authority
	factor    *engine.Factor[coordinate, typevalue.Value]
	output    engine.Output[typevalue.Value]
	read      engine.ReadForm[typevalue.Value, engine.OrderedCells[typevalue.Value]]
	write     engine.WriteForm[typevalue.Value]
	carry     engine.CarryForm
}

func Declare(composition *engine.Composition, semantic engine.SemanticKey, authority *typevalue.Authority) (*Owner, bool) {
	if composition == nil || !semantic.Available() || authority == nil || !validCoordinateCount(authority.RootCount()) {
		return nil, false
	}
	owner := &Owner{authority: authority}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, typevalue.Value]{
		Semantic: semantic, KeyEnd: uint64(authority.RootCount()), Lattice: authority.Lattice(), Default: authority.Bottom(),
		AdmitAt: owner.admits, Fingerprint: authority.Fingerprint,
		WidenRank: engine.Measure[coordinate, typevalue.Value]{Width: 1, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, typevalue.Value]) bool {
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

func (owner *Owner) Authority() *typevalue.Authority {
	if owner == nil {
		return nil
	}
	return owner.authority
}
func (owner *Owner) Output() engine.Output[typevalue.Value] {
	if owner == nil {
		return engine.Output[typevalue.Value]{}
	}
	return owner.output
}
func (owner *Owner) ExactRead() engine.ReadForm[typevalue.Value, engine.OrderedCells[typevalue.Value]] {
	if owner == nil {
		return engine.ReadForm[typevalue.Value, engine.OrderedCells[typevalue.Value]]{}
	}
	return owner.read
}
func (owner *Owner) ExactWrite() engine.WriteForm[typevalue.Value] {
	if owner == nil {
		return engine.WriteForm[typevalue.Value]{}
	}
	return owner.write
}
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues TypeValue's exact Wave-E coordinate capability for one
// Authority-local root. Authority owns the sole reverse lookup and Factor
// owns the sealed-composition fence.
func (owner *Owner) Locate(root typevalue.Root) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.authority == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.authority.RootIndex(root)
	if !ok || uint64(index) >= uint64(owner.authority.RootCount()) {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(coordinate(index))
}

func (owner *Owner) admits(index coordinate, fact typevalue.Value) bool {
	if owner == nil || owner.authority == nil || uint64(index) >= uint64(owner.authority.RootCount()) {
		return false
	}
	_, ok := owner.authority.RootAt(int(index))
	return ok && owner.authority.Owns(fact)
}
func (owner *Owner) widenRank(index coordinate, fact typevalue.Value, component int) uint64 {
	if component != 0 || owner == nil || owner.authority == nil || uint64(index) >= uint64(owner.authority.RootCount()) || !owner.authority.Owns(fact) {
		return 0
	}
	return owner.authority.WidenRank(fact)
}
func validCoordinateCount(count int) bool { return count >= 0 && uint64(count) <= uint64(^uint32(0))+1 }
