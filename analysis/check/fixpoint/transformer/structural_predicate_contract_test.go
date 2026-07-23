package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// This file contracts the source-equality and structural-point layer of
// compiler_structural_predicate.go: samePredicateSource, samePredicateSourceActive,
// predicateSourcePath, and containsStructuralPoint. These four are pure
// functions over factflow.Facts/ValueSource and never touch the compiler's
// Builder/Arena, so they are driven directly rather than through a
// planCompileContext.

func mustScalarShape(t *testing.T) factflow.ValueSourceShape {
	t.Helper()
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("scalar value source shape rejected")
	}
	return shape
}

func TestContainsStructuralPoint(t *testing.T) {
	tests := []struct {
		name   string
		points []cfg.Point
		target cfg.Point
		want   bool
	}{
		{"empty region", nil, cfg.Point(1), false},
		{"single point match", []cfg.Point{5}, cfg.Point(5), true},
		{"single point miss", []cfg.Point{5}, cfg.Point(6), false},
		{"first of many", []cfg.Point{2, 4, 6}, cfg.Point(2), true},
		{"middle of many", []cfg.Point{2, 4, 6}, cfg.Point(4), true},
		{"last of many", []cfg.Point{2, 4, 6}, cfg.Point(6), true},
		{"before first", []cfg.Point{2, 4, 6}, cfg.Point(1), false},
		{"between elements", []cfg.Point{2, 4, 6}, cfg.Point(3), false},
		{"after last", []cfg.Point{2, 4, 6}, cfg.Point(7), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsStructuralPoint(tt.points, tt.target); got != tt.want {
				t.Fatalf("containsStructuralPoint(%v, %d) = %v, want %v", tt.points, tt.target, got, tt.want)
			}
		})
	}
}

