package flatinterproc

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFlatDifferentialRepresentativeFixtures(t *testing.T) {
	reg := standard.Registry()
	stringValue := typevalue.LiteralString(reg, "s")
	numberValue := typevalue.LiteralInt(reg, 1)
	fixtures := []struct {
		name    string
		program program
	}{
		{"direct-chain", chainProgram(reg, stringValue)},
		{"recursive-base", recursiveProgram(reg, stringValue)},
		{"dynamic-contexts", contextualProgram(reg, stringValue, numberValue)},
		{"external-fallback", externalProgram(reg, stringValue)},
		{"loop-widen-narrow", loopProgram(reg, numberValue, stringValue)},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			nested, err := solveNested(context.Background(), fixture.program)
			if err != nil {
				t.Fatal(err)
			}
			engine := &flatEngine{program: fixture.program}
			flat, err := engine.solve(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			assertSnapshotsEqual(t, fixture.program, nested, flat)
			if !reflect.DeepEqual(diagnostics(reg, nested), diagnostics(reg, flat)) {
				t.Fatalf("nested diagnostics=%v flat=%v", diagnostics(reg, nested), diagnostics(reg, flat))
			}
		})
	}
}

func TestFlatEliminatesNestedWholeBodyRestartMultiplier(t *testing.T) {
	reg := standard.Registry()
	// Put callers before callees in SummaryKey order. The nested reference is
	// then forced to carry a newly grown summary through one complete body solve
	// per dependency layer. The flat WTO derives its order from dependency edges,
	// rather than inheriting this incidental key order.
	p := reverseOrderedDeepChainProgram(reg, typevalue.LiteralString(reg, "s"), 24)
	discovery, err := (&flatEngine{program: p}).solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nested, err := solveNestedFrom(context.Background(), p, discovery.contexts)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := solveFlatPreseeded(context.Background(), p, discovery.contexts)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotsEqual(t, p, nested, flat)
	t.Logf("chain transfer calls: nested=%d flat=%d", nested.transfers, flat.transfers)
	if flat.transfers*4 >= nested.transfers {
		t.Fatalf("flat transfers=%d nested=%d, want more than 4x structural reduction", flat.transfers, nested.transfers)
	}
}

func TestFlatFavorableDependencyOrderIsIrreducibleCellVisit(t *testing.T) {
	reg := standard.Registry()
	p := deepChainProgram(reg, typevalue.LiteralString(reg, "s"), 24)
	discovery, err := (&flatEngine{program: p}).solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	flat, err := solveFlatPreseeded(context.Background(), p, discovery.contexts)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	for key := range discovery.contexts {
		want += len(p.functions[key.Ref].nodes) + 1 // flow points + summary boundary
	}
	if flat.transfers != want {
		t.Fatalf("flat transfers=%d cells=%d, want one irreducible transfer per equation cell", flat.transfers, want)
	}
}

func TestKnownInternalMustBeBottomFirstForMonotonicity(t *testing.T) {
	reg := standard.Registry()
	known := typevalue.LiteralString(reg, "known")
	bottom := product.Bottom(reg)
	top := product.Top()
	if !product.LessOrEq(reg, bottom, known) {
		t.Fatal("known internal Bottom cannot grow to first summary")
	}
	if product.LessOrEq(reg, top, known) {
		t.Fatal("missing-as-Top unexpectedly grows monotonically to known")
	}
	if got := product.Join(reg, top, known); !product.Equal(reg, got, top) {
		t.Fatal("test did not reproduce stale unknown precision")
	}
}

func TestFlatCancellationPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	p := chainProgram(reg, typevalue.LiteralString(reg, "s"))
	engine := &flatEngine{program: p}
	first, err := engine.solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := engine.solve(ctx)
	if !errors.Is(err, solve.ErrCanceled) {
		t.Fatalf("cancel error=%v", err)
	}
	assertSnapshotsEqual(t, p, first, got)
}

func TestFlatEntryGenerationJoinsProgressivelyReachableCallers(t *testing.T) {
	reg := standard.Registry()
	left := typevalue.LiteralString(reg, "left")
	right := typevalue.LiteralInt(reg, 2)
	p, target := progressiveEntryProgram(reg, left, right)

	nested, err := solveNested(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	flat, err := (&flatEngine{program: p}).solve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshotsEqual(t, p, nested, flat)

	key := summary.DefaultSummaryKey(target)
	want := product.Join(reg, left, right)
	if got := valueOf(reg, flat.contexts[key]); !product.Equal(reg, got, want) {
		t.Fatalf("target entry=%#v, want joined progressively reachable callers %#v", got, want)
	}
}

func assertSnapshotsEqual(t *testing.T, p program, left, right snapshot) {
	t.Helper()
	if len(left.contexts) != len(right.contexts) || len(left.summaries) != len(right.summaries) {
		t.Fatalf("context/summary shape nested=%d/%d flat=%d/%d", len(left.contexts), len(left.summaries), len(right.contexts), len(right.summaries))
	}
	domain := summary.NormalizedDomain(p.reg)
	states := state.Domain(p.reg)
	for key, want := range left.summaries {
		if got, ok := right.summaries[key]; !ok || !domain.Equal(want, got) {
			t.Fatalf("summary %v nested=%#v flat=%#v", key, want, got)
		}
	}
	for key, want := range left.contexts {
		if got, ok := right.contexts[key]; !ok || !states.Equal(want, got) {
			t.Fatalf("entry %v differs", key)
		}
	}
}

func chainProgram(reg *axis.Registry, value product.Value) program {
	leaf, wrapper, root := ref.FromSymbol(1), ref.FromSymbol(2), ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			leaf:    {ref: leaf, entry: 0, nodes: []node{{op: opReturn}}},
			wrapper: {ref: wrapper, entry: 0, nodes: []node{{op: opCall, callee: leaf, next: []int{1}}, {op: opReturn}}},
			root:    {ref: root, entry: 0, nodes: []node{{op: opCall, callee: wrapper, next: []int{1}}, {op: opReturn}}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, value)},
	}
}

