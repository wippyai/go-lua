package symboliccall

import (
	"context"
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGuardedRowsPreserveCorrelatedReturns(t *testing.T) {
	reg := standard.Registry()
	ok := testValue(runtimekind.Boolean, 0)
	number := testValue(runtimekind.Number, 0)
	errValue := testValue(runtimekind.String, 0)
	text := testValue(runtimekind.String, 0)
	transformer := NewGuardedTransformer(reg, 1, []GuardedRow{
		{Guards: []Guard{{Param: 0, Kind: GuardTruthy}}, Returns: []Expr{Const(ok), Const(number)}},
		{Guards: []Guard{{Param: 0, Kind: GuardFalsy}}, Returns: []Expr{Const(errValue), Const(text)}},
	}, nil)

	truthyRows, err := transformer.InstantiateRows([]product.Value{testValue(runtimekind.Table, 1)})
	if err != nil || len(truthyRows) != 1 || !product.Equal(reg, truthyRows[0][1], number) {
		t.Fatalf("truthy rows = %#v, err=%v", truthyRows, err)
	}
	falsyRows, err := transformer.InstantiateRows([]product.Value{product.Absent(reg)})
	if err != nil || len(falsyRows) != 1 || !product.Equal(reg, falsyRows[0][1], text) {
		t.Fatalf("falsy rows = %#v, err=%v", falsyRows, err)
	}
	unknownRows, err := transformer.InstantiateRows([]product.Value{product.Top()})
	if err != nil || len(unknownRows) != 2 {
		t.Fatalf("unknown rows = %#v, err=%v", unknownRows, err)
	}
	// Correlation is represented by the two rows. There is no independently
	// combined (ok,string) or (err,number) row.
	for _, row := range unknownRows {
		if product.Equal(reg, row[0], ok) && !product.Equal(reg, row[1], number) {
			t.Fatal("fabricated ok/non-number cross-pair")
		}
	}
}

func TestContradictoryGuardsDropRowsAndMissingSlotsBecomeAbsent(t *testing.T) {
	reg := standard.Registry()
	value := testValue(runtimekind.String, 0)
	transformer := NewGuardedTransformer(reg, 1, []GuardedRow{
		{Guards: []Guard{{Param: 0, Kind: GuardTruthy}, {Param: 0, Kind: GuardFalsy}}, Returns: []Expr{Const(value)}},
		{Returns: []Expr{Const(value)}},
		{Returns: []Expr{Const(value), Const(value)}},
	}, nil)
	rows, err := transformer.InstantiateRows([]product.Value{product.Top()})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %#v, err=%v", rows, err)
	}
	if len(rows[0]) != 2 || !product.Equal(reg, rows[0][1], product.Absent(reg)) {
		t.Fatalf("short row missing slot was not Lua absent: %#v", rows[0])
	}
}

func TestGuardWeakeningIsUpperBoundAndRequirementOverflowFallsBack(t *testing.T) {
	reg := standard.Registry()
	value := testValue(runtimekind.String, 0)
	base := NewGuardedTransformer(reg, 2, []GuardedRow{{
		Guards:  []Guard{{Param: 0, Kind: GuardTruthy}, {Param: 1, Kind: GuardTruthy}},
		Returns: []Expr{Const(value)},
	}}, nil)
	widened := WidenGuarded(GuardedTransformer{}, base, GuardedLimits{MaxGuardsPerRow: 1})
	if !widened.Widened() || !LessOrEqGuarded(base, widened) {
		t.Fatalf("guard weakening is not an upper bound: %#v", widened)
	}
	rows, err := widened.InstantiateRows([]product.Value{product.Absent(reg), product.Absent(reg)})
	if err != nil || len(rows) != 1 {
		t.Fatalf("weakened guard did not retain behavior: rows=%v err=%v", rows, err)
	}
	requirement := NewGuardedTransformer(reg, 2, nil, []GuardedRequirement{{
		Guards: []Guard{{Param: 0, Kind: GuardTruthy}, {Param: 1, Kind: GuardTruthy}}, Param: 0, Value: value,
	}})
	got := WidenGuarded(GuardedTransformer{}, requirement, GuardedLimits{MaxGuardsPerRow: 1})
	if got.ContextualReason() != "requirement guard budget" {
		t.Fatalf("requirement overflow = %q, want atomic fallback", got.ContextualReason())
	}
}

