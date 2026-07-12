package symboliccall

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEffectsReadStableModuleRoot(t *testing.T) {
	reg := standard.Registry()
	root := GlobalRoot{Module: "kickside.automation", Name: "STATUS"}
	value := testValue(runtimekind.String, 0)
	tr := NewEffectTransformer(reg, 0, 0, []EffectRow{{
		Boundary: BoundaryRow{VarargLength: ExactVarargLength(0), Returns: []Expr{Global(root.Module, root.Name)}},
	}}, nil, EffectPolicy{})
	results, err := tr.InstantiateEffects("call", "closure", nil, nil, nil, map[GlobalRoot]product.Value{root: value}, nil)
	if err != nil || len(results) != 1 || !product.Equal(reg, results[0].Values[0], value) {
		t.Fatalf("stable global result=%#v err=%v", results, err)
	}
	if _, err := tr.InstantiateEffects("call", "closure", nil, nil, nil, nil, nil); err == nil {
		t.Fatal("unbound module root was guessed from ambient state")
	}
}

func TestAllocationIdentityRebasesPerCallAndUpdatesFreshSitesStrongly(t *testing.T) {
	reg := standard.Registry()
	initial := testValue(runtimekind.String, 0)
	updated := testValue(runtimekind.Number, 0)
	site := AllocationSite("table@12")
	location := SymbolicLocation{Kind: LocationAllocation, Site: site}
	tr := NewEffectTransformer(reg, 0, 0, []EffectRow{{
		Boundary:    BoundaryRow{VarargLength: ExactVarargLength(0)},
		Allocations: []AllocationSpec{{Site: site, Initial: Const(initial)}},
		Writes:      []EffectWrite{{Target: location, Value: Const(updated)}},
		ReturnRefs:  []SymbolicLocation{location},
	}}, nil, EffectPolicy{})

	first, err := tr.InstantiateEffects("caller-A#1", "closure", nil, nil, nil, nil, nil)
	if err != nil || len(first) != 1 {
		t.Fatalf("first instantiate: %#v err=%v", first, err)
	}
	second, err := tr.InstantiateEffects("caller-B#1", "closure", nil, nil, nil, nil, nil)
	if err != nil || len(second) != 1 {
		t.Fatalf("second instantiate: %#v err=%v", second, err)
	}
	a, b := first[0].References[0], second[0].References[0]
	if a == b || a.Allocation.Call != "caller-A#1" || b.Allocation.Call != "caller-B#1" {
		t.Fatalf("allocation identities conflated: %#v %#v", a, b)
	}
	if !product.Equal(reg, first[0].Heap[a], updated) {
		t.Fatal("fresh non-escaped allocation did not receive a strong update")
	}
}

func TestCaptureAndGlobalWritesWeakJoin(t *testing.T) {
	reg := standard.Registry()
	before := testValue(runtimekind.String, 0)
	after := testValue(runtimekind.Number, 0)
	global := GlobalRoot{Module: "module", Name: "state"}
	captureTarget := SymbolicLocation{Kind: LocationCapture, Capture: 0}
	globalTarget := SymbolicLocation{Kind: LocationGlobal, Global: global}
	tr := NewEffectTransformer(reg, 0, 1, []EffectRow{{
		Boundary: BoundaryRow{VarargLength: ExactVarargLength(0)},
		Writes: []EffectWrite{
			{Target: captureTarget, Value: Const(after)},
			{Target: globalTarget, Value: Const(after)},
		},
	}}, nil, EffectPolicy{})
	results, err := tr.InstantiateEffects("call", "closure-A", nil, []product.Value{before}, nil, map[GlobalRoot]product.Value{global: before}, nil)
	if err != nil || len(results) != 1 {
		t.Fatalf("instantiate: %#v err=%v", results, err)
	}
	want := product.Join(reg, before, after)
	captureLocation := ConcreteLocation{Kind: LocationCapture, Closure: "closure-A", Capture: 0}
	globalLocation := ConcreteLocation{Kind: LocationGlobal, Global: global}
	if !product.Equal(reg, results[0].Heap[captureLocation], want) || !product.Equal(reg, results[0].Heap[globalLocation], want) {
		t.Fatal("shared locations were overwritten rather than weak-joined")
	}
}

func TestEffectsFailClosedForEscapingAndActorOwnedState(t *testing.T) {
	reg := standard.Registry()
	tests := []struct {
		policy EffectPolicy
		row    EffectRow
		want   string
	}{
		{policy: EffectPolicy{MutableAmbientGlobals: true}, want: "mutable ambient global"},
		{policy: EffectPolicy{MailboxOrActorState: true}, want: "mailbox or actor state"},
		{policy: EffectPolicy{CrossActorHeap: true}, want: "cross-actor heap"},
		{row: EffectRow{Boundary: BoundaryRow{VarargLength: ExactVarargLength(0)}, Allocations: []AllocationSpec{{Site: "escape", Escapes: true}}}, want: "escaping heap"},
	}
	for _, tt := range tests {
		rows := []EffectRow{tt.row}
		if tt.row.Boundary.VarargLength == (VarargLength{}) && len(tt.row.Allocations) == 0 {
			rows = nil
		}
		got := NewEffectTransformer(reg, 0, 0, rows, nil, tt.policy)
		if got.ContextualReason() != tt.want {
			t.Fatalf("policy=%#v reason=%q want=%q", tt.policy, got.ContextualReason(), tt.want)
		}
	}
}

func TestEffectTransformerLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	row := func(site AllocationSite) EffectTransformer {
		return NewEffectTransformer(reg, 0, 0, []EffectRow{{
			Boundary:    BoundaryRow{VarargLength: ExactVarargLength(0)},
			Allocations: []AllocationSpec{{Site: site, Initial: Const(product.Absent(reg))}},
		}}, nil, EffectPolicy{})
	}
	a, b := row("a"), row("b")
	c := JoinEffects(a, b)
	top := EffectTransformer{reg: reg, valid: true, contextual: "top"}
	domain := lattice.Lattice[EffectTransformer]{
		Bottom:   func() EffectTransformer { return EffectTransformer{} },
		Top:      func() EffectTransformer { return top },
		Equal:    EqualEffects,
		LessOrEq: LessOrEqEffects,
		Join:     JoinEffects,
		Widen:    func(prev, next EffectTransformer) EffectTransformer { return WidenEffects(prev, next, 4) },
	}
	latticelaws.LawSuite[EffectTransformer]{
		Name:          "poc.symboliccall.effects",
		Domain:        domain,
		Sample:        []EffectTransformer{{}, top, a, b, c},
		WideningBound: 8,
	}.Run(t)
}

func TestRecursiveEffectsBoundThroughProductionWTO(t *testing.T) {
	reg := standard.Registry()
	equation := EffectEquation{ID: "recursive"}
	equation.Transfer = func(read func(FunctionID) EffectTransformer) EffectTransformer {
		prev := read("recursive")
		if prev.contextual != "" {
			return prev
		}
		rows := cloneEffectRows(prev.rows)
		rows = append(rows, EffectRow{
			Boundary:    BoundaryRow{VarargLength: ExactVarargLength(0)},
			Allocations: []AllocationSpec{{Site: AllocationSite(fmt.Sprintf("site-%d", len(rows))), Initial: Const(product.Absent(reg))}},
		})
		return NewEffectTransformer(reg, 0, 0, rows, nil, EffectPolicy{})
	}
	var stats solve.Stats
	got, err := SolveEffects(context.Background(), reg, []EffectEquation{equation}, func(id FunctionID) []FunctionID {
		return []FunctionID{id}
	}, 4, &stats)
	if err != nil {
		t.Fatal(err)
	}
	result := got["recursive"]
	if result.ContextualReason() != "effect row budget" || !result.Widened() {
		t.Fatalf("recursive effect result=%#v", result)
	}
	if stats.TransferCalls == 0 || stats.TransferCalls > 30 {
		t.Fatalf("recursive transfer calls=%d", stats.TransferCalls)
	}
}

func TestRandomEffectsDifferentialAgainstSequentialUpdates(t *testing.T) {
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(0xe771ec7))
	values := []product.Value{product.Absent(reg), testValue(runtimekind.String, 0), testValue(runtimekind.Number, 0), testValue(runtimekind.Table, 1)}
	for trial := 0; trial < 1000; trial++ {
		before := values[rng.Intn(len(values))]
		write := values[rng.Intn(len(values))]
		initial := values[rng.Intn(len(values))]
		site := AllocationSite("site")
		tr := NewEffectTransformer(reg, 0, 1, []EffectRow{{
			Boundary:    BoundaryRow{VarargLength: ExactVarargLength(0)},
			Allocations: []AllocationSpec{{Site: site, Initial: Const(initial)}},
			Writes: []EffectWrite{
				{Target: SymbolicLocation{Kind: LocationCapture, Capture: 0}, Value: Const(write)},
				{Target: SymbolicLocation{Kind: LocationAllocation, Site: site}, Value: Const(write)},
			},
		}}, nil, EffectPolicy{})
		got, err := tr.InstantiateEffects(fmt.Sprintf("call-%d", trial), "closure", nil, []product.Value{before}, nil, nil, nil)
		if err != nil || len(got) != 1 {
			t.Fatalf("trial %d result=%#v err=%v", trial, got, err)
		}
		capture := ConcreteLocation{Kind: LocationCapture, Closure: "closure", Capture: 0}
		allocation := concreteAllocation(fmt.Sprintf("call-%d", trial), site)
		if !product.Equal(reg, got[0].Heap[capture], product.Join(reg, before, write)) {
			t.Fatalf("trial %d weak update mismatch", trial)
		}
		if !product.Equal(reg, got[0].Heap[allocation], write) {
			t.Fatalf("trial %d strong update retained initial value", trial)
		}
	}
}
