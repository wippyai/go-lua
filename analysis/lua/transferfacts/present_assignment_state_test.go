package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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
	base := presentAssignmentState{
		key: {path: x, value: product.Top(), hasValue: true},
	}

	unchanged := l.transferPresentAssignmentState(&factflow.FactsInput{}, cfg.Point(1), base, false)
	if stats.stateClones != 0 || stats.reusedStates != 1 {
		t.Fatalf("no-op stats = clones %d, reused %d; want 0/1", stats.stateClones, stats.reusedStates)
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
	if got := base[key]; got.value != product.Top() || got.fromBranch {
		t.Fatalf("shared sibling input was mutated: %#v", got)
	}
	if got := changed[key]; !got.hasValue || !got.fromBranch || got.value == product.Top() {
		t.Fatalf("reassignment was not recorded independently: %#v", got)
	}
}

func TestTransferPresentAssignmentStateInvalidationClonesOnceAndPreservesSharedInput(t *testing.T) {
	stats := &presentAssignmentTransferStats{}
	l := &lowerer{presentAssignmentStats: stats}
	xChild := path.NewPath(symbol.ID(1), "x").Field("child")
	y := path.NewPath(symbol.ID(2), "y")
	xKey, _ := presentAssignmentKeyForPath(xChild)
	yKey, _ := presentAssignmentKeyForPath(y)
	base := presentAssignmentState{
		xKey: {path: xChild},
		yKey: {path: y},
	}
	input := &factflow.FactsInput{
		PathAssignments: map[cfg.Point]factflow.PathAssignment{
			1: factflow.NewPathAssignment(xChild.RootOnly(), factflow.ValueSource{Kind: factflow.ValueSourceUnknown}),
		},
	}

	changed := l.transferPresentAssignmentState(input, cfg.Point(1), base, false)
	if stats.stateClones != 1 {
		t.Fatalf("invalidation clones = %d, want exactly 1", stats.stateClones)
	}
	if _, ok := changed[xKey]; ok {
		t.Fatal("overlapping descendant survived invalidation")
	}
	if _, ok := changed[yKey]; !ok {
		t.Fatal("non-overlapping sibling was invalidated")
	}
	if _, ok := base[xKey]; !ok {
		t.Fatal("shared incoming state was alias-mutated")
	}
}

func BenchmarkTransferPresentAssignmentStateNoOp(b *testing.B) {
	stats := &presentAssignmentTransferStats{}
	l := &lowerer{presentAssignmentStats: stats}
	state := make(presentAssignmentState, 64)
	for i := 1; i <= 64; i++ {
		p := path.NewPath(symbol.ID(i), "value")
		key, _ := presentAssignmentKeyForPath(p)
		state[key] = conditionalAssignment{path: p}
	}
	input := &factflow.FactsInput{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := l.transferPresentAssignmentState(input, cfg.Point(1), state, false); len(got) != len(state) {
			b.Fatal("no-op transfer changed state")
		}
	}
	b.StopTimer()
	if stats.stateClones != 0 {
		b.Fatalf("no-op transfers cloned %d states", stats.stateClones)
	}
}
