package generated

import (
	"testing"

	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func heterogeneousPlanLawSpec() CompiledRuleSpec {
	return CompiledRuleSpec{
		AxisCount:  3,
		InputCount: 3,
		Candidate:  ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:    ruleplan.ReducerAddr{Axis: 2, Member: 0},
		Reads: []ReadPlan{
			{
				Input: 0, Factor: 1, Axis: 1,
				Sources:    ruleplan.Span{Start: 0, Count: 1},
				Relation:   ruleplan.RelationAddr{Axis: 1, Member: 4},
				Key:        ruleplan.ProjectionAddr{Axis: 1, Member: 6},
				Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
				Form:       ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract: ruleplan.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaqueRefuse,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
				RowCapacity:  2,
				CellCapacity: 3,
			},
			{
				Input: 1, Factor: 0, Axis: 0,
				Sources:          ruleplan.Span{Start: 1, Count: 2},
				Relation:         ruleplan.RelationAddr{Axis: 0, Member: 7},
				Key:              ruleplan.ProjectionAddr{Axis: 0, Member: 8},
				Predicate:        ruleplan.ProjectionAddr{Axis: 0, Member: 9},
				PredicatePresent: true,
				Form:             ruleprogram.Selected,
				PointBound:       ruleprogram.PointBound,
				Contract: ruleplan.ReadContract{
					Order:        ruleprogram.OrderByTag,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
					Multiplicity: ruleprogram.MultiplicityMany,
				},
				Denominator:  ruleplan.DenominatorAddr{Ordinal: 5, Present: true},
				RowCapacity:  4,
				CellCapacity: 8,
			},
		},
		Outputs: []OutputPlan{{
			Factor:           2,
			Axis:             2,
			Address:          ruleplan.OutputAddr{Axis: 2, Frame: 7},
			Destination:      ruleplan.ProjectionAddr{Axis: 0, Member: 11},
			Mode:             ruleprogram.ModeRoute,
			Slot:             0,
			RouteJoin:        1,
			RouteJoinPresent: true,
		}},
		Carry: &CarryPlan{
			Input: 2, Factor: 2, Mode: ruleprogram.CarryIdentity, Identity: true,
		},
	}
}

func TestCompiledRuleSealsOrderedHeterogeneousJoinTable(t *testing.T) {
	descriptor, ok := NewPlanCompiledRule(heterogeneousPlanLawSpec())
	if !ok || !descriptor.Available() {
		t.Fatalf("heterogeneous descriptor refused: %+v/%t", descriptor, ok)
	}
	if descriptor.InputCount() != 3 || descriptor.ReadCount() != 2 || descriptor.OutputCount() != 1 {
		t.Fatalf("descriptor cardinality = inputs=%d reads=%d outputs=%d", descriptor.InputCount(), descriptor.ReadCount(), descriptor.OutputCount())
	}
	first, firstOK := descriptor.ReadAt(0)
	second, secondOK := descriptor.ReadAt(1)
	if !firstOK || !secondOK || first.Form != ruleprogram.Exact || second.Form != ruleprogram.Selected ||
		first.Factor == second.Factor || first.Axis == second.Axis || first.Sources != (ruleplan.Span{Start: 0, Count: 1}) ||
		second.Sources != (ruleplan.Span{Start: 1, Count: 2}) || !second.PredicatePresent || second.Denominator != (ruleplan.DenominatorAddr{Ordinal: 5, Present: true}) {
		t.Fatalf("ordered join metadata lost: first=%+v second=%+v", first, second)
	}
	if form, formOK := descriptor.ReadFormAt(1); !formOK || form != ruleprogram.Selected {
		t.Fatalf("selected form accessor = %v/%t", form, formOK)
	}
	if contract, contractOK := descriptor.ReadContractAt(1); !contractOK || contract.Multiplicity != ruleprogram.MultiplicityMany || contract.OnOpaque != ruleprogram.OnOpaquePropagateAuthenticated {
		t.Fatalf("selected contract accessor = %+v/%t", contract, contractOK)
	}
	if mode, modeOK := descriptor.OutputMode(); !modeOK || mode != ruleprogram.ModeRoute {
		t.Fatalf("route output mode = %v/%t", mode, modeOK)
	}
	if !descriptor.CarryIdentity() || descriptor.CarryInput() != 2 {
		t.Fatalf("identity carry = %t/%d", descriptor.CarryIdentity(), descriptor.CarryInput())
	}
}

func TestCompiledRuleRejectsUnsupportedFormsAndCarryWithoutDroppingThem(t *testing.T) {
	for _, form := range []ruleprogram.ReadForm{ruleprogram.Summary, ruleprogram.Complete} {
		t.Run("read-form", func(t *testing.T) {
			spec := heterogeneousPlanLawSpec()
			spec.Reads[0].Form = form
			if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
				t.Fatalf("unsupported read form admitted: %v %+v/%t", form, descriptor, ok)
			}
		})
	}
	{
		// A transformed carry is sealed with its transform, never as an
		// identity carry and never with the address dropped: the transform is
		// the whole difference between the two dispositions.
		spec := heterogeneousPlanLawSpec()
		spec.Carry.Mode = ruleprogram.CarryTransform
		spec.Carry.Identity = false
		spec.Carry.TransformPresent = true
		spec.Carry.Transform = ruleplan.CarryTransformAddr{Axis: 2, Member: 1}
		descriptor, ok := NewPlanCompiledRule(spec)
		if !ok || !descriptor.Available() {
			t.Fatalf("transformed carry refused: %+v/%t", descriptor, ok)
		}
		if descriptor.CarryIdentity() {
			t.Fatal("a transformed carry was sealed as an identity carry")
		}
		if mode, modeOK := descriptor.CarryMode(); !modeOK || mode != ruleprogram.CarryTransform {
			t.Fatalf("sealed carry mode = %v/%t", mode, modeOK)
		}
		if address, present := descriptor.CarryTransform(); !present || address != (ruleplan.CarryTransformAddr{Axis: 2, Member: 1}) {
			t.Fatalf("sealed transform address = %+v/%t", address, present)
		}
	}
	{
		// The two halves of the disposition cannot disagree. A mode without an
		// address, and an address without the mode, are both incomplete rows
		// rather than a carry the runtime may guess at.
		spec := heterogeneousPlanLawSpec()
		spec.Carry.Mode = ruleprogram.CarryTransform
		spec.Carry.Identity = false
		if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
			t.Fatalf("transformed carry without its address admitted: %+v/%t", descriptor, ok)
		}
		spec = heterogeneousPlanLawSpec()
		spec.Carry.TransformPresent = true
		spec.Carry.Transform = ruleplan.CarryTransformAddr{Axis: 2, Member: 1}
		if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
			t.Fatalf("identity carry carrying a transform address admitted: %+v/%t", descriptor, ok)
		}
	}
	{
		spec := heterogeneousPlanLawSpec()
		spec.Outputs[0].Mode = ruleprogram.ModeStructural
		spec.Outputs[0].Exact = false
		spec.Outputs[0].Strong = false
		if descriptor, ok := NewPlanCompiledRule(spec); ok || descriptor.Available() {
			t.Fatalf("structural output admitted: %+v/%t", descriptor, ok)
		}
	}
}
