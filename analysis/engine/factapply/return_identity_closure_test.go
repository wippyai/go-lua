package factapply

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

func returnClosureTerm(index uint64) identity.Term {
	return identity.ConcreteTerm(identity.ID{Kind: "test.return-closure", Site: "law", Index: index})
}

func returnBoolAlgebra() ReturnBooleanAlgebra[bool] {
	return ReturnBooleanAlgebra[bool]{
		False: false,
		And:   func(left, right bool) (bool, error) { return left && right, nil },
		Or:    func(left, right bool) (bool, error) { return left || right, nil },
		Not:   func(value bool) (bool, error) { return !value, nil },
		Equal: func(left, right bool) bool { return left == right },
	}
}

func returnMaskAlgebra(mask uint8) ReturnBooleanAlgebra[uint8] {
	return ReturnBooleanAlgebra[uint8]{
		False: 0,
		And:   func(left, right uint8) (uint8, error) { return left & right & mask, nil },
		Or:    func(left, right uint8) (uint8, error) { return (left | right) & mask, nil },
		Not:   func(value uint8) (uint8, error) { return (^value) & mask, nil },
		Equal: func(left, right uint8) bool { return left == right },
	}
}

func TestCloseReturnIdentitiesChain(t *testing.T) {
	a, b, c := returnClosureTerm(1), returnClosureTerm(2), returnClosureTerm(3)
	got, err := CloseReturnIdentities(context.Background(), returnBoolAlgebra(),
		[]ReturnIdentityCondition[bool]{{Root: a, Condition: true}},
		[]ReturnIdentityCondition[bool]{{Root: a, Condition: true}},
		[]ReturnIdentityEdgeCondition[bool]{
			{From: a, To: b, Condition: true},
			{From: b, To: c, Condition: true},
		},
	)
	want := []ReturnIdentityCondition[bool]{
		{Root: a, Condition: true},
		{Root: b, Condition: true},
		{Root: c, Condition: true},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("CloseReturnIdentities() = %#v, %v; want %#v", got, err, want)
	}
}

func TestCloseReturnIdentitiesCycleReachesLeastFixedPoint(t *testing.T) {
	a, b, c := returnClosureTerm(1), returnClosureTerm(2), returnClosureTerm(3)
	got, err := CloseReturnIdentities(context.Background(), returnBoolAlgebra(),
		[]ReturnIdentityCondition[bool]{{Root: a, Condition: true}},
		[]ReturnIdentityCondition[bool]{{Root: a, Condition: true}},
		[]ReturnIdentityEdgeCondition[bool]{
			{From: a, To: b, Condition: true},
			{From: b, To: c, Condition: true},
			{From: c, To: a, Condition: true},
		},
	)
	if err != nil || len(got) != 3 {
		t.Fatalf("cyclic closure = %#v, %v; want three reachable roots", got, err)
	}
}

func TestCloseReturnIdentitiesPreservesGuardAlgebra(t *testing.T) {
	// Four bits are the complete truth table of two guard atoms. Duplicate
	// edges contribute x OR y; the final edge selects NOT x.
	a, b, c := returnClosureTerm(1), returnClosureTerm(2), returnClosureTerm(3)
	const all, x, y uint8 = 0b1111, 0b1100, 0b1010
	got, err := CloseReturnIdentities(context.Background(), returnMaskAlgebra(all),
		[]ReturnIdentityCondition[uint8]{{Root: a, Condition: all}},
		[]ReturnIdentityCondition[uint8]{{Root: a, Condition: all}},
		[]ReturnIdentityEdgeCondition[uint8]{
			{From: a, To: b, Condition: x},
			{From: a, To: b, Condition: y},
			{From: b, To: c, Condition: (^x) & all},
		},
	)
	want := []ReturnIdentityCondition[uint8]{
		{Root: a, Condition: all},
		{Root: b, Condition: x | y},
		{Root: c, Condition: y &^ x},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("guarded closure = %#v, %v; want %#v", got, err, want)
	}
}

