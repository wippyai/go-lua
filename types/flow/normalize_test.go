package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNormalizeMutatorAssignmentsOrdersFieldsUsedByEquality(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(1), "items")
	a := &Inputs{
		MapMutatorAssignments: []MapMutatorAssignment{
			{Point: 10, Target: target, KeyType: typ.String, ValueType: typ.Number},
			{Point: 10, Target: target, KeyType: typ.Number, ValueType: typ.String},
		},
		TableMutatorAssignments: []TableMutatorAssignment{
			{Point: 10, Target: target, ValueType: typ.String, LengthDelta: 2},
			{Point: 10, Target: target, ValueType: typ.String, LengthDelta: 1},
		},
	}
	b := &Inputs{
		MapMutatorAssignments: []MapMutatorAssignment{
			a.MapMutatorAssignments[1],
			a.MapMutatorAssignments[0],
		},
		TableMutatorAssignments: []TableMutatorAssignment{
			a.TableMutatorAssignments[1],
			a.TableMutatorAssignments[0],
		},
	}

	a.Normalize()
	b.Normalize()

	if !InputsEqual(a, b) {
		t.Fatalf("Normalize left equality-distinct mutator rows order-sensitive:\nA=%#v\nB=%#v", a, b)
	}
}
