package symboliccall

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBoundaryRootsAreNamespaceDistinctAndCapturesBindAtCall(t *testing.T) {
	reg := standard.Registry()
	param := testValue(runtimekind.Number, 0)
	capture := testValue(runtimekind.String, 0)
	vararg := testValue(runtimekind.Table, 1)
	tr := NewBoundaryTransformer(reg, 1, 1, []BoundaryRow{{
		VarargLength: ExactVarargLength(1),
		Returns:      []Expr{Param(0), Capture(0), Vararg(0)},
	}}, nil, BoundaryPolicy{})
	rows, err := tr.Instantiate([]product.Value{param}, []product.Value{capture}, []product.Value{vararg})
	if err != nil || len(rows) != 1 {
		t.Fatalf("instantiate: rows=%v err=%v", rows, err)
	}
	want := []product.Value{param, capture, vararg}
	for i := range want {
		if !product.Equal(reg, rows[0][i], want[i]) {
			t.Fatalf("slot %d aliased another namespace", i)
		}
	}
	if _, err := tr.Instantiate([]product.Value{param}, nil, []product.Value{vararg}); err == nil {
		t.Fatal("missing closure environment was silently reconstructed")
	}
}

func TestVarargPackPreservesLengthAbsentAndExplicitNil(t *testing.T) {
	reg := standard.Registry()
	nilValue := typevalue.Nil(reg)
	tr := NewBoundaryTransformer(reg, 0, 0, []BoundaryRow{
		{VarargLength: ExactVarargLength(0), Returns: []Expr{Vararg(0)}},
		{VarargLength: ExactVarargLength(1), Returns: []Expr{Vararg(0)}},
	}, nil, BoundaryPolicy{})
	empty, err := tr.Instantiate(nil, nil, nil)
	if err != nil || len(empty) != 1 || !product.Equal(reg, empty[0][0], product.Absent(reg)) {
		t.Fatalf("empty pack = %#v err=%v", empty, err)
	}
	explicitNil, err := tr.Instantiate(nil, nil, []product.Value{nilValue})
	if err != nil || len(explicitNil) != 1 || !product.Equal(reg, explicitNil[0][0], nilValue) {
		t.Fatalf("explicit nil pack = %#v err=%v", explicitNil, err)
	}
	if product.Equal(reg, empty[0][0], explicitNil[0][0]) {
		t.Fatal("absent vararg position collapsed into explicit nil")
	}
}

func TestBoundaryRequirementsAreContravariant(t *testing.T) {
	reg := standard.Registry()
	stringValue := testValue(runtimekind.String, 0)
	numberValue := testValue(runtimekind.Number, 0)
	either := product.Join(reg, stringValue, numberValue)
	root := Root{Kind: RootCapture, Index: 0}
	a := NewBoundaryTransformer(reg, 0, 1, nil, []BoundaryRequirement{{Root: root, Allowed: either}}, BoundaryPolicy{})
	b := NewBoundaryTransformer(reg, 0, 1, nil, []BoundaryRequirement{{Root: root, Allowed: stringValue}}, BoundaryPolicy{})
	joined := JoinBoundary(a, b)
	if joined.ContextualReason() != "" || len(joined.requirements) != 1 || !product.Equal(reg, joined.requirements[0].Allowed, stringValue) {
		t.Fatalf("requirements did not meet: %#v", joined)
	}
	if _, err := joined.Instantiate(nil, []product.Value{numberValue}, nil); err == nil {
		t.Fatal("caller violating capture requirement was accepted")
	}
	if _, err := joined.Instantiate(nil, []product.Value{stringValue}, nil); err != nil {
		t.Fatalf("valid capture rejected: %v", err)
	}
}