func TestCloseReturnIdentitiesDeterministicUnderInputOrder(t *testing.T) {
	a, b, c := returnClosureTerm(1), returnClosureTerm(2), returnClosureTerm(3)
	sources := []ReturnIdentityCondition[uint8]{{Root: a, Condition: 0b0101}, {Root: a, Condition: 0b1010}}
	admissions := []ReturnIdentityCondition[uint8]{{Root: a, Condition: 0b1100}, {Root: a, Condition: 0b0011}}
	edges := []ReturnIdentityEdgeCondition[uint8]{
		{From: b, To: c, Condition: 0b1111},
		{From: a, To: b, Condition: 0b0011},
		{From: a, To: b, Condition: 0b1100},
	}
	forward, err := CloseReturnIdentities(context.Background(), returnMaskAlgebra(0b1111), sources, admissions, edges)
	if err != nil {
		t.Fatal(err)
	}
	reverseConditions := func(input []ReturnIdentityCondition[uint8]) []ReturnIdentityCondition[uint8] {
		out := append([]ReturnIdentityCondition[uint8](nil), input...)
		for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
			out[left], out[right] = out[right], out[left]
		}
		return out
	}
	reversedEdges := append([]ReturnIdentityEdgeCondition[uint8](nil), edges...)
	for left, right := 0, len(reversedEdges)-1; left < right; left, right = left+1, right-1 {
		reversedEdges[left], reversedEdges[right] = reversedEdges[right], reversedEdges[left]
	}
	reversed, err := CloseReturnIdentities(context.Background(), returnMaskAlgebra(0b1111), reverseConditions(sources), reverseConditions(admissions), reversedEdges)
	if err != nil || !reflect.DeepEqual(forward, reversed) {
		t.Fatalf("permuted closure = %#v, %v; want %#v", reversed, err, forward)
	}
}

func TestCloseReturnIdentitiesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := CloseReturnIdentities(ctx, returnBoolAlgebra(), nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseReturnIdentities() error = %v, want context.Canceled", err)
	}
}

func TestCloseReturnIdentitiesCancellationDuringSealWithEmptyQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	algebra := returnBoolAlgebra()
	algebra.Or = func(left, right bool) (bool, error) {
		cancel()
		return left || right, nil
	}
	root := returnClosureTerm(1)
	_, err := CloseReturnIdentities(ctx, algebra, nil,
		[]ReturnIdentityCondition[bool]{
			{Root: root, Condition: true},
			{Root: root, Condition: false},
		}, nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseReturnIdentities() error = %v, want context.Canceled", err)
	}
}

func TestCloseReturnIdentitiesCancellationAfterFinalWorkItem(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	algebra := returnBoolAlgebra()
	andCalls := 0
	algebra.And = func(left, right bool) (bool, error) {
		andCalls++
		if andCalls == 2 {
			cancel()
		}
		return left && right, nil
	}
	root := returnClosureTerm(1)
	_, err := CloseReturnIdentities(ctx, algebra,
		[]ReturnIdentityCondition[bool]{{Root: root, Condition: true}},
		[]ReturnIdentityCondition[bool]{{Root: root, Condition: true}},
		[]ReturnIdentityEdgeCondition[bool]{{From: root, To: root, Condition: true}},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CloseReturnIdentities() error = %v, want context.Canceled", err)
	}
}

func TestCloseReturnIdentitiesConditionalCycleRequeuesFreshGuards(t *testing.T) {
	a, b := returnClosureTerm(1), returnClosureTerm(2)
	const all, x, y uint8 = 0b1111, 0b1100, 0b1010
	got, err := CloseReturnIdentities(context.Background(), returnMaskAlgebra(all),
		[]ReturnIdentityCondition[uint8]{
			{Root: a, Condition: x},
			{Root: b, Condition: y},
		},
		[]ReturnIdentityCondition[uint8]{
			{Root: a, Condition: all},
			{Root: b, Condition: all},
		},
		[]ReturnIdentityEdgeCondition[uint8]{
			{From: a, To: b, Condition: all},
			{From: b, To: a, Condition: all},
		},
	)
	want := []ReturnIdentityCondition[uint8]{
		{Root: a, Condition: x | y},
		{Root: b, Condition: x | y},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("conditional cyclic closure = %#v, %v; want %#v", got, err, want)
	}
}
