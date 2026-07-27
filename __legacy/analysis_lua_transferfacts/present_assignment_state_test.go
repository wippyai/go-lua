package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestTransferPresentAssignmentStateReusesNoOpAndClonesBeforeSiblingMutation(t *testing.T) {
	reg := standard.Registry()
	stats := &presentAssignmentTransferStats{}
	l := &lowerer{
		registry:               reg,
		typeValues:             typevalue.NewCache(),
		presentAssignmentStats: stats,
	}
	x := path.NewPath(symbol.ID(1), "x")
	key, _ := presentAssignmentKeyForPath(x)
	base := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{
		key: {path: x, value: product.Top(), hasValue: true},
	})

	unchanged := l.transferPresentAssignmentState(&factflow.FactsInput{}, cfg.Point(1), base, false)
	if stats.stateClones != 0 || stats.reusedStates != 1 {
		t.Fatalf("no-op stats = clones %d, reused %d; want 0/1", stats.stateClones, stats.reusedStates)
	}
	if unchanged.identity != base.identity {
		t.Fatal("no-op transfer did not preserve immutable state identity")
	}

	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("literal source shape is invalid")
	}
	source, ok := factflow.NewStringLiteralValueSource("new", 0, 0, 0, shape)
	if !ok {
		t.Fatal("literal source is invalid")
	}
	input := &factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			2: factflow.NewPathAssignment(x, source),
		},
	}
	changed := l.transferPresentAssignmentState(input, cfg.Point(2), unchanged, true)
	if stats.stateClones != 1 {
		t.Fatalf("mutation clones = %d, want exactly 1", stats.stateClones)
	}
	if got, _ := base.get(key); got.value != product.Top() || got.fromBranch {
		t.Fatalf("shared sibling input was mutated: %#v", got)
	}
	if got, _ := changed.get(key); !got.hasValue || !got.fromBranch || got.value == product.Top() {
		t.Fatalf("reassignment was not recorded independently: %#v", got)
	}
}

func TestIntersectPresentAssignmentStateReusesOperandsAndConvergesByIdentity(t *testing.T) {
	x := path.NewPath(symbol.ID(1), "x")
	y := path.NewPath(symbol.ID(2), "y")
	xKey, _ := presentAssignmentKeyForPath(x)
	yKey, _ := presentAssignmentKeyForPath(y)
	common := conditionalAssignment{path: x, value: product.Top(), hasValue: true}
	left := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{
		xKey: common,
		yKey: {path: y},
	})
	right := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{xKey: common})

	if got := intersectPresentAssignmentState(left, left); got.identity != left.identity {
		t.Fatal("self intersection did not reuse its operand")
	}
	meet := intersectPresentAssignmentState(left, right)
	if meet.identity != right.identity {
		t.Fatal("intersection equal to right operand did not reuse it")
	}
	for i := 0; i < 100; i++ {
		next := intersectPresentAssignmentState(meet, left)
		if next.identity != meet.identity || !presentAssignmentStateEqual(meet, next) {
			t.Fatalf("cycle iteration %d lost converged identity", i)
		}
		meet = next
	}
}

func TestPresentAssignmentStateStructuralEqualityDoesNotRequireSharedIdentity(t *testing.T) {
	x := path.NewPath(symbol.ID(1), "x")
	key, _ := presentAssignmentKeyForPath(x)
	assignment := conditionalAssignment{path: x, value: product.Top(), hasValue: true}
	a := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{key: assignment})
	b := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{key: assignment})
	if a.identity == b.identity {
		t.Fatal("independent states unexpectedly share identity")
	}
	if !presentAssignmentStateEqual(a, b) {
		t.Fatal("independent semantic equals must retain structural fallback")
	}
}

