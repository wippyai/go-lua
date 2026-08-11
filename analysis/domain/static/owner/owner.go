// Package owner declares Static's cold Factor schema without constructing a
// carrier or concrete coordinate handles.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/static"
	"github.com/wippyai/go-lua/analysis/engine"
)

type coordinate uint32

type Owner struct {
	authority *static.Authority
	factor    *engine.Factor[coordinate, static.Value]
	output    engine.Output[static.Value]
	read      engine.ReadForm[static.Value, engine.OrderedCells[static.Value]]
	write     engine.WriteForm[static.Value]
	carry     engine.CarryForm
}

// Declare attaches the sealed authored Static family once.  Its dense
// position is solely carrier layout, introduced later by Wave E.
func Declare(composition *engine.Composition, semantic engine.SemanticKey, authority *static.Authority) (*Owner, bool) {
	if composition == nil || !semantic.Available() || authority == nil || !authority.ContentID().Available() || !validCoordinateCount(authority.CoordinateCount()) {
		return nil, false
	}
	owner := &Owner{authority: authority}
	factor, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, static.Value]{
		Semantic: semantic, KeyEnd: uint64(authority.CoordinateCount()), Lattice: authority.Lattice(), Default: authority.Bottom(),
		AdmitAt: owner.admits, Fingerprint: authority.Fingerprint,
		WidenRank: engine.Measure[coordinate, static.Value]{Width: 1, At: owner.widenRank},
	}, func(factor *engine.Factor[coordinate, static.Value]) bool {
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
	// Locate still cannot issue a capability until the entire Composition seals.
	owner.factor = factor
	return owner, true
}

func (owner *Owner) Authority() *static.Authority {
	if owner == nil {
		return nil
	}
	return owner.authority
}
func (owner *Owner) Output() engine.Output[static.Value] {
	if owner == nil {
		return engine.Output[static.Value]{}
	}
	return owner.output
}
func (owner *Owner) ExactRead() engine.ReadForm[static.Value, engine.OrderedCells[static.Value]] {
	if owner == nil {
		return engine.ReadForm[static.Value, engine.OrderedCells[static.Value]]{}
	}
	return owner.read
}
func (owner *Owner) ExactWrite() engine.WriteForm[static.Value] {
	if owner == nil {
		return engine.WriteForm[static.Value]{}
	}
	return owner.write
}
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues Static's exact Wave-E coordinate capability for one canonical
// Authority-local coordinate. It deliberately accepts neither a dense index
// nor a portable type reference: Authority owns the sole reverse lookup and
// Factor owns the sealed-composition fence.
func (owner *Owner) Locate(location static.Coordinate) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.authority == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.authority.CoordinateIndex(location)
	if !ok || uint64(index) >= uint64(owner.authority.CoordinateCount()) {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(coordinate(index))
}

func (owner *Owner) admits(index coordinate, fact static.Value) bool {
	if owner == nil || owner.authority == nil || uint64(index) >= uint64(owner.authority.CoordinateCount()) {
		return false
	}
	_, ok := owner.authority.CoordinateAt(int(index))
	return ok && owner.authority.Owns(fact)
}

func (owner *Owner) widenRank(index coordinate, fact static.Value, component int) uint64 {
	if component != 0 || owner == nil || owner.authority == nil || uint64(index) >= uint64(owner.authority.CoordinateCount()) || !owner.authority.Owns(fact) {
		return 0
	}
	return owner.authority.WidenRank(fact)
}

func validCoordinateCount(count int) bool { return count >= 0 && uint64(count) <= uint64(^uint32(0))+1 }