func TestRequirementsJoinContravariantlyByMeet(t *testing.T) {
	reg := standard.Registry()
	stringValue := testValue(runtimekind.String, 0)
	numberValue := testValue(runtimekind.Number, 0)
	stringOrNumber := product.Join(reg, stringValue, numberValue)
	a := NewGuardedTransformer(reg, 1, nil, []GuardedRequirement{{Param: 0, Value: stringOrNumber}})
	b := NewGuardedTransformer(reg, 1, nil, []GuardedRequirement{{Param: 0, Value: stringValue}})
	joined := JoinGuarded(a, b)
	if joined.contextual != "" || len(joined.requirements) != 1 || !product.Equal(reg, joined.requirements[0].Value, stringValue) {
		t.Fatalf("requirement join did not use product meet: %#v", joined)
	}
	contradiction := JoinGuarded(b, NewGuardedTransformer(reg, 1, nil, []GuardedRequirement{{Param: 0, Value: numberValue}}))
	if contradiction.ContextualReason() != "contradictory requirement" {
		t.Fatalf("bottom requirement did not fallback: %q", contradiction.ContextualReason())
	}
	missing := NewGuardedTransformer(reg, 1, nil, nil)
	explicitTop := NewGuardedTransformer(reg, 1, nil, []GuardedRequirement{{Param: 0, Value: product.Top()}})
	if !EqualGuarded(missing, explicitTop) {
		t.Fatal("missing requirement is not canonical Top/no-constraint")
	}
}

func TestGuardedJoinLawsAndOrderDeterminism(t *testing.T) {
	reg := standard.Registry()
	a := NewGuardedTransformer(reg, 1, []GuardedRow{{Guards: []Guard{{Param: 0, Kind: GuardTruthy}}, Returns: []Expr{Param(0)}}}, nil)
	b := NewGuardedTransformer(reg, 1, []GuardedRow{{Guards: []Guard{{Param: 0, Kind: GuardFalsy}}, Returns: []Expr{Const(product.Absent(reg))}}}, nil)
	c := NewGuardedTransformer(reg, 1, []GuardedRow{{Returns: []Expr{Const(testValue(runtimekind.String, 0))}}}, nil)
	if !EqualGuarded(JoinGuarded(a, a), a) || !EqualGuarded(JoinGuarded(a, b), JoinGuarded(b, a)) {
		t.Fatal("guarded join idempotence/commutativity failed")
	}
	left := JoinGuarded(JoinGuarded(a, b), c)
	right := JoinGuarded(a, JoinGuarded(c, b))
	if !EqualGuarded(left, right) {
		t.Fatal("guarded join associativity/order determinism failed")
	}
}

func TestGuardedTransformerLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	a := NewGuardedTransformer(reg, 1, []GuardedRow{{Guards: []Guard{{Param: 0, Kind: GuardTruthy}}, Returns: []Expr{Param(0)}}}, nil)
	b := NewGuardedTransformer(reg, 1, []GuardedRow{{Guards: []Guard{{Param: 0, Kind: GuardFalsy}}, Returns: []Expr{Const(product.Absent(reg))}}}, nil)
	c := JoinGuarded(a, b)
	top := GuardedTransformer{reg: reg, params: 1, valid: true, contextual: "top"}
	domain := lattice.Lattice[GuardedTransformer]{
		Bottom:   func() GuardedTransformer { return GuardedTransformer{} },
		Top:      func() GuardedTransformer { return top },
		Equal:    EqualGuarded,
		LessOrEq: LessOrEqGuarded,
		Join:     JoinGuarded,
		Widen: func(prev, next GuardedTransformer) GuardedTransformer {
			return WidenGuarded(prev, next, GuardedLimits{MaxRows: 4, MaxGuardsPerRow: 2})
		},
	}
	latticelaws.LawSuite[GuardedTransformer]{
		Name:          "poc.symboliccall.guarded",
		Domain:        domain,
		Sample:        []GuardedTransformer{{}, top, a, b, c},
		WideningBound: 8,
	}.Run(t)
}

