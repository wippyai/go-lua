package semanticguard

import (
	"math/rand"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func TestInstantiatePreservesGuardCorrelationAndRebasesBothSides(t *testing.T) {
	relations := []factflow.BranchPathRelation{
		factflow.NewBranchPathEquality(
			pathdom.NewPlaceholder(0).Field("kind"),
			pathdom.Path{Root: "ret[0]"}.Field("kind"),
			true, false,
		),
		factflow.NewBranchPathInequality(
			pathdom.NewPlaceholder(1),
			pathdom.Path{Root: "ret[1]"},
			true, false,
		),
		factflow.NewBranchPathTypeUnmatch(
			pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1), false, true,
		),
	}
	plan, err := Compile(relations)
	if err != nil {
		t.Fatal(err)
	}
	bindings := callboundary.NewPathBindings(
		[]pathdom.Path{pathdom.NewPath(11, "arg"), pathdom.NewPath(12, "tag")},
		[]pathdom.Path{pathdom.NewPath(21, "out"), pathdom.NewPath(22, "other")},
	)

	got, ok := plan.Instantiate(true, bindings)
	if !ok || len(got) != 2 {
		t.Fatalf("true row = %d/%v, want 2/true", len(got), ok)
	}
	assertBound(t, got[0], factflow.BranchPathRelationEqual,
		pathdom.NewPath(11, "arg").Field("kind"), pathdom.NewPath(21, "out").Field("kind"))
	assertBound(t, got[1], factflow.BranchPathRelationNotEqual,
		pathdom.NewPath(12, "tag"), pathdom.NewPath(22, "other"))

	got, ok = plan.Instantiate(false, bindings)
	if !ok || len(got) != 1 {
		t.Fatalf("false row = %d/%v, want 1/true", len(got), ok)
	}
	assertBound(t, got[0], factflow.BranchPathRelationTypeUnmatch,
		pathdom.NewPath(11, "arg"), pathdom.NewPath(12, "tag"))
	if plan.Executable() {
		t.Fatal("Stage 2 structural proof must not claim executable State semantics")
	}
}

