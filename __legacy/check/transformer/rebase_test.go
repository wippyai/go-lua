package transformer

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

func TestRebaseTermDAGsPreservesNamespacesCorrelationAndCanonicalIdentity(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	calleeShape := Shape{Params: 1, Captures: 1, Globals: 1, Ambients: 1}
	callerShape := Shape{Params: 4}
	cp := Root{Kind: RootParam, Index: 0}
	cc := Root{Kind: RootCapture, Index: 0}
	cg := Root{Kind: RootGlobal, Index: 0}
	ca := Root{Kind: RootAmbient, Index: 0}
	p0 := caller.Root(Root{Kind: RootParam, Index: 0})
	p1 := caller.Root(Root{Kind: RootParam, Index: 1})
	p2 := caller.Root(Root{Kind: RootParam, Index: 2})
	p3 := caller.Root(Root{Kind: RootParam, Index: 3})
	constant := callee.Constant(typevalue.LiteralString(reg, "fixed"))
	joined := callee.JoinValue(callee.Root(cp), callee.Root(cc), callee.Root(cg), callee.Root(ca), constant)
	guard := callee.And(
		callee.Truthy(callee.Root(cp)),
		callee.Or(callee.Falsy(callee.Root(cc)), callee.Truthy(joined)),
	)
	basePath := caller.Path(Root{Kind: RootParam, Index: 1}, segment.Segment{Kind: segment.SegmentField, Name: "base"})
	sourcePath := callee.Path(cc, segment.Segment{Kind: segment.SegmentField, Name: "leaf"})
	ambientPath := caller.Path(Root{Kind: RootParam, Index: 3}, segment.Segment{Kind: segment.SegmentField, Name: "ambient"})
	sourceAmbientPath := callee.Path(ca, segment.Segment{Kind: segment.SegmentField, Name: "leaf"})
	bindings, err := NewTermRootBindings(calleeShape, callerShape, []ValueTerm{p0, p1, p2, p3}, []PathTerm{0, basePath, 0, ambientPath})
	if err != nil {
		t.Fatal(err)
	}
	input := TermRebaseInput{Values: []ValueTerm{joined, constant}, Paths: []PathTerm{sourcePath, sourceAmbientPath}, Guards: []Guard{guard}}
	got, err := RebaseTermDAGs(caller, callee, bindings, input)
	if err != nil {
		t.Fatal(err)
	}
	wantJoin := caller.JoinValue(p0, p1, p2, p3, caller.Constant(typevalue.LiteralString(reg, "fixed")))
	if got.Values[0] != wantJoin || got.Values[1] != caller.Constant(typevalue.LiteralString(reg, "fixed")) {
		t.Fatalf("rebased values = %v, want canonical join %d", got.Values, wantJoin)
	}
	wantGuard := caller.And(caller.Truthy(p0), caller.Or(caller.Falsy(p1), caller.Truthy(wantJoin)))
	if got.Guards[0] != wantGuard {
		t.Fatalf("rebased guard = %d, want correlated canonical guard %d", got.Guards[0], wantGuard)
	}
	wantPath := caller.Path(Root{Kind: RootParam, Index: 1},
		segment.Segment{Kind: segment.SegmentField, Name: "base"},
		segment.Segment{Kind: segment.SegmentField, Name: "leaf"},
	)
	if got.Paths[0] != wantPath {
		t.Fatalf("rebased path = %d, want composed canonical path %d", got.Paths[0], wantPath)
	}
	wantAmbientPath := caller.Path(Root{Kind: RootParam, Index: 3},
		segment.Segment{Kind: segment.SegmentField, Name: "ambient"},
		segment.Segment{Kind: segment.SegmentField, Name: "leaf"},
	)
	if got.Paths[1] != wantAmbientPath {
		t.Fatalf("rebased ambient path = %d, want composed canonical path %d", got.Paths[1], wantAmbientPath)
	}
	// Equal numeric source indices in distinct namespaces remained distinct.
	if p0 == p1 || p0 == p2 || p0 == p3 || p1 == p2 || p1 == p3 || p2 == p3 {
		t.Fatal("test assumption: caller namespace bindings unexpectedly alias")
	}
	again, err := RebaseTermDAGs(caller, callee, bindings, input)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("repeat import lost hash-consed identity: %#v, %v", again, err)
	}
}

func TestRebaseTermDAGsFailsAtomicallyForMissingBinding(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 2}
	bound := caller.Root(Root{Kind: RootParam, Index: 0})
	missing := callee.Root(Root{Kind: RootParam, Index: 1})
	constant := callee.Constant(typevalue.LiteralString(reg, "must-not-publish"))
	join := callee.JoinValue(constant, missing)
	bindings, err := NewTermRootBindings(shape, shape, []ValueTerm{bound, bound}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bindings.values[1] = 0 // corrupt the private snapshot to exercise fail-closed validation
	beforeValues, beforePaths, beforeGuards := len(caller.values), len(caller.paths), len(caller.guards)
	got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{join}})
	if err == nil || len(got.Values) != 0 || len(got.Paths) != 0 || len(got.Guards) != 0 {
		t.Fatalf("missing binding partially published: %#v, %v", got, err)
	}
	if len(caller.values) != beforeValues || len(caller.paths) != beforePaths || len(caller.guards) != beforeGuards {
		t.Fatal("failed transaction mutated caller arena")
	}
}