func deepChainProgram(reg *axis.Registry, value product.Value, depth int) program {
	functions := make(map[ref.FuncRef]function, depth+1)
	leaf := ref.FromSymbol(1)
	functions[leaf] = function{ref: leaf, entry: 0, nodes: []node{{op: opReturn}}}
	prior := leaf
	for i := 2; i <= depth; i++ {
		current := ref.FromSymbol(symbol.ID(i))
		functions[current] = function{ref: current, entry: 0, nodes: []node{{op: opCall, callee: prior, next: []int{1}}, {op: opReturn}}}
		prior = current
	}
	root := ref.Root()
	functions[root] = function{ref: root, entry: 0, nodes: []node{{op: opCall, callee: prior, next: []int{1}}, {op: opReturn}}}
	return program{reg: reg, functions: functions, roots: map[summary.SummaryKey]state.State{
		summary.DefaultSummaryKey(root): stateWithValue(reg, value),
	}}
}

func reverseOrderedDeepChainProgram(reg *axis.Registry, value product.Value, depth int) program {
	functions := make(map[ref.FuncRef]function, depth+1)
	leaf := ref.FromSymbol(symbol.ID(depth))
	functions[leaf] = function{ref: leaf, entry: 0, nodes: []node{{op: opReturn}}}
	prior := leaf
	for i := depth - 1; i >= 1; i-- {
		current := ref.FromSymbol(symbol.ID(i))
		functions[current] = function{ref: current, entry: 0, nodes: []node{{op: opCall, callee: prior, next: []int{1}}, {op: opReturn}}}
		prior = current
	}
	root := ref.Root()
	functions[root] = function{ref: root, entry: 0, nodes: []node{{op: opCall, callee: prior, next: []int{1}}, {op: opReturn}}}
	return program{reg: reg, functions: functions, roots: map[summary.SummaryKey]state.State{
		summary.DefaultSummaryKey(root): stateWithValue(reg, value),
	}}
}

func recursiveProgram(reg *axis.Registry, base product.Value) program {
	rec, root := ref.FromSymbol(10), ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			rec: {ref: rec, entry: 0, nodes: []node{
				{op: opFork, next: []int{1, 3}},
				{op: opConst, value: base, next: []int{2}},
				{op: opReturn},
				{op: opCall, callee: rec, next: []int{4}},
				{op: opReturn},
			}},
			root: {ref: root, entry: 0, nodes: []node{{op: opCall, callee: rec, next: []int{1}}, {op: opReturn}}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, base)},
	}
}

func contextualProgram(reg *axis.Registry, left, right product.Value) program {
	id, root := ref.FromSymbol(20), ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			id: {ref: id, entry: 0, nodes: []node{{op: opReturn}}},
			root: {ref: root, entry: 0, nodes: []node{
				{op: opFork, next: []int{1, 3}},
				{op: opConst, value: left, next: []int{2}},
				{op: opCall, callee: id, contextual: true, next: []int{5}},
				{op: opConst, value: right, next: []int{4}},
				{op: opCall, callee: id, contextual: true, next: []int{5}},
				{op: opReturn},
			}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, left)},
	}
}

func externalProgram(reg *axis.Registry, value product.Value) program {
	root := ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			root: {ref: root, entry: 0, nodes: []node{{op: opCall, external: true, next: []int{1}}, {op: opReturn}}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, value)},
	}
}

func loopProgram(reg *axis.Registry, entry, loopValue product.Value) program {
	root := ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			root: {ref: root, entry: 0, nodes: []node{
				{op: opFork, next: []int{1, 2}, loopHead: true},
				{op: opConst, value: loopValue, next: []int{0}},
				{op: opReturn},
			}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, entry)},
	}
}

func progressiveEntryProgram(reg *axis.Registry, left, right product.Value) (program, ref.FuncRef) {
	target, root := ref.FromSymbol(30), ref.Root()
	return program{
		reg: reg,
		functions: map[ref.FuncRef]function{
			target: {ref: target, entry: 0, nodes: []node{{op: opReturn}}},
			root: {ref: root, entry: 0, nodes: []node{
				{op: opConst, value: left, next: []int{1}},
				// The second call is unreachable until this first target summary
				// becomes non-Bottom in a later immutable-entry generation.
				{op: opCall, callee: target, next: []int{2}},
				{op: opConst, value: right, next: []int{3}},
				{op: opCall, callee: target, next: []int{4}},
				{op: opReturn},
			}},
		},
		roots: map[summary.SummaryKey]state.State{summary.DefaultSummaryKey(root): stateWithValue(reg, left)},
	}, target
}
