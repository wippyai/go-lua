package execution

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// TestTransformedCarryIsItsOwnForm states that a sealed transformed carry is a
// form of its own. An identity carry hands the prior output fact on unchanged,
// which the exact fold can do with no domain call at all; a transformed carry
// applies one owner-issued candidate-indexed transition to it, which the exact
// fold cannot do and must never be given to silently. The two descriptors
// differ only in their carry, so the derivation must separate them there.
func TestTransformedCarryIsItsOwnForm(t *testing.T) {
	identity := planCompiledExactRule(t)
	transformed := planCompiledTransformedCarryRule(t)

	if row, ok := DeclaredForm(identity); !ok || row.Form != FormExact {
		t.Fatalf("an identity carry stopped deriving the exact form: %q/%t", row.Form.Name(), ok)
	}

	carried, ok := DeclaredForm(transformed)
	if !ok || carried.Form != FormCarry {
		t.Fatalf("transformed carry derived as %q/%t, want carry", carried.Form.Name(), ok)
	}
	if carried.Input != 0 {
		t.Fatalf("transformed carry read port = %d, want 0", carried.Input)
	}
	if _, present := carried.Rule.CarryTransform(); !present {
		t.Fatal("the derived row lost the transform address it was derived from")
	}
}

// planCompiledTransformedCarryRule seals the one-join exact-output descriptor
// whose carry names an owner-issued transform member.
func planCompiledTransformedCarryRule(t *testing.T) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 1},
		Reads: []generated.ReadPlan{{
			Input: 0, Factor: 2, Axis: 0,
			Relation: ruleplan.RelationAddr{Axis: 0, Member: 0}, Key: ruleplan.ProjectionAddr{Axis: 0, Member: 0},
			Addressing: ruleplan.RelationAddr{Axis: 0, Member: 0}, AddressingPresent: true,
			Form:        ruleprogram.Exact,
			PointBound:  ruleprogram.PointBound,
			Contract:    ruleplan.ReadContract{Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit, OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne},
			RowCapacity: 1, CellCapacity: 1,
		}},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{
			Input: 0, Factor: 2, Mode: ruleprogram.CarryTransform,
			Transform: ruleplan.CarryTransformAddr{Axis: 2, Member: 1}, TransformPresent: true,
		},
	})
	if !ok {
		t.Fatal("sealed transformed carry plan")
	}
	return rule
}

// TestAReadFreeTransformedCarryIsItsOwnForm states that the carry disposition
// decides that a descriptor is a carry, and the declared read vocabulary
// decides which carry it is. A carry over one exact cell reduces that cell and
// holds its publication to the region the cell reported; a carry that declares
// no read has neither, answers from its candidate, and publishes over the
// invocation's own support. Handing the second to FoldCarry would open a cursor
// the declaration never named.
//
// The port it opens at is the one its carry declares, because no read declares
// one. That port is already held to the descriptor's sealed input prefix by
// generated.NewPlanCompiledRule, which refuses to seal a carry naming a port
// no input declares, so a descriptor reaching the classifier cannot carry one.
func TestAReadFreeTransformedCarryIsItsOwnForm(t *testing.T) {
	readFree := planCompiledReadFreeCarryRule(t, 0)

	row, ok := DeclaredForm(readFree)
	if !ok || row.Form != FormSourceCarry {
		t.Fatalf("read-free transformed carry derived as %q/%t, want source-carry", row.Form.Name(), ok)
	}
	if row.Input != 0 {
		t.Fatalf("read-free carry port = %d, want the port its carry names", row.Input)
	}
	if _, present := row.Rule.CarryTransform(); !present {
		t.Fatal("the derived row lost the transform address it was derived from")
	}
	if carried, carriedOK := DeclaredForm(planCompiledTransformedCarryRule(t)); !carriedOK || carried.Form != FormCarry {
		t.Fatalf("the one-read carry stopped deriving its own form: %q/%t", carried.Form.Name(), carriedOK)
	}
}

// planCompiledReadFreeCarryRule seals a transformed-carry descriptor that
// declares no read, with its carry naming the given input port.
func planCompiledReadFreeCarryRule(t *testing.T, port uint32) generated.CompiledRule {
	t.Helper()
	rule, ok := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: 3, InputCount: 1,
		Candidate: ruleplan.RelationAddr{Axis: 0, Member: 0},
		Reducer:   ruleplan.ReducerAddr{Axis: 2, Member: 1},
		// The declared read vector is empty, not absent: this rule reads
		// nothing, which is a statement it makes rather than one it omits.
		Reads: []generated.ReadPlan{},
		Outputs: []generated.OutputPlan{{
			Factor: 2, Axis: 2, Address: ruleplan.OutputAddr{Axis: 2, Frame: 0},
			Destination: ruleplan.ProjectionAddr{Axis: 0, Member: 0}, Mode: ruleprogram.ModeExact, Exact: true, Strong: true,
		}},
		Carry: &generated.CarryPlan{
			Input: port, Factor: 2, Mode: ruleprogram.CarryTransform,
			Transform: ruleplan.CarryTransformAddr{Axis: 2, Member: 1}, TransformPresent: true,
		},
	})
	if !ok {
		t.Fatal("sealed read-free transformed carry plan")
	}
	return rule
}
