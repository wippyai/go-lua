package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestProjectFunctionRefsByReferencePathsSeparatesExactAndSubtree(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(10), "root")
	other := constraint.NewPath(cfg.SymbolID(20), "other")
	refs := WithFunctionRef(nil, root.Field("used").Key(), FunctionRefSetOf(FunctionRef{GraphID: 1}))
	refs = WithFunctionRef(refs, root.Field("used").Field("nested").Key(), FunctionRefSetOf(FunctionRef{GraphID: 2}))
	refs = WithFunctionRef(refs, root.Field("forwarded").Field("call").Key(), FunctionRefSetOf(FunctionRef{GraphID: 3}))
	refs = WithFunctionRef(refs, root.Field("unused").Key(), FunctionRefSetOf(FunctionRef{GraphID: 4}))
	refs = WithFunctionRef(refs, other.Field("used").Key(), FunctionRefSetOf(FunctionRef{GraphID: 5}))

	got := ProjectFunctionRefsByReferencePaths(refs, ReferencePathProjection{
		Exact:    []constraint.Path{root.Field("used")},
		Subtrees: []constraint.Path{root.Field("forwarded")},
	})

	if _, ok := FunctionRefAt(got, root.Field("used").Key()); !ok {
		t.Fatal("exact path was not retained")
	}
	if _, ok := FunctionRefAt(got, root.Field("used").Field("nested").Key()); ok {
		t.Fatal("descendant of exact-only path was retained")
	}
	if _, ok := FunctionRefAt(got, root.Field("forwarded").Field("call").Key()); !ok {
		t.Fatal("descendant of subtree path was not retained")
	}
	if _, ok := FunctionRefAt(got, root.Field("unused").Key()); ok {
		t.Fatal("unprojected same-root path was retained")
	}
	if _, ok := FunctionRefAt(got, other.Field("used").Key()); ok {
		t.Fatal("unprojected other-root path was retained")
	}
}

func TestProjectClosureRefsByReferencePathsSeparatesExactAndSubtree(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(30), "root")
	closure := ClosureRefOf(FunctionRef{GraphID: 11}, CaptureCellsDomain.Bottom(), nil)
	refs := WithClosureRef(nil, root.Field("used").Key(), ClosureRefSetOf(closure))
	refs = WithClosureRef(refs, root.Field("used").Field("nested").Key(), ClosureRefSetOf(closure))
	refs = WithClosureRef(refs, root.Field("forwarded").Field("call").Key(), ClosureRefSetOf(closure))
	refs = WithClosureRef(refs, root.Field("unused").Key(), ClosureRefSetOf(closure))

	got := ProjectClosureRefsByReferencePaths(refs, ReferencePathProjection{
		Exact:    []constraint.Path{root.Field("used")},
		Subtrees: []constraint.Path{root.Field("forwarded")},
	})

	if _, ok := ClosureRefAt(got, root.Field("used").Key()); !ok {
		t.Fatal("exact path was not retained")
	}
	if _, ok := ClosureRefAt(got, root.Field("used").Field("nested").Key()); ok {
		t.Fatal("descendant of exact-only path was retained")
	}
	if _, ok := ClosureRefAt(got, root.Field("forwarded").Field("call").Key()); !ok {
		t.Fatal("descendant of subtree path was not retained")
	}
	if _, ok := ClosureRefAt(got, root.Field("unused").Key()); ok {
		t.Fatal("unprojected same-root path was retained")
	}
}