func TestBoundaryPolicyFailsClosedBeforeComposition(t *testing.T) {
	reg := standard.Registry()
	tests := []struct {
		policy BoundaryPolicy
		want   string
	}{
		{BoundaryPolicy{HeapMutatedCaptures: true}, "heap-mutated capture"},
		{BoundaryPolicy{Allocation: true}, "allocation identity"},
		{BoundaryPolicy{ActorState: true}, "actor state"},
		{BoundaryPolicy{HeapMutatedCaptures: true, ActorState: true}, "actor state"},
	}
	for _, tt := range tests {
		got := NewBoundaryTransformer(reg, 0, 0, nil, nil, tt.policy)
		if got.ContextualReason() != tt.want {
			t.Fatalf("policy %#v reason=%q want=%q", tt.policy, got.ContextualReason(), tt.want)
		}
	}
}

func TestBoundaryCanonicalJoinOrderAndLaws(t *testing.T) {
	reg := standard.Registry()
	a := NewBoundaryTransformer(reg, 1, 1, []BoundaryRow{{VarargLength: ExactVarargLength(0), Returns: []Expr{Capture(0)}}}, nil, BoundaryPolicy{})
	b := NewBoundaryTransformer(reg, 1, 1, []BoundaryRow{{Guards: []RootGuard{{Root: Root{Kind: RootParam, Index: 0}, Kind: GuardTruthy}}, VarargLength: ExactVarargLength(1), Returns: []Expr{Vararg(0)}}}, nil, BoundaryPolicy{})
	c := NewBoundaryTransformer(reg, 1, 1, []BoundaryRow{{VarargLength: VarargLength{Min: 2, Max: -1}, Returns: []Expr{Param(0)}}}, nil, BoundaryPolicy{})
	if !EqualBoundary(JoinBoundary(a, a), a) || !EqualBoundary(JoinBoundary(a, b), JoinBoundary(b, a)) {
		t.Fatal("boundary join idempotence/commutativity failed")
	}
	if !EqualBoundary(JoinBoundary(JoinBoundary(a, b), c), JoinBoundary(a, JoinBoundary(c, b))) {
		t.Fatal("boundary join associativity failed")
	}
	if !LessOrEqBoundary(a, JoinBoundary(a, b)) || !LessOrEqBoundary(b, JoinBoundary(a, b)) {
		t.Fatal("boundary join is not an upper bound")
	}
}

func TestRandomBoundaryDifferentialAgainstSequentialEvaluation(t *testing.T) {
	reg := standard.Registry()
	rng := rand.New(rand.NewSource(0xc4a7e))
	values := []product.Value{product.Absent(reg), typevalue.Nil(reg), testValue(runtimekind.String, 0), testValue(runtimekind.Number, 0), testValue(runtimekind.Table, 1)}
	for trial := 0; trial < 2000; trial++ {
		length := rng.Intn(5)
		params := []product.Value{values[rng.Intn(len(values))]}
		captures := []product.Value{values[rng.Intn(len(values))]}
		varargs := make([]product.Value, length)
		for i := range varargs {
			varargs[i] = values[rng.Intn(len(values))]
		}
		rows := make([]BoundaryRow, 5)
		for n := range rows {
			rows[n] = BoundaryRow{VarargLength: ExactVarargLength(n), Returns: []Expr{Capture(0), Vararg(0), Vararg(3), Param(0)}}
		}
		tr := NewBoundaryTransformer(reg, 1, 1, rows, nil, BoundaryPolicy{})
		got, err := tr.Instantiate(params, captures, varargs)
		if err != nil || len(got) != 1 {
			t.Fatalf("trial %d: rows=%d err=%v", trial, len(got), err)
		}
		at := func(i int) product.Value {
			if i >= len(varargs) {
				return product.Absent(reg)
			}
			return varargs[i]
		}
		want := []product.Value{captures[0], at(0), at(3), params[0]}
		for i := range want {
			if !product.Equal(reg, got[0][i], want[i]) {
				t.Fatalf("trial %d slot %d mismatch", trial, i)
			}
		}
	}
}