func TestPredicateSourcePath(t *testing.T) {
	shape := mustScalarShape(t)
	p := pathdom.NewPath(symbol.ID(7), "n")

	t.Run("path kind resolves its canonical path", func(t *testing.T) {
		source, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, shape)
		if !ok {
			t.Fatal("path source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{})
		got, ok := predicateSourcePath(facts, source)
		if !ok || !got.Equal(p) {
			t.Fatalf("predicateSourcePath = %#v/%v, want %#v/true", got, ok, p)
		}
	})

	t.Run("path kind with a malformed key fails closed", func(t *testing.T) {
		source := factflow.ValueSource{Kind: factflow.ValueSourcePath, PathKey: "not-a-resolver-path"}
		facts := factflow.NewFacts(factflow.FactsInput{})
		if _, ok := predicateSourcePath(facts, source); ok {
			t.Fatal("malformed path key resolved a path")
		}
	})

	t.Run("expression kind resolves a registered expression path", func(t *testing.T) {
		ref := factflow.ExprRef(3)
		source, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
		if !ok {
			t.Fatal("expression source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: p}})
		got, ok := predicateSourcePath(facts, source)
		if !ok || !got.Equal(p) {
			t.Fatalf("predicateSourcePath = %#v/%v, want %#v/true", got, ok, p)
		}
	})

	t.Run("expression kind without a path fact is not a path", func(t *testing.T) {
		ref := factflow.ExprRef(4)
		source, ok := factflow.NewExpressionValueSource(ref, 0, 0, 0, shape)
		if !ok {
			t.Fatal("expression source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{})
		if _, ok := predicateSourcePath(facts, source); ok {
			t.Fatal("expression with no registered path fact resolved a path")
		}
	})

	t.Run("literal kind is never a path", func(t *testing.T) {
		source, ok := factflow.NewIntegerLiteralValueSource(5, 0, 0, 0, shape)
		if !ok {
			t.Fatal("literal source rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{})
		if _, ok := predicateSourcePath(facts, source); ok {
			t.Fatal("literal source resolved a path")
		}
	})
}

func TestSamePredicateSourceLiteralAndWildcardSemantics(t *testing.T) {
	shape := mustScalarShape(t)
	facts := factflow.NewFacts(factflow.FactsInput{})

	t.Run("same literal value with different index metadata is equal", func(t *testing.T) {
		a, ok := factflow.NewIntegerLiteralValueSource(5, 0, 0, 0, shape)
		if !ok {
			t.Fatal("literal source rejected")
		}
		b, ok := factflow.NewIntegerLiteralValueSource(5, 1, 0, 0, shape)
		if !ok {
			t.Fatal("literal source rejected")
		}
		if a == b {
			t.Fatal("test setup: expected distinct struct instances")
		}
		if !samePredicateSource(facts, a, b) {
			t.Fatal("identical integer literals were not recognized as the same source")
		}
	})

	t.Run("different literal values are not equal", func(t *testing.T) {
		a, _ := factflow.NewIntegerLiteralValueSource(5, 0, 0, 0, shape)
		c, _ := factflow.NewIntegerLiteralValueSource(6, 0, 0, 0, shape)
		if samePredicateSource(facts, a, c) {
			t.Fatal("different integer literals were recognized as the same source")
		}
	})

	t.Run("nil sources are equal regardless of target index", func(t *testing.T) {
		n1 := factflow.NewNilValueSource(0)
		n2 := factflow.NewNilValueSource(1)
		if !samePredicateSource(facts, n1, n2) {
			t.Fatal("two nil sources were not recognized as the same source")
		}
	})

	t.Run("unknown sources are equal regardless of target index", func(t *testing.T) {
		u1 := factflow.NewUnknownValueSource(0)
		u2 := factflow.NewUnknownValueSource(1)
		if !samePredicateSource(facts, u1, u2) {
			t.Fatal("two unknown sources were not recognized as the same source")
		}
	})

	t.Run("nil and unknown are distinct kinds, not interchangeable", func(t *testing.T) {
		n := factflow.NewNilValueSource(0)
		u := factflow.NewUnknownValueSource(0)
		if samePredicateSource(facts, n, u) {
			t.Fatal("nil and unknown sources were treated as the same source")
		}
	})
}

func TestSamePredicateSourcePathSemantics(t *testing.T) {
	shape := mustScalarShape(t)
	facts := factflow.NewFacts(factflow.FactsInput{})
	p := pathdom.NewPath(symbol.ID(11), "v")

	t.Run("same path with different index metadata is equal", func(t *testing.T) {
		left, ok := factflow.NewPathValueSource(p.Key(), 0, 0, 0, shape)
		if !ok {
			t.Fatal("path source rejected")
		}
		right, ok := factflow.NewPathValueSource(p.Key(), 1, 1, 1, shape)
		if !ok {
			t.Fatal("path source rejected")
		}
		if left == right {
			t.Fatal("test setup: expected distinct struct instances")
		}
		if !samePredicateSource(facts, left, right) {
			t.Fatal("identical local paths were not recognized as the same source")
		}
	})

	t.Run("different paths are not equal", func(t *testing.T) {
		left, _ := factflow.NewPathValueSource(p.Key(), 0, 0, 0, shape)
		other := pathdom.NewPath(symbol.ID(12), "w")
		right, ok := factflow.NewPathValueSource(other.Key(), 0, 0, 0, shape)
		if !ok {
			t.Fatal("path source rejected")
		}
		if samePredicateSource(facts, left, right) {
			t.Fatal("different local paths were recognized as the same source")
		}
	})
}

func TestSamePredicateSourceOperationTreeSemantics(t *testing.T) {
	shape := mustScalarShape(t)
	leafOne, _ := factflow.NewIntegerLiteralValueSource(1, 0, 0, 0, shape)
	leafTwo, _ := factflow.NewIntegerLiteralValueSource(2, 0, 0, 0, shape)
	leafThree, _ := factflow.NewIntegerLiteralValueSource(3, 0, 0, 0, shape)

	t.Run("structurally identical operation trees under different ExprRefs are equal", func(t *testing.T) {
		opA, ok := factflow.NewBinaryExpressionOperation("+", leafOne, leafTwo)
		if !ok {
			t.Fatal("binary operation rejected")
		}
		opB, ok := factflow.NewBinaryExpressionOperation("+", leafOne, leafTwo)
		if !ok {
			t.Fatal("binary operation rejected")
		}
		refA, refB := factflow.ExprRef(10), factflow.ExprRef(20)
		sourceA, _ := factflow.NewExpressionValueSource(refA, 0, 0, 0, shape)
		sourceB, _ := factflow.NewExpressionValueSource(refB, 0, 0, 0, shape)
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{refA: opA, refB: opB},
		})
		if !samePredicateSource(facts, sourceA, sourceB) {
			t.Fatal("structurally identical operation trees were not recognized as the same source")
		}
	})

	t.Run("operation trees differing in one operand are not equal", func(t *testing.T) {
		opA, _ := factflow.NewBinaryExpressionOperation("+", leafOne, leafTwo)
		opC, _ := factflow.NewBinaryExpressionOperation("+", leafOne, leafThree)
		refA, refC := factflow.ExprRef(30), factflow.ExprRef(31)
		sourceA, _ := factflow.NewExpressionValueSource(refA, 0, 0, 0, shape)
		sourceC, _ := factflow.NewExpressionValueSource(refC, 0, 0, 0, shape)
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{refA: opA, refC: opC},
		})
		if samePredicateSource(facts, sourceA, sourceC) {
			t.Fatal("operation trees with different operands were recognized as the same source")
		}
	})

	t.Run("mutually cyclic operation facts fail closed instead of recursing forever", func(t *testing.T) {
		refX, refY := factflow.ExprRef(40), factflow.ExprRef(41)
		sourceX, _ := factflow.NewExpressionValueSource(refX, 0, 0, 0, shape)
		sourceY, _ := factflow.NewExpressionValueSource(refY, 0, 0, 0, shape)
		opX, ok := factflow.NewUnaryExpressionOperation("-", sourceY)
		if !ok {
			t.Fatal("unary operation rejected")
		}
		opY, ok := factflow.NewUnaryExpressionOperation("-", sourceX)
		if !ok {
			t.Fatal("unary operation rejected")
		}
		facts := factflow.NewFacts(factflow.FactsInput{
			ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{refX: opX, refY: opY},
		})
		// This must terminate (not hang) and, since the cycle can never certify
		// either side against a base case, must resolve to false.
		if samePredicateSource(facts, sourceX, sourceY) {
			t.Fatal("mutually cyclic operation facts were recognized as the same source")
		}
	})
}

func TestExactStructuralBranchConditionRecognizesOnlyNormalizedNot(t *testing.T) {
	shape := mustScalarShape(t)
	path := pathdom.NewPath(symbol.ID(47), "enabled")
	operand, ok := factflow.NewPathValueSource(path.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("operand source rejected")
	}
	not, ok := factflow.NewUnaryExpressionOperation("not", operand)
	if !ok {
		t.Fatal("not operation rejected")
	}
	left, ok := factflow.NewExpressionValueSource(1, 0, 0, 0, shape)
	if !ok {
		t.Fatal("not expression source rejected")
	}
	facts := factflow.NewFacts(factflow.FactsInput{ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{1: not}})

	falsy, ok := factflow.NewBranchCondition(operand, false)
	if !ok {
		t.Fatal("falsy branch condition rejected")
	}
	if !exactStructuralBranchCondition(facts, left, falsy) {
		t.Fatal("normalized falsy operand did not certify exact not expression")
	}

	truthy, ok := factflow.NewBranchCondition(operand, true)
	if !ok {
		t.Fatal("truthy branch condition rejected")
	}
	if exactStructuralBranchCondition(facts, left, truthy) {
		t.Fatal("truthy operand incorrectly certified as a not expression")
	}

	otherPath := pathdom.NewPath(symbol.ID(48), "other")
	other, _ := factflow.NewPathValueSource(otherPath.Key(), 0, 0, 0, shape)
	wrong, _ := factflow.NewBranchCondition(other, false)
	if exactStructuralBranchCondition(facts, left, wrong) {
		t.Fatal("different falsy operand incorrectly certified as exact")
	}
}

// TestSamePredicateSourceRecognizesRepeatedDynamicIndexRead mirrors
// TestExternalCensusStructuralBranchConditionNotExactLeftOperand
// (analysis/check/fixpoint/program/external_predicate_census_test.go):
// `local values: {[string]: string} = {}; local k = "a"; return values[k] or
// ""`. The branch's condition and the "or" operation's own left operand are
// two independently lowered ExprRefs for the identical values[k] read: a
// dynamic index has neither a static ExpressionPath nor an ExpressionOperation
// fact, so neither the path branch nor the operation-tree branch of
// samePredicateSourceActive ever fires for it. The contract is that two
// dynamic-index reads of the same table and key denote the same source;
// samePredicateSourceActive currently has no case at all for
// DynamicIndexExpression and falls through to false.
func TestSamePredicateSourceRecognizesRepeatedDynamicIndexRead(t *testing.T) {
	shape := mustScalarShape(t)
	tablePath := pathdom.NewPath(symbol.ID(50), "values")
	key, ok := factflow.NewStringLiteralValueSource("k", 0, 0, 0, shape)
	if !ok {
		t.Fatal("literal key source rejected")
	}
	dyn, ok := factflow.NewDynamicIndexExpression(tablePath, key)
	if !ok {
		t.Fatal("dynamic index expression rejected")
	}
	refA, refB := factflow.ExprRef(51), factflow.ExprRef(52)
	sourceA, _ := factflow.NewExpressionValueSource(refA, 0, 0, 0, shape)
	sourceB, _ := factflow.NewExpressionValueSource(refB, 0, 0, 0, shape)
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{refA: dyn, refB: dyn},
	})
	if !samePredicateSource(facts, sourceA, sourceB) {
		t.Fatal("two dynamic-index reads of the identical table[key] were not recognized as the same predicate source")
	}
}

// TestSamePredicateSourceRecognizesRepeatedCastDynamicIndexRead mirrors
// TestExternalCensusValueSourceOperand: `local meta = (entry :: any).meta or
// {}` inside an ipairs loop. The dynamic index's table producer is itself an
// any-cast (an ExpressionRefinement), not a plain lexical path, so
// predicateSourcePath cannot resolve either side; the contract is again that
// two dynamic reads through the identical cast and key denote the same
// source.
func TestSamePredicateSourceRecognizesRepeatedCastDynamicIndexRead(t *testing.T) {
	shape := mustScalarShape(t)
	castRef := factflow.ExprRef(60)
	castSource, _ := factflow.NewExpressionValueSource(castRef, 0, 0, 0, shape)
	key, ok := factflow.NewStringLiteralValueSource("meta", 0, 0, 0, shape)
	if !ok {
		t.Fatal("literal key source rejected")
	}
	dyn, ok := factflow.NewDynamicIndexExpressionFromSource(castSource, key)
	if !ok {
		t.Fatal("dynamic index expression rejected")
	}
	refA, refB := factflow.ExprRef(61), factflow.ExprRef(62)
	sourceA, _ := factflow.NewExpressionValueSource(refA, 0, 0, 0, shape)
	sourceB, _ := factflow.NewExpressionValueSource(refB, 0, 0, 0, shape)
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{refA: dyn, refB: dyn},
	})
	if !samePredicateSource(facts, sourceA, sourceB) {
		t.Fatal("two dynamic reads through the identical any-cast were not recognized as the same predicate source")
	}
}
