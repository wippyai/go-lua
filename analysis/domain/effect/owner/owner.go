// Package owner declares Effect's one body-root may-set Factor.
//
// The body-root algebra remains the semantic authority. This package only
// closes that algebra over one engine Factor and issues its typed cold forms;
// it does not retain a root table or expose a dense coordinate plane.
package owner

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/factor"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
)

// coordinate is the private dense Factor layout. A body Root remains the
// semantic key and is translated only at the owner boundary.
type coordinate uint32

// Owner is Effect's sole cold Factor issuer. Its state contains no copied root
// relation, ordinal mirror, or concrete Program/Link payload.
type Owner struct {
	algebra *factor.Algebra
	factor  *engine.Factor[coordinate, factor.Value]

	output engine.Output[factor.Value]
	read   engine.ReadForm[factor.Value, engine.OrderedCells[factor.Value]]
	write  engine.WriteForm[factor.Value]
	carry  engine.CarryForm
}

// Declare closes one already-sealed body-root Effect algebra over one engine
// Factor. KeyEnd is exactly the algebra's RootCount; there is no second
// coordinate authority. The second semantic key remains part of the
// declaration boundary for composition compatibility; BodyCall uses staged
// exact reads and does not declare or retain an all-body summary form.
func Declare(composition *engine.Composition, semantic, summarySemantic engine.SemanticKey, algebra *factor.Algebra) (*Owner, bool) {
	if composition == nil || !semantic.Available() || !summarySemantic.Available() || semantic == summarySemantic || algebra == nil || !algebra.Valid() || !validCoordinateCount(algebra.RootCount()) {
		return nil, false
	}
	lattice, ok := algebra.Lattice()
	if !ok {
		return nil, false
	}
	defaultValue := algebra.Default()
	if !lattice.Equal(defaultValue, defaultValue) {
		return nil, false
	}

	owner := &Owner{algebra: algebra}
	declared, ok := engine.DeclareFactor(composition, engine.FactorSpec[coordinate, factor.Value]{
		Semantic:    semantic,
		KeyEnd:      uint64(algebra.RootCount()),
		Lattice:     lattice,
		Default:     defaultValue,
		AdmitAt:     owner.admits,
		Fingerprint: algebra.Fingerprint,
		WidenRank: engine.Measure[coordinate, factor.Value]{
			Width: 1,
			At:    owner.widenRank,
		},
	}, func(issued *engine.Factor[coordinate, factor.Value]) bool {
		read, readOK := engine.ExactReadForm(issued)
		write, writeOK := engine.ExactWriteForm(issued)
		carry, carryOK := engine.Carry(issued)
		if !readOK || !writeOK || !carryOK {
			return false
		}
		owner.output = issued.Output()
		owner.read = read
		owner.write = write
		owner.carry = carry
		return true
	})
	if !ok || declared == nil {
		// The Factor is intentionally retained only after its declaration
		// callback has closed. Ref issuance remains sealed-Composition-gated.
		return nil, false
	}
	owner.factor = declared
	return owner, true
}

// Algebra exposes Effect's immutable body-root semantic authority.
func (owner *Owner) Algebra() *factor.Algebra {
	if owner == nil {
		return nil
	}
	return owner.algebra
}

// Link returns the exact Link provenance of the algebra. It is a provenance
// fence only; it does not expose or recreate Effect roots or atom payloads.
func (owner *Owner) Link() *link.Link {
	if owner == nil || owner.algebra == nil {
		return nil
	}
	return owner.algebra.Link()
}

// Output is Effect's sole typed Rule-output capability.
func (owner *Owner) Output() engine.Output[factor.Value] {
	if owner == nil {
		return engine.Output[factor.Value]{}
	}
	return owner.output
}

// ExactRead is Effect's exact body-root observation form.
func (owner *Owner) ExactRead() engine.ReadForm[factor.Value, engine.OrderedCells[factor.Value]] {
	if owner == nil {
		return engine.ReadForm[factor.Value, engine.OrderedCells[factor.Value]]{}
	}
	return owner.read
}

// ExactWrite is Effect's exact body-root output form.
func (owner *Owner) ExactWrite() engine.WriteForm[factor.Value] {
	if owner == nil {
		return engine.WriteForm[factor.Value]{}
	}
	return owner.write
}

// Carry is the explicit whole-factor predecessor form.
func (owner *Owner) Carry() engine.CarryForm {
	if owner == nil {
		return engine.CarryForm{}
	}
	return owner.carry
}

// Locate issues the exact Factor capability for one algebra-owned body Root.
// RootIndex is the sole inverse relation: foreign roots, including roots from
// another algebra over the same Link, fail closed without a retained map.
func (owner *Owner) Locate(root factor.Root) (engine.Ref[coordinate], bool) {
	if owner == nil || owner.algebra == nil || owner.factor == nil {
		return engine.Ref[coordinate]{}, false
	}
	index, ok := owner.algebra.RootIndex(root)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return engine.Ref[coordinate]{}, false
	}
	return owner.factor.Ref(coordinate(index))
}

// rootAt reconstructs no relation: it asks the semantic algebra for the
// canonical root at one private Factor coordinate while validating callbacks.
func (owner *Owner) rootAt(index coordinate) (factor.Root, bool) {
	if owner == nil || owner.algebra == nil || uint64(index) >= uint64(owner.algebra.RootCount()) {
		return factor.Root{}, false
	}
	return owner.algebra.RootAt(int(index))
}

func (owner *Owner) admits(index coordinate, value factor.Value) bool {
	root, ok := owner.rootAt(index)
	return ok && owner.algebra.Admit(root, value)
}

func (owner *Owner) widenRank(index coordinate, value factor.Value, component int) uint64 {
	root, ok := owner.rootAt(index)
	if !ok || component != 0 {
		return 0
	}
	return owner.algebra.WidenRank(root, value, component)
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))+1
}
