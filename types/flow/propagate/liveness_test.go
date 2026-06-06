package propagate

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConditionProjectorCacheIsPointScoped(t *testing.T) {
	a := constraint.Path{Root: "a", Symbol: 101}
	b := constraint.Path{Root: "b", Symbol: 102}
	c := constraint.Path{Root: "c", Symbol: 103}
	d := constraint.Path{Root: "d", Symbol: 104}

	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1: {Kind: cfg.NodeEntry, Point: 1},
			2: {Kind: cfg.NodeAssign, Point: 2},
			3: {Kind: cfg.NodeAssign, Point: 3},
		},
		preds: map[cfg.Point][]cfg.Point{
			1: {},
			2: {1},
			3: {1},
		},
		succs: map[cfg.Point][]cfg.Point{
			1: {2, 3},
			2: {},
			3: {},
		},
		rpo: []cfg.Point{1, 2, 3},
	}

	projector := NewConditionProjector(&Inputs{
		Graph: g,
		Demand: &Demand{Uses: map[cfg.Point][]constraint.Path{
			2: {a, c},
			3: {b, d},
		}},
	})

	cond := constraint.FromDisjuncts([][]constraint.Constraint{
		constraint.NewConjunction(constraint.Truthy{Path: a}, constraint.Truthy{Path: b}),
		constraint.NewConjunction(constraint.Truthy{Path: c}, constraint.Truthy{Path: d}),
	})

	wantAt2 := constraint.FromDisjuncts([][]constraint.Constraint{
		constraint.NewConjunction(constraint.Truthy{Path: a}),
		constraint.NewConjunction(constraint.Truthy{Path: c}),
	})
	gotAt2 := projector.Project(2, cond)
	if !gotAt2.Equals(wantAt2) {
		t.Fatalf("point 2 projection mismatch:\n  got  %v\n  want %v", gotAt2, wantAt2)
	}
	gotAt2Again := projector.Project(2, cond)
	if !gotAt2Again.Equals(wantAt2) {
		t.Fatalf("cached point 2 projection mismatch:\n  got  %v\n  want %v", gotAt2Again, wantAt2)
	}

	wantAt3 := constraint.FromDisjuncts([][]constraint.Constraint{
		constraint.NewConjunction(constraint.Truthy{Path: b}),
		constraint.NewConjunction(constraint.Truthy{Path: d}),
	})
	gotAt3 := projector.Project(3, cond)
	if !gotAt3.Equals(wantAt3) {
		t.Fatalf("point 3 projection reused the wrong point cache:\n  got  %v\n  want %v", gotAt3, wantAt3)
	}
}

func TestConditionProjectorFieldPathLivenessIsVersionSensitive(t *testing.T) {
	xV1 := constraint.Path{Root: "x", Symbol: 201, Version: 1}
	xV2 := constraint.Path{Root: "x", Symbol: 201, Version: 2}
	oldKind := constraint.FieldEquals{Target: xV1, Field: "kind", Value: typ.LiteralString("old")}
	newKind := constraint.FieldEquals{Target: xV2, Field: "kind", Value: typ.LiteralString("new")}

	projector := NewConditionProjector(&Inputs{
		Graph: singlePointGraph(1),
		Demand: &Demand{Uses: map[cfg.Point][]constraint.Path{
			1: {xV2.Field("value")},
		}},
	})

	cond := constraint.FromConstraints(oldKind, newKind)
	want := constraint.FromConstraints(newKind)
	got := projector.Project(1, cond)
	if !got.Equals(want) {
		t.Fatalf("version-sensitive field projection mismatch:\n  got  %v\n  want %v", got, want)
	}
}

func TestConditionProjectorKeepsSiblingDiscriminantSameVersion(t *testing.T) {
	xV1 := constraint.Path{Root: "x", Symbol: 202, Version: 1}
	kind := constraint.FieldEquals{Target: xV1, Field: "kind", Value: typ.LiteralString("event")}

	projector := NewConditionProjector(&Inputs{
		Graph: singlePointGraph(1),
		Demand: &Demand{Uses: map[cfg.Point][]constraint.Path{
			1: {xV1.Field("value")},
		}},
	})

	cond := constraint.FromConstraints(kind)
	got := projector.Project(1, cond)
	if !got.Equals(cond) {
		t.Fatalf("same-version sibling discriminant was dropped:\n  got  %v\n  want %v", got, cond)
	}
}

func TestConditionProjectorProjectOutRebasesPhiTargetDemandToOperand(t *testing.T) {
	const (
		pred cfg.Point    = 1
		join cfg.Point    = 2
		sym  cfg.SymbolID = 301
	)
	operandVersion := cfg.Version{Root: "x", Symbol: sym, ID: 2}
	targetVersion := cfg.Version{Root: "x", Symbol: sym, ID: 3}
	operand := constraint.Path{Root: "x", Symbol: sym, Version: operandVersion.ID}
	target := constraint.Path{Root: "x", Symbol: sym, Version: targetVersion.ID}
	fact := constraint.FromConstraints(constraint.FieldEquals{
		Target: operand,
		Field:  "target",
		Value:  typ.LiteralString("node"),
	})

	projector := NewConditionProjector(&Inputs{
		Graph: &mockGraph{
			entry: pred,
			nodes: map[cfg.Point]*cfg.Node{
				pred: {Kind: cfg.NodeAssign, Point: pred},
				join: {Kind: cfg.NodeAssign, Point: join},
			},
			preds: map[cfg.Point][]cfg.Point{
				pred: {},
				join: {pred},
			},
			succs: map[cfg.Point][]cfg.Point{
				pred: {join},
				join: {},
			},
			rpo: []cfg.Point{pred, join},
			phis: []cfg.PhiNode{{
				Point:  join,
				Target: targetVersion,
				Operands: []cfg.PhiOperand{{
					From:    pred,
					Version: operandVersion,
				}},
			}},
		},
		Demand: &Demand{
			Uses: map[cfg.Point][]constraint.Path{
				join: {target.Field("target")},
			},
			Defs: map[cfg.Point][]cfg.Version{
				pred: {operandVersion},
			},
		},
	})

	if got := projector.ProjectOut(pred, fact); !got.Equals(fact) {
		t.Fatalf("ProjectOut dropped phi operand fact:\n  got  %v\n  want %v", got, fact)
	}
	if got := projector.Project(pred, fact); !got.IsTrue() {
		t.Fatalf("Project retained post-def fact in live-in phase: %v", got)
	}
}

func TestVersionedDemandKeyUsesCanonicalSuffixParser(t *testing.T) {
	stripped := constraint.PathKey(`sym401["a\"b"].value`)
	got := versionedDemandKey(stripped, cfg.SymbolID(401), 7)
	want := constraint.PathKey(`sym401@7["a\"b"].value`)
	if got != want {
		t.Fatalf("versionedDemandKey = %q, want %q", got, want)
	}
}

func singlePointGraph(p cfg.Point) *mockGraph {
	return &mockGraph{
		entry: p,
		nodes: map[cfg.Point]*cfg.Node{
			p: {Kind: cfg.NodeEntry, Point: p},
		},
		preds: map[cfg.Point][]cfg.Point{
			p: {},
		},
		succs: map[cfg.Point][]cfg.Point{
			p: {},
		},
		rpo: []cfg.Point{p},
	}
}
