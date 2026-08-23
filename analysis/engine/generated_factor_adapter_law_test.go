package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/lattice"
)

type generatedFactorAdapterFixture struct {
	factor      *boundFactor[uint32, uint64]
	binding     *factbinding.Binding[uint32, uint64]
	composition *carrier.Composition
	plan        carrier.ContributionPlan
	source      carrier.PointState
	slot        shape.Slot
	unit        carrier.Unit
	target      carrier.Target
	weak        carrier.Target
	work        *carrier.Work
	state       carrier.State
	whole       support.Mask
}

func newGeneratedFactorAdapterFixture(t *testing.T) generatedFactorAdapterFixture {
	t.Helper()
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, wholeOK := support.True(manager)
	if !wholeOK {
		t.Fatal("whole support")
	}
	values := lattice.Lattice[uint64]{
		Bottom: func() uint64 { return 0 },
		Top:    func() uint64 { return ^uint64(0) },
		Equal:  func(left, right uint64) bool { return left == right },
		Same:   func(left, right uint64) bool { return left == right },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen:    func(left, right uint64) uint64 { return left | right },
		LessOrEq: func(left, right uint64) bool { return left <= right },
	}
	algebra, admitted := factbinding.Admit(uint64(1), uint64(0), values, func(_ uint32, _ uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint32, uint64]{}, factbinding.Measure[uint32, uint64]{})
	if !admitted {
		t.Fatal("factor algebra")
	}
	var unit carrier.Unit
	var target, weak carrier.Target
	binding, bound := factbinding.Bind(algebra, manager, func(binding *factbinding.Binding[uint32, uint64]) bool {
		var ok bool
		unit, ok = binding.DeclareExact(0)
		if !ok {
			return false
		}
		target, ok = binding.DeclareStrong(unit)
		if !ok {
			return false
		}
		weak, ok = binding.DeclareWeak([]carrier.Unit{unit})
		return ok
	})
	if !bound {
		t.Fatal("factor binding")
	}
	prepared, preparedOK := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !preparedOK {
		t.Fatal("prepare composition")
	}
	composition, attached := prepared.Attach()
	if !attached {
		t.Fatal("attach composition")
	}
	slot, slotOK := target.Slot()
	if !slotOK {
		t.Fatal("factor slot")
	}
	plan, planOK := composition.SealContribution(1, []shape.Slot{slot}, nil)
	if !planOK {
		t.Fatal("contribution plan")
	}
	state, stateOK := carrier.NewState(composition, composition.Scope(), whole)
	work, workOK := composition.NewWork()
	if !stateOK || !workOK {
		t.Fatal("runtime state")
	}
	source, sourceOK := work.EmptyPointState(state)
	if !sourceOK {
		t.Fatal("point source")
	}
	return generatedFactorAdapterFixture{
		factor:      &boundFactor[uint32, uint64]{binding: binding, slot: slot, hasSlot: true},
		binding:     binding,
		composition: composition,
		plan:        plan,
		source:      source,
		slot:        slot,
		unit:        unit,
		target:      target,
		weak:        weak,
		work:        work,
		state:       state,
		whole:       whole,
	}
}