func TestIntersectPresentAssignmentStatePreservesLeftBranchProvenance(t *testing.T) {
	x := path.NewPath(symbol.ID(1), "x")
	y := path.NewPath(symbol.ID(2), "y")
	xKey, _ := presentAssignmentKeyForPath(x)
	yKey, _ := presentAssignmentKeyForPath(y)
	left := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{
		xKey: {path: x, fromBranch: true},
		yKey: {path: y},
	})
	right := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{
		xKey: {path: x, fromBranch: false},
	})
	meet := intersectPresentAssignmentState(left, right)
	got, ok := meet.get(xKey)
	if !ok || !got.fromBranch {
		t.Fatal("intersection changed the historical left-biased branch provenance")
	}
	if meet.identity == right.identity {
		t.Fatal("intersection unsafely reused operand with different representation")
	}
}

func TestTransferPresentAssignmentStateInvalidationClonesOnceAndPreservesSharedInput(t *testing.T) {
	stats := &presentAssignmentTransferStats{}
	l := &lowerer{presentAssignmentStats: stats}
	xChild := path.NewPath(symbol.ID(1), "x").Field("child")
	y := path.NewPath(symbol.ID(2), "y")
	xKey, _ := presentAssignmentKeyForPath(xChild)
	yKey, _ := presentAssignmentKeyForPath(y)
	base := newPresentAssignmentState(map[presentAssignmentKey]conditionalAssignment{
		xKey: {path: xChild},
		yKey: {path: y},
	})
	input := &factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			1: factflow.NewPathAssignment(xChild.RootOnly(), factflow.ValueSource{Kind: factflow.ValueSourceUnknown}),
		},
	}

	changed := l.transferPresentAssignmentState(input, cfg.Point(1), base, false)
	if stats.stateClones != 1 {
		t.Fatalf("invalidation clones = %d, want exactly 1", stats.stateClones)
	}
	if _, ok := changed.get(xKey); ok {
		t.Fatal("overlapping descendant survived invalidation")
	}
	if _, ok := changed.get(yKey); !ok {
		t.Fatal("non-overlapping sibling was invalidated")
	}
	if _, ok := base.get(xKey); !ok {
		t.Fatal("shared incoming state was alias-mutated")
	}
}

func BenchmarkTransferPresentAssignmentStateNoOp(b *testing.B) {
	stats := &presentAssignmentTransferStats{}
	l := &lowerer{presentAssignmentStats: stats}
	entries := make(map[presentAssignmentKey]conditionalAssignment, 64)
	for i := 1; i <= 64; i++ {
		p := path.NewPath(symbol.ID(i), "value")
		key, _ := presentAssignmentKeyForPath(p)
		entries[key] = conditionalAssignment{path: p}
	}
	state := newPresentAssignmentState(entries)
	input := &factflow.FactsInput{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := l.transferPresentAssignmentState(input, cfg.Point(1), state, false); got.len() != state.len() {
			b.Fatal("no-op transfer changed state")
		}
	}
	b.StopTimer()
	if stats.stateClones != 0 {
		b.Fatalf("no-op transfers cloned %d states", stats.stateClones)
	}
}

func BenchmarkPresentAssignmentStateEqualIdentity(b *testing.B) {
	entries := make(map[presentAssignmentKey]conditionalAssignment, 64)
	for i := 1; i <= 64; i++ {
		p := path.NewPath(symbol.ID(i), "value")
		key, _ := presentAssignmentKeyForPath(p)
		entries[key] = conditionalAssignment{path: p}
	}
	state := newPresentAssignmentState(entries)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !presentAssignmentStateEqual(state, state) {
			b.Fatal("state is not equal to itself")
		}
	}
}

func BenchmarkIntersectPresentAssignmentStateIdentity(b *testing.B) {
	entries := make(map[presentAssignmentKey]conditionalAssignment, 64)
	for i := 1; i <= 64; i++ {
		p := path.NewPath(symbol.ID(i), "value")
		key, _ := presentAssignmentKeyForPath(p)
		entries[key] = conditionalAssignment{path: p}
	}
	state := newPresentAssignmentState(entries)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if got := intersectPresentAssignmentState(state, state); got.identity != state.identity {
			b.Fatal("intersection did not reuse state")
		}
	}
}
