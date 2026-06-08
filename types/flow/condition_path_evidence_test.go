package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/constraint/theory"
)

func TestConditionPathEvidenceRegistersResolvedConstraintAndAtomEqualities(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 1}
	y := constraint.Path{Root: "y", Symbol: 2}
	z := constraint.Path{Root: "z", Symbol: 3}

	resolve := func(path constraint.Path) constraint.PathKey {
		if path.IsEmpty() {
			return ""
		}
		return constraint.PathKey("resolved:" + path.Key())
	}

	evidence := newConditionPathEvidence(
		[]constraint.Atom{
			constraint.AtomEq(constraint.TermVar("atom:x"), constraint.TermVar("atom:y")),
		},
		[]constraint.Constraint{
			constraint.NewEqPath(x, y),
			constraint.Truthy{Path: z},
		},
		resolve,
	)

	graph := theory.NewEGraph()
	evidence.RegisterInto(graph)

	if graph.Find("resolved:"+x.Key()) != graph.Find("resolved:"+y.Key()) {
		t.Fatal("resolved EqPath endpoints were not unified")
	}
	if graph.Find("atom:x") != graph.Find("atom:y") {
		t.Fatal("atom equality endpoints were not unified")
	}

	keys := evidence.Keys()
	want := map[constraint.PathKey]bool{
		"resolved:" + x.Key(): true,
		"resolved:" + y.Key(): true,
		"resolved:" + z.Key(): true,
		"atom:x":              true,
		"atom:y":              true,
	}
	if len(keys) != len(want) {
		t.Fatalf("Keys() length = %d, want %d: %v", len(keys), len(want), keys)
	}
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected evidence key %q in %v", key, keys)
		}
	}
}

func TestConditionPathEvidenceFallsBackToPathKeys(t *testing.T) {
	path := constraint.Path{Root: "value", Symbol: 1}.Append(
		constraint.Segment{Kind: constraint.SegmentField, Name: "kind"},
	)

	evidence := newConstraintPathEvidence(constraint.Truthy{Path: path}, func(path constraint.Path) constraint.PathKey {
		if path.IsEmpty() {
			return ""
		}
		return path.Key()
	})

	keys := evidence.Keys()
	want := map[constraint.PathKey]bool{
		constraint.Path{Root: "value", Symbol: 1}.Key(): true,
		path.Key(): true,
	}
	if len(keys) != len(want) {
		t.Fatalf("Keys() length = %d, want %d: %v", len(keys), len(want), keys)
	}
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected fallback key %q in %v", key, keys)
		}
	}
}
