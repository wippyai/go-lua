package merge_test

import (
	"reflect"
	"testing"

	physicalmerge "github.com/wippyai/go-lua/analysis/engine/relation/operator/merge"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

func TestRecomputeAffectedOverlappingPathsOnceAndIncludesUnchangedBranch(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, nodeOK := fixture.MergeNode()
	if !nodeOK {
		t.Fatal("merge node")
	}
	binding, bindingOK := node.Merge()
	if !bindingOK || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	values := input.Tuples()
	if len(values) == 0 {
		t.Fatal("input values")
	}
	changed, changedOK := tuple.PreserveRange(fixture.Mounted(), input, input.Scope(), []tuple.Tuple{values[0]})
	if !changedOK {
		t.Fatal("changed path")
	}
	// Both paths report the same affected key. The second authored branch is
	// deliberately the complete successor vector, so the result proves that
	// recomputation includes an unchanged alternative rather than folding only
	// the changed path.
	result, resultOK := physicalmerge.RecomputeAffected(
		binding,
		fixture.Mounted(),
		[]tuple.Batch{changed, changed},
		[][]tuple.Batch{{input}, {input}},
	)
	if !resultOK || len(result) != 1 || result[0].Len() != 1 {
		t.Fatalf("affected result=(%v,%d)", resultOK, len(result))
	}
	got, gotOK := result[0].At(0)
	if !gotOK {
		t.Fatal("affected tuple")
	}
	want, wantOK := tuple.Merge(fixture.Mounted(), []tuple.Tuple{values[0], values[0]}, binding.Key().KeyColumns())
	if !wantOK || !got.Same(want) {
		t.Fatal("unchanged authored alternative was not included")
	}
}

func TestRecomputeAffectedRefusesWithoutSealedSuccessorAlternatives(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	node, nodeOK := fixture.MergeNode()
	if !nodeOK {
		t.Fatal("merge node")
	}
	binding, bindingOK := node.Merge()
	if !bindingOK || !binding.Available() {
		t.Fatal("merge binding")
	}
	input := inputSourceAtRoot(t, fixture, fixture.LeftRoot())
	// An unavailable alternative cannot trigger a relation-wide fallback. The
	// function has no Reader/scan argument by construction and must fail closed.
	var unavailable tuple.Batch
	if result, resultOK := physicalmerge.RecomputeAffected(binding, fixture.Mounted(), []tuple.Batch{input}, [][]tuple.Batch{{unavailable}}); resultOK || result != nil {
		t.Fatal("unsealed successor alternative accepted")
	}
	if typ := reflect.TypeOf(physicalmerge.RecomputeAffected); typ.NumIn() != 4 || typ.In(2) != reflect.TypeOf([]tuple.Batch{}) || typ.In(3) != reflect.TypeOf([][]tuple.Batch{}) {
		t.Fatalf("unexpected affected API: %v", typ)
	}
}