func TestRecursiveGuardGrowthWidensAndConvergesThroughWTO(t *testing.T) {
	reg := standard.Registry()
	equation := GuardedEquation{ID: "loop"}
	equation.Transfer = func(read func(FunctionID) GuardedTransformer) GuardedTransformer {
		prev := read("loop")
		rows := cloneRows(prev.rows)
		if !prev.valid {
			rows = nil
		}
		index := len(rows) % 8
		rows = append(rows, GuardedRow{Guards: []Guard{{Param: index, Kind: GuardTruthy}}, Returns: []Expr{Param(index)}})
		return NewGuardedTransformer(reg, 8, rows, nil)
	}
	var stats solve.Stats
	got, err := SolveGuarded(context.Background(), reg, []GuardedEquation{equation}, func(id FunctionID) []FunctionID {
		if id == "loop" {
			return []FunctionID{"loop"}
		}
		return nil
	}, GuardedLimits{MaxRows: 4, MaxGuardsPerRow: 2}, &stats)
	if err != nil {
		t.Fatal(err)
	}
	result := got["loop"]
	if !result.Widened() || result.RowCount() > 4 || result.ContextualReason() != "" {
		t.Fatalf("recursive result = %#v", result)
	}
	if stats.TransferCalls == 0 || stats.TransferCalls > 30 {
		t.Fatalf("recursive transfer count = %d", stats.TransferCalls)
	}
	stationary := WidenGuarded(result, result, GuardedLimits{MaxRows: 4, MaxGuardsPerRow: 2})
	if !EqualGuarded(stationary, result) {
		t.Fatal("widened recursive result is not stationary")
	}
}

func TestRandomGuardedInstantiationAndWidenDifferential(t *testing.T) {
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(0x6a17))
	values := []product.Value{product.Absent(reg), testValue(runtimekind.String, 0), testValue(runtimekind.Table, 1), product.Top()}
	for trial := 0; trial < 1000; trial++ {
		var rows []GuardedRow
		for i := 0; i < 1+rng.Intn(8); i++ {
			guardCount := rng.Intn(4)
			row := GuardedRow{Returns: []Expr{Const(values[rng.Intn(len(values))])}}
			for j := 0; j < guardCount; j++ {
				row.Guards = append(row.Guards, Guard{Param: rng.Intn(3), Kind: GuardKind(1 + rng.Intn(2))})
			}
			rows = append(rows, row)
		}
		base := NewGuardedTransformer(reg, 3, rows, nil)
		widened := WidenGuarded(GuardedTransformer{}, base, GuardedLimits{MaxRows: 3, MaxGuardsPerRow: 2})
		if !LessOrEqGuarded(base, widened) {
			t.Fatalf("trial %d: widening is not an upper bound", trial)
		}
		params := []product.Value{values[rng.Intn(len(values))], values[rng.Intn(len(values))], values[rng.Intn(len(values))]}
		baseValue := instantiateSlotOrBottom(t, base, params)
		wideValue := instantiateSlotOrBottom(t, widened, params)
		if !product.LessOrEq(reg, baseValue, wideValue) {
			t.Fatalf("trial %d: concrete widened result is not an upper bound", trial)
		}
	}
}

func instantiateSlotOrBottom(t *testing.T, transformer GuardedTransformer, params []product.Value) product.Value {
	t.Helper()
	values, err := transformer.Instantiate(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) == 0 {
		return product.Bottom(transformer.reg)
	}
	return values[0]
}
