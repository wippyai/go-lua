package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConditionProofProjectorDirectHasTypeWithoutRootSeed(t *testing.T) {
	const point cfg.Point = 3
	path := constraint.NewPath(7, "value")
	projector := ConditionProofProjector{
		ConditionAt: func(cfg.Point) constraint.Condition {
			return constraint.FromConstraints(constraint.HasType{
				Path: path,
				Type: narrow.BuiltinTypeKey("string"),
			})
		},
	}

	got := projector.ConditionTypeAt(point, path)
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ConditionTypeAt(value) = %v, want string", got)
	}
}

func TestConditionProofProjectorJoinsDirectHasTypeDisjunctsWithoutRootSeed(t *testing.T) {
	const point cfg.Point = 4
	path := constraint.NewPath(8, "value")
	projector := ConditionProofProjector{
		ConditionAt: func(cfg.Point) constraint.Condition {
			return constraint.FromDisjuncts([][]constraint.Constraint{
				{
					constraint.HasType{
						Path: path,
						Type: narrow.BuiltinTypeKey("string"),
					},
				},
				{
					constraint.HasType{
						Path: path,
						Type: narrow.BuiltinTypeKey("number"),
					},
				},
			})
		},
	}

	got := projector.ConditionTypeAt(point, path)
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("ConditionTypeAt(value) = %v, want %v", got, want)
	}
}

func TestConditionProofProjectorRejectsPartialDirectHasTypeProofWithoutRootSeed(t *testing.T) {
	const point cfg.Point = 5
	path := constraint.NewPath(9, "value")
	other := constraint.NewPath(10, "other")
	projector := ConditionProofProjector{
		ConditionAt: func(cfg.Point) constraint.Condition {
			return constraint.FromDisjuncts([][]constraint.Constraint{
				{
					constraint.HasType{
						Path: path,
						Type: narrow.BuiltinTypeKey("string"),
					},
				},
				{
					constraint.HasType{
						Path: other,
						Type: narrow.BuiltinTypeKey("string"),
					},
				},
			})
		},
	}

	got := projector.ConditionTypeAt(point, path)
	if got != nil {
		t.Fatalf("ConditionTypeAt(value) = %v, want nil for partial OR proof", got)
	}
}

func TestConditionProofProjectorDoesNotUseSiblingDirectHasTypeProof(t *testing.T) {
	const point cfg.Point = 6
	root := constraint.NewPath(11, "event")
	projector := ConditionProofProjector{
		ConditionAt: func(cfg.Point) constraint.Condition {
			return constraint.FromConstraints(constraint.HasType{
				Path: root.Field("id"),
				Type: narrow.BuiltinTypeKey("string"),
			})
		},
	}

	got := projector.ConditionTypeAt(point, root.Field("name"))
	if got != nil {
		t.Fatalf("ConditionTypeAt(event.name) = %v, want nil from sibling proof", got)
	}
}