func TestInstantiateFailsClosedAtomically(t *testing.T) {
	plan, err := Compile([]factflow.BranchPathRelation{
		factflow.NewBranchPathEquality(pathdom.NewPlaceholder(0), pathdom.NewPlaceholder(1), true, false),
		factflow.NewBranchPathEquality(pathdom.NewPlaceholder(0), pathdom.Path{Root: "ret[2]"}, true, false),
		// This missing binding is inactive on the selected edge and must not fail it.
		factflow.NewBranchPathEquality(pathdom.NewPlaceholder(9), pathdom.NewPlaceholder(9), false, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	bindings := callboundary.NewPathBindings(
		[]pathdom.Path{pathdom.NewPath(1, "a"), pathdom.NewPath(2, "b")},
		[]pathdom.Path{pathdom.NewPath(3, "only")},
	)
	if got, ok := plan.Instantiate(true, bindings); ok || got != nil {
		t.Fatalf("partial correlated row escaped: %#v/%v", got, ok)
	}
}

func TestCompileAndInstantiateDifferentialAgainstProductionDTOs(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5eed))
	for iteration := 0; iteration < 5000; iteration++ {
		params := randomBindings(rng, rng.Intn(5))
		returns := randomBindings(rng, rng.Intn(4))
		bindings := callboundary.NewPathBindings(params, returns)
		count := 1 + rng.Intn(8)
		relations := make([]factflow.BranchPathRelation, count)
		for i := range relations {
			relations[i] = randomRelation(rng)
		}
		plan, err := Compile(relations)
		if err != nil {
			t.Fatalf("iteration %d: %v", iteration, err)
		}
		for _, edge := range []bool{false, true} {
			want, wantOK := oracleInstantiate(relations, edge, bindings)
			got, gotOK := plan.Instantiate(edge, bindings)
			if gotOK != wantOK || len(got) != len(want) {
				t.Fatalf("iteration %d edge %v: got %d/%v want %d/%v", iteration, edge, len(got), gotOK, len(want), wantOK)
			}
			for i := range want {
				if got[i].Kind() != want[i].Kind() || !got[i].LeftPath().Equal(want[i].LeftPath()) || !got[i].RightPath().Equal(want[i].RightPath()) {
					t.Fatalf("iteration %d edge %v relation %d differs", iteration, edge, i)
				}
			}
		}
	}
}

func TestPlanOwnsRelationPaths(t *testing.T) {
	left := pathdom.NewPlaceholder(0).Field("before")
	relation := factflow.NewBranchPathEquality(left, pathdom.NewPlaceholder(1), true, false)
	plan, err := Compile([]factflow.BranchPathRelation{relation})
	if err != nil {
		t.Fatal(err)
	}
	left.Segments[0].Name = "mutated"
	bound, ok := plan.Instantiate(true, callboundary.NewPathBindings(
		[]pathdom.Path{pathdom.NewPath(1, "x"), pathdom.NewPath(2, "y")}, nil,
	))
	if !ok || len(bound) != 1 || bound[0].LeftPath().Segments[0].Name != "before" {
		t.Fatalf("compiled plan aliases source paths: %#v/%v", bound, ok)
	}
	exposed := bound[0].LeftPath()
	exposed.Segments[0].Name = "again"
	if bound[0].LeftPath().Segments[0].Name != "before" {
		t.Fatal("bound relation exposes mutable path storage")
	}
}

func oracleInstantiate(relations []factflow.BranchPathRelation, edge bool, bindings callboundary.PathBindings) ([]BoundRelation, bool) {
	var out []BoundRelation
	for _, relation := range relations {
		if !relation.ActiveOnEdge(edge) {
			continue
		}
		left, ok := bindings.Substitute(relation.LeftPath())
		if !ok {
			return nil, false
		}
		right, ok := bindings.Substitute(relation.RightPath())
		if !ok {
			return nil, false
		}
		out = append(out, BoundRelation{kind: relation.Kind(), left: left, right: right})
	}
	return out, true
}

func randomRelation(rng *rand.Rand) factflow.BranchPathRelation {
	left, right := randomBoundaryPath(rng), randomBoundaryPath(rng)
	onTrue, onFalse := rng.Intn(2) == 0, rng.Intn(2) == 0
	switch rng.Intn(4) {
	case 0:
		return factflow.NewBranchPathEquality(left, right, onTrue, onFalse)
	case 1:
		return factflow.NewBranchPathInequality(left, right, onTrue, onFalse)
	case 2:
		return factflow.NewBranchPathTypeMatch(left, right, onTrue, onFalse)
	default:
		return factflow.NewBranchPathTypeUnmatch(left, right, onTrue, onFalse)
	}
}

func randomBoundaryPath(rng *rand.Rand) pathdom.Path {
	var p pathdom.Path
	switch rng.Intn(5) {
	case 0, 1:
		p = pathdom.NewPlaceholder(rng.Intn(7))
	case 2:
		p = pathdom.Path{Root: "ret[" + string(rune('0'+rng.Intn(6))) + "]"}
	case 3:
		p = pathdom.NewPath(pathdom.SymbolID(1+rng.Intn(30)), "local")
	default:
		// Non-canonical return-like roots are concrete syntax paths and pass through.
		p = pathdom.Path{Root: "ret[01]"}
	}
	for i, n := 0, rng.Intn(4); i < n; i++ {
		switch rng.Intn(3) {
		case 0:
			p.Segments = append(p.Segments, segment.Segment{Kind: segment.SegmentField, Name: "f"})
		case 1:
			p.Segments = append(p.Segments, segment.Segment{Kind: segment.SegmentIndexString, Name: "k"})
		default:
			p.Segments = append(p.Segments, segment.Segment{Kind: segment.SegmentIndexInt, Index: rng.Intn(9)})
		}
	}
	return p
}

func randomBindings(rng *rand.Rand, count int) []pathdom.Path {
	out := make([]pathdom.Path, count)
	for i := range out {
		if rng.Intn(8) == 0 {
			continue
		}
		out[i] = pathdom.NewPath(pathdom.SymbolID(100+i), "bound")
		if rng.Intn(2) == 0 {
			out[i] = out[i].Field("base")
		}
	}
	return out
}

func assertBound(t *testing.T, got BoundRelation, kind factflow.BranchPathRelationKind, left, right pathdom.Path) {
	t.Helper()
	if got.Kind() != kind || !got.LeftPath().Equal(left) || !got.RightPath().Equal(right) {
		t.Fatalf("got kind=%d left=%s right=%s, want kind=%d left=%s right=%s",
			got.Kind(), got.LeftPath().Key(), got.RightPath().Key(), kind, left.Key(), right.Key())
	}
}
