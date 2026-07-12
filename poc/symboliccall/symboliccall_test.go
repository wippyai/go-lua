package symboliccall

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func testValue(tag runtimekind.Tag, id uint64) product.Value {
	reg := standard.Registry()
	v := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	v = product.Set(reg, v, runtimekind.Key, runtimekind.Singleton(tag))
	if id != 0 {
		v = product.Set(reg, v, identity.Key, identity.Singleton(identity.ID{Kind: "poc", Site: "test", Index: id}))
	}
	return v
}

func TestDirectCompositionUsesProductionProduct(t *testing.T) {
	reg := standard.Registry()
	stringValue := testValue(runtimekind.String, 0)
	tableValue := testValue(runtimekind.Table, 7)
	defs := []Definition{
		{ID: "identity", Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{Param(0)}},
		{ID: "wrapper", Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{Join(Call("identity", 0, Param(0)), Const(stringValue))}},
	}
	got, err := Compile(context.Background(), defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := got["wrapper"].Instantiate(reg, []product.Value{tableValue})
	if err != nil {
		t.Fatal(err)
	}
	want := product.Join(reg, tableValue, stringValue)
	if len(results) != 1 || !product.Equal(reg, results[0], want) {
		t.Fatalf("composed result does not match production join")
	}
}

func TestRandomAcyclicCompositionMatchesSequentialCalls(t *testing.T) {
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(0x51cc))
	constants := []product.Value{
		testValue(runtimekind.String, 0),
		testValue(runtimekind.Number, 0),
		testValue(runtimekind.Table, 1),
	}
	for trial := 0; trial < 100; trial++ {
		defs := make([]Definition, 40)
		byID := make(map[FunctionID]Definition, len(defs))
		for i := range defs {
			id := FunctionID(fmt.Sprintf("f%02d", i))
			expr := Param(0)
			if i > 0 {
				callee := FunctionID(fmt.Sprintf("f%02d", rng.Intn(i)))
				expr = Call(callee, 0, expr)
			}
			if rng.Intn(3) != 0 {
				expr = Join(expr, Const(constants[rng.Intn(len(constants))]))
			}
			defs[i] = Definition{ID: id, Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{expr}}
			byID[id] = defs[i]
		}
		compiled, err := Compile(context.Background(), defs, nil)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		arg := constants[rng.Intn(len(constants))]
		for _, def := range defs {
			got, err := compiled[def.ID].Instantiate(reg, []product.Value{arg})
			if err != nil {
				t.Fatalf("trial %d %s: %v", trial, def.ID, err)
			}
			want, err := evalSequential(reg, def.Returns[0], []product.Value{arg}, byID)
			if err != nil {
				t.Fatal(err)
			}
			if !product.Equal(reg, got[0], want) {
				t.Fatalf("trial %d %s: composed and sequential results differ", trial, def.ID)
			}
		}
	}
}

func TestMutualRecursionConvergesThroughWTO(t *testing.T) {
	reg := standard.Registry()
	constant := testValue(runtimekind.String, 0)
	defs := []Definition{
		{ID: "f", Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{Join(Param(0), Call("g", 0, Param(0)))}},
		{ID: "g", Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{Join(Const(constant), Call("f", 0, Param(0)))}},
	}
	var stats Stats
	compiled, err := Compile(context.Background(), defs, &stats)
	if err != nil {
		t.Fatal(err)
	}
	arg := testValue(runtimekind.Table, 9)
	want := product.Join(reg, arg, constant)
	for _, id := range []FunctionID{"f", "g"} {
		got, err := compiled[id].Instantiate(reg, []product.Value{arg})
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !product.Equal(reg, got[0], want) {
			t.Fatalf("%s did not reach the least recursive solution", id)
		}
	}
	if stats.Solver.TransferCalls == 0 || stats.Solver.TransferCalls > 20 {
		t.Fatalf("unexpected WTO transfer count %d", stats.Solver.TransferCalls)
	}
	second, err := Compile(context.Background(), []Definition{defs[1], defs[0]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []FunctionID{"f", "g"} {
		if !transformerEqual(compiled[id], second[id]) {
			t.Fatalf("%s depends on definition order", id)
		}
	}
}

func TestUnsupportedLaneFallsBackAtomically(t *testing.T) {
	defs := []Definition{
		{ID: "actor", Params: 1, Uses: []state.LaneID{state.LaneValues, state.LaneChannelSelect}, Returns: []Expr{Param(0)}},
		{ID: "caller", Params: 1, Uses: []state.LaneID{state.LaneValues}, Returns: []Expr{Call("actor", 0, Param(0))}},
	}
	compiled, err := Compile(context.Background(), defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []FunctionID{"actor", "caller"} {
		if !compiled[id].Contextual() {
			t.Fatalf("%s published a partial symbolic result", id)
		}
	}
	if !strings.Contains(compiled["actor"].Reason(), "channel-select") {
		t.Fatalf("fallback reason = %q", compiled["actor"].Reason())
	}
	if _, err := compiled["caller"].Instantiate(standard.Registry(), []product.Value{testValue(runtimekind.String, 0)}); err == nil {
		t.Fatal("contextual transformer instantiated")
	}
}

func TestCapabilityCensusCoversSeventeenProductionStateLanes(t *testing.T) {
	lanes := state.DefaultLanes()
	if len(lanes) != 17 {
		t.Fatalf("default state lane count = %d, want 17", len(lanes))
	}
	seen := map[state.LaneID]bool{}
	for _, lane := range lanes {
		if seen[lane] {
			t.Fatalf("duplicate lane %q", lane)
		}
		seen[lane] = true
	}
	if !seen[state.LaneValues] || !seen[state.LaneTypestates] || !seen[state.LaneChannelSelect] {
		t.Fatalf("capability census omitted core actor lanes: %v", lanes)
	}
}

func evalSequential(reg *axis.Registry, expr Expr, params []product.Value, defs map[FunctionID]Definition) (product.Value, error) {
	if expr.n == nil {
		return product.Bottom(reg), nil
	}
	switch expr.n.op {
	case opParam:
		if expr.n.param >= len(params) {
			return product.Value{}, fmt.Errorf("parameter out of range")
		}
		return params[expr.n.param], nil
	case opConst:
		return expr.n.value, nil
	case opJoin:
		out := product.Bottom(reg)
		for _, arg := range expr.n.args {
			value, err := evalSequential(reg, arg, params, defs)
			if err != nil {
				return product.Value{}, err
			}
			out = product.Join(reg, out, value)
		}
		return out, nil
	case opCall:
		def, ok := defs[expr.n.callee]
		if !ok || expr.n.slot >= len(def.Returns) {
			return product.Value{}, fmt.Errorf("unknown call %q", expr.n.callee)
		}
		args := make([]product.Value, len(expr.n.args))
		for i, arg := range expr.n.args {
			var err error
			args[i], err = evalSequential(reg, arg, params, defs)
			if err != nil {
				return product.Value{}, err
			}
		}
		return evalSequential(reg, def.Returns[expr.n.slot], args, defs)
	default:
		return product.Bottom(reg), nil
	}
}
