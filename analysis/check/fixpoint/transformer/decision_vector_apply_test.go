package transformer

import (
	"context"
	"testing"
)

func decisionTestLeafAt(t *testing.T, kernel *decisionKernel, root decisionRef, valuation map[uint32]bool) decisionLeaf {
	t.Helper()
	for {
		if int(root) >= len(kernel.nodes) {
			t.Fatalf("decision root %d escaped arena", root)
		}
		node := kernel.nodes[root]
		if node.terminal {
			return node.leaf
		}
		if valuation[node.variable] {
			root = node.high
		} else {
			root = node.low
		}
	}
}

func TestDecisionVectorApplyMatchesPointwiseCareLaws(t *testing.T) {
	for _, sample := range []struct {
		name string
		op   formalComponentBinaryOp
	}{
		{name: "join", op: formalComponentJoin},
		{name: "meet", op: formalComponentMeet},
		{name: "widen", op: formalComponentWiden},
		{name: "narrow", op: formalComponentNarrow},
	} {
		t.Run(sample.name, func(t *testing.T) {
			kernel := newDecisionKernel()
			kernel.resetBoolean()
			leftCare := kernel.branch(0, decisionFalse, decisionTrue)
			rightCare := kernel.branch(1, decisionFalse, decisionTrue)
			resultCare := leftCare
			var err error
			switch sample.op {
			case formalComponentJoin, formalComponentWiden:
				resultCare, err = kernel.apply(context.Background(), uint8(decisionOr), true, leftCare, rightCare, decisionLeafOr)
			case formalComponentMeet:
				resultCare, err = kernel.apply(context.Background(), uint8(decisionAnd), true, leftCare, rightCare, decisionLeafAnd)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := kernel.applyVectorUnderCare(
				context.Background(), resultCare, leftCare, rightCare,
				[]decisionRef{kernel.terminal(2), kernel.terminal(4)},
				[]decisionRef{kernel.terminal(3), kernel.terminal(5)},
				func(left, right []decisionLeaf) ([]decisionLeaf, error) {
					switch {
					case len(left) != 0 && len(right) != 0:
						// Deliberately ordered: this pins widen/narrow operand order
						// as well as complete two-member carrier correlation.
						return []decisionLeaf{left[0]*10 + right[0], left[1]*10 + right[1]}, nil
					case len(left) != 0:
						return append([]decisionLeaf(nil), left...), nil
					case len(right) != 0:
						return append([]decisionLeaf(nil), right...), nil
					default:
						return nil, errDecisionMalformed
					}
				},
			)
			if err != nil || len(result) != 2 {
				t.Fatalf("vector Apply = %v, %v", result, err)
			}
			for bits := 0; bits < 4; bits++ {
				leftLive, rightLive := bits&1 != 0, bits&2 != 0
				valuation := map[uint32]bool{0: leftLive, 1: rightLive}
				wantLive := false
				switch sample.op {
				case formalComponentJoin, formalComponentWiden:
					wantLive = leftLive || rightLive
				case formalComponentMeet:
					wantLive = leftLive && rightLive
				case formalComponentNarrow:
					wantLive = leftLive
				}
				if gotLive := decisionTestLeafAt(t, &kernel, resultCare, valuation) == 1; gotLive != wantLive {
					t.Fatalf("valuation %02b Care = %t, want %t", bits, gotLive, wantLive)
				}
				if !wantLive {
					continue
				}
				want := []decisionLeaf{2, 4}
				switch {
				case leftLive && rightLive:
					want = []decisionLeaf{23, 45}
				case rightLive:
					want = []decisionLeaf{3, 5}
				}
				for index := range result {
					if got := decisionTestLeafAt(t, &kernel, result[index], valuation); got != want[index] {
						t.Fatalf("valuation %02b member %d = %d, want %d", bits, index, got, want[index])
					}
				}
			}
		})
	}
}

func TestDecisionVectorApplyDoesNotCrossIndependentCarriers(t *testing.T) {
	kernel := newDecisionKernel()
	kernel.resetBoolean()
	calls := 0
	for variable := uint32(0); variable < 12; variable++ {
		root := kernel.branch(variable, kernel.terminal(2), kernel.terminal(3))
		_, err := kernel.applyVectorUnderCare(context.Background(), decisionTrue, decisionTrue, decisionTrue,
			[]decisionRef{root}, []decisionRef{kernel.terminal(4)},
			func(left, right []decisionLeaf) ([]decisionLeaf, error) {
				calls++
				return []decisionLeaf{left[0] + right[0]}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Each independent one-variable carrier has exactly two reachable local
	// leaf pairs. A global tuple partition would have 2^12 rows.
	if calls != 24 {
		t.Fatalf("terminal calls = %d, want additive 24", calls)
	}
}