func TestRebaseTermDAGsRejectsNestedCellResultWithoutPartialOutput(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	calleeParam := callee.Root(Root{Kind: RootParam})
	cell := callee.CellResultValue(CellRef{Function: 9, Slot: 2}, calleeParam)
	join := callee.JoinValue(callee.Constant(product.Top()), cell)
	guard := callee.Truthy(join)
	callerParam := caller.Root(Root{Kind: RootParam})
	bindings, _ := NewTermRootBindings(shape, shape, []ValueTerm{callerParam}, nil)
	before := len(caller.values)
	got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{
		Values: []ValueTerm{callee.Constant(typevalue.LiteralString(reg, "earlier"))},
		Guards: []Guard{guard},
	})
	if err == nil || !reflect.DeepEqual(got, TermRebaseOutput{}) {
		t.Fatalf("CellResult partially published: %#v, %v", got, err)
	}
	if len(caller.values) != before {
		t.Fatal("CellResult failure mutated caller arena")
	}
}

func TestRebaseTermDAGsRejectsInvalidAndCyclicSourceDAGs(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	callerParam := caller.Root(Root{Kind: RootParam})
	bindings, _ := NewTermRootBindings(shape, shape, []ValueTerm{callerParam}, nil)
	root := callee.Root(Root{Kind: RootParam})
	constant := callee.Constant(product.Top())
	join := callee.JoinValue(root, constant)
	callee.values[join].args[0] = join // corrupt a private source node into a cycle
	before := len(caller.values)
	if got, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{join}}); err == nil || !reflect.DeepEqual(got, TermRebaseOutput{}) {
		t.Fatalf("cyclic value DAG accepted: %#v, %v", got, err)
	}
	if len(caller.values) != before {
		t.Fatal("cyclic source failure mutated caller arena")
	}

	guardCallee := NewArena(reg)
	guardRoot := guardCallee.Root(Root{Kind: RootParam})
	guardOther := guardCallee.Constant(typevalue.LiteralBool(reg, true))
	cyclicGuard := guardCallee.And(guardCallee.Truthy(guardRoot), guardCallee.Falsy(guardOther))
	guardCallee.guards[cyclicGuard].args[0] = cyclicGuard
	if got, err := RebaseTermDAGs(caller, guardCallee, bindings, TermRebaseInput{Guards: []Guard{cyclicGuard}}); err == nil || !reflect.DeepEqual(got, TermRebaseOutput{}) {
		t.Fatalf("cyclic guard DAG accepted: %#v, %v", got, err)
	}
	if len(caller.values) != before {
		t.Fatal("cyclic guard failure mutated caller arena")
	}

	// A malformed path root is rejected before suffix import.
	badPath := callee.Path(Root{Kind: RootParam, Index: 0})
	callee.paths[badPath].root.Index = 3
	if _, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Paths: []PathTerm{badPath}}); err == nil {
		t.Fatal("invalid source path root accepted")
	}
}

func TestRebaseTermDAGsDoesNotMutateSourceArenaOrBorrowBindings(t *testing.T) {
	reg := standard.Registry()
	callee, caller := NewArena(reg), NewArena(reg)
	shape := Shape{Params: 1}
	sourceRoot := Root{Kind: RootParam}
	sourceValue := callee.JoinValue(callee.Root(sourceRoot), callee.Constant(product.Top()))
	sourcePath := callee.Path(sourceRoot, segment.Segment{Kind: segment.SegmentField, Name: "x"})
	sourceGuard := callee.Truthy(sourceValue)
	valuesSnapshot := append([]valueNode(nil), callee.values...)
	pathsSnapshot := append([]pathNode(nil), callee.paths...)
	guardsSnapshot := append([]guardNode(nil), callee.guards...)
	packedValues := []ValueTerm{caller.Root(Root{Kind: RootParam})}
	packedPaths := []PathTerm{caller.Path(Root{Kind: RootParam})}
	bindings, _ := NewTermRootBindings(shape, shape, packedValues, packedPaths)
	packedValues[0], packedPaths[0] = 0, 0
	if _, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{
		Values: []ValueTerm{sourceValue}, Paths: []PathTerm{sourcePath}, Guards: []Guard{sourceGuard},
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(callee.values, valuesSnapshot) || !reflect.DeepEqual(callee.paths, pathsSnapshot) || !reflect.DeepEqual(callee.guards, guardsSnapshot) {
		t.Fatal("rebasing mutated source arena")
	}
}

func TestRebaseTermDAGsRejectsForeignRegistry(t *testing.T) {
	// Distinct registries can assign different meanings to product lanes, so
	// constants must never cross this boundary based on a coincidental hash.
	callee := NewArena(standard.Registry())
	foreign, err := standard.RegistryWithAxes()
	if err != nil {
		t.Fatal(err)
	}
	caller := NewArena(foreign)
	bindings, _ := NewTermRootBindings(Shape{}, Shape{}, nil, nil)
	if _, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{callee.Constant(product.Top())}}); err == nil {
		t.Fatal("foreign registry accepted")
	}
}
