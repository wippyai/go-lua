package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestReferencePathProjectionNormalizesAddressView(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(7), "root")
	exact := root.Field("fn")
	subtree := root.Field("module")

	projection := NewReferencePathProjection(
		[]constraint.Path{exact},
		[]constraint.Path{subtree},
	)
	addresses := projection.addressProjection()

	exactAddr, _ := StableAddressOfPath(exact)
	if len(addresses.ExactKeys) != 1 || addresses.ExactKeys[0] != exactAddr.Key() {
		t.Fatalf("exact keys = %#v, want %q", addresses.ExactKeys, exactAddr.Key())
	}
	if !addresses.contains(exactAddr.Key()) {
		t.Fatal("normalized projection should contain exact address key")
	}
	nested, _ := StableAddressOfPath(exact.Field("nested"))
	if addresses.contains(nested.Key()) {
		t.Fatal("exact projection should not contain descendant key")
	}
	subtreeNested, _ := StableAddressOfPath(subtree.Field("nested"))
	if !addresses.contains(subtreeNested.Key()) {
		t.Fatal("subtree projection should contain descendant key")
	}
}

func TestReferencePathProjectionConstructorIsDefensive(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(8), "root").Field("kept")
	want := constraint.NewPath(cfg.SymbolID(8), "root").Field("kept")
	paths := []constraint.Path{path}

	projection := NewReferencePathProjection(paths, nil)
	paths[0].Segments[0].Name = "mutated"

	if !projection.Exact[0].Equal(want) {
		t.Fatalf("projection path was mutated through caller slice: %v", projection.Exact[0])
	}
	addr, _ := StableAddressOfPath(want)
	if !projection.addressProjection().contains(addr.Key()) {
		t.Fatal("normalized address view changed after caller mutation")
	}
}

func TestProjectFunctionRefsByReferencePathsSeparatesExactAndSubtree(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(10), "root")
	other := constraint.NewPath(cfg.SymbolID(20), "other")
	refs := WithFunctionRefPath(nil, root.Field("used"), FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRefPath(refs, root.Field("used").Field("nested"), FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRefPath(refs, root.Field("forwarded").Field("call"), FunctionRefSetOf(FunctionRef{GraphID: 3}))
	refs = WithFunctionRefPath(refs, root.Field("unused"), FunctionRefSetOf(FunctionRef{GraphID: 4}))
	refs = WithFunctionRefPath(refs, other.Field("used"), FunctionRefSetOf(FunctionRef{GraphID: 5}))

	got := ProjectFunctionRefsByReferencePaths(refs, NewReferencePathProjection(
		[]constraint.Path{root.Field("used")},
		[]constraint.Path{root.Field("forwarded")},
	))

	if _, ok := FunctionRefAtPath(got, root.Field("used")); !ok {
		t.Fatal("exact path was not retained")
	}
	if _, ok := FunctionRefAtPath(got, root.Field("used").Field("nested")); ok {
		t.Fatal("descendant of exact-only path was retained")
	}
	if _, ok := FunctionRefAtPath(got, root.Field("forwarded").Field("call")); !ok {
		t.Fatal("descendant of subtree path was not retained")
	}
	if _, ok := FunctionRefAtPath(got, root.Field("unused")); ok {
		t.Fatal("unprojected same-root path was retained")
	}
	if _, ok := FunctionRefAtPath(got, other.Field("used")); ok {
		t.Fatal("unprojected other-root path was retained")
	}
}

func TestProjectClosureRefsByReferencePathsSeparatesExactAndSubtree(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(30), "root")
	closure := ClosureRefOf(FunctionRef{GraphID: 11}, CaptureCellsDomain.Bottom(), nil)
	refs := WithClosureRefAddress(nil, testStableAddressPath(t, root.Field("used")), ClosureRefSetOf(closure))
	refs = WithClosureRefAddress(refs, testStableAddressPath(t, root.Field("used").Field("nested")), ClosureRefSetOf(closure))
	refs = WithClosureRefAddress(refs, testStableAddressPath(t, root.Field("forwarded").Field("call")), ClosureRefSetOf(closure))
	refs = WithClosureRefAddress(refs, testStableAddressPath(t, root.Field("unused")), ClosureRefSetOf(closure))

	got := ProjectClosureRefsByReferencePaths(refs, NewReferencePathProjection(
		[]constraint.Path{root.Field("used")},
		[]constraint.Path{root.Field("forwarded")},
	))

	if _, ok := ClosureRefAtPath(got, root.Field("used")); !ok {
		t.Fatal("exact path was not retained")
	}
	if _, ok := ClosureRefAtPath(got, root.Field("used").Field("nested")); ok {
		t.Fatal("descendant of exact-only path was retained")
	}
	if _, ok := ClosureRefAtPath(got, root.Field("forwarded").Field("call")); !ok {
		t.Fatal("descendant of subtree path was not retained")
	}
	if _, ok := ClosureRefAtPath(got, root.Field("unused")); ok {
		t.Fatal("unprojected same-root path was retained")
	}
}
