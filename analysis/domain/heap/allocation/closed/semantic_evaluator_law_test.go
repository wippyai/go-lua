package closed_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	closed "github.com/wippyai/go-lua/analysis/domain/heap/allocation/closed"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
)

func TestClosedEvaluatorReceiptSourceOrderNilDeletionAndCarry(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local child = {}; return { item = child, item = nil }`)
	child, childOK := firstTableAtom(t, heap, values)
	nilAtom, nilOK := values.OpaqueKind(runtimekind.Nil)
	childValue, childValueOK := values.Singleton(child)
	nilValue, nilValueOK := values.Singleton(nilAtom)
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !childOK || !nilOK || !childValueOK || !nilValueOK || !predecessorOK {
		t.Fatal("closed evaluator source-order fixture")
	}
	inputs := []valuedomain.Value{values.Top(), values.Top()}
	for index := 0; index < operand.Count(); index++ {
		field, fieldOK := operand.At(index)
		if !fieldOK {
			t.Fatal("closed evaluator field")
		}
		if index == 0 {
			inputs[field.ValueOrdinal()] = childValue
		} else {
			inputs[field.ValueOrdinal()] = nilValue
		}
	}
	first, normal, resultOK := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
	if !resultOK || !normal {
		t.Fatalf("closed source-order result=%t/%t", resultOK, normal)
	}
	assertClosedWorldKind(t, first, heapdomain.WorldOne)
	field, fieldOK := operand.At(1)
	selector, selectorOK := field.ExactSelector()
	seenAbsent := false
	if !fieldOK || !selectorOK || !heap.VisitRawAccess(operand.Key(), first, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		cell, cellOK := access.Cell()
		raw, rawOK := cell.Raw()
		if rawOK && raw == heapdomain.RawAbsent {
			seenAbsent = true
		}
		return cellOK && rawOK
	}) || !seenAbsent {
		t.Fatal("closed source-order nil did not delete the exact field")
	}
	second, secondNormal, secondOK := closed.EvaluateClosedForTest(heap, values, operand, first, inputs)
	if !secondOK || !secondNormal {
		t.Fatalf("closed carry result=%t/%t", secondOK, secondNormal)
	}
	assertClosedWorldKind(t, second, heapdomain.WorldMany)
}

func TestClosedEvaluatorReceiptDiagonalAndIndependentProducts(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		coordinates int
		worlds      int
	}{
		{name: "diagonal", source: `local a = {}; local x = a; return { [x] = x }`, coordinates: 1, worlds: 2},
		{name: "independent", source: `local a = {}; local b = {}; return { [a] = b }`, coordinates: 2, worlds: 4},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			heap, values, operand := closedSemanticFixture(t, testCase.source)
			if operand.CoordinateCount() != testCase.coordinates {
				t.Fatalf("coordinates=%d, want %d", operand.CoordinateCount(), testCase.coordinates)
			}
			left, right := tableAtoms(t, heap, values, 2)
			alternatives, alternativesOK := values.Alternatives(left, right)
			predecessor, predecessorOK := heap.EmptyObject(operand.Key())
			if !alternativesOK || !predecessorOK {
				t.Fatal("closed product fixture")
			}
			inputs := make([]valuedomain.Value, operand.CoordinateCount())
			for index := range inputs {
				inputs[index] = alternatives
			}
			result, normal, resultOK := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
			if !resultOK || !normal {
				t.Fatalf("product result=%t/%t", resultOK, normal)
			}
			if result.WorldCount() != testCase.worlds {
				t.Fatalf("product worlds=%d, want %d", result.WorldCount(), testCase.worlds)
			}
			atoms := []valuedomain.Atom{left, right}
			var expected heapdomain.Value
			haveExpected := false
			joinLeaf := func(inputs []valuedomain.Value) {
				leaf, leafNormal, leafOK := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
				if !leafOK || !leafNormal {
					t.Fatalf("ordinary product leaf=%t/%t", leafOK, leafNormal)
				}
				if !haveExpected {
					expected, haveExpected = leaf, true
					return
				}
				var joinedOK bool
				expected, joinedOK = heapdomain.Join(expected, leaf)
				if !joinedOK {
					t.Fatal("ordinary product join")
				}
			}
			if testCase.coordinates == 1 {
				for _, atom := range atoms {
					singleton, singletonOK := values.Singleton(atom)
					if !singletonOK {
						t.Fatal("diagonal singleton")
					}
					joinLeaf([]valuedomain.Value{singleton})
				}
			} else {
				for _, keyAtom := range atoms {
					for _, valueAtom := range atoms {
						keyValue, keyValueOK := values.Singleton(keyAtom)
						payloadValue, payloadValueOK := values.Singleton(valueAtom)
						if !keyValueOK || !payloadValueOK {
							t.Fatal("independent singleton")
						}
						joinLeaf([]valuedomain.Value{keyValue, payloadValue})
					}
				}
			}
			if !haveExpected || !heap.Domain().Equal(result, expected) {
				t.Fatal("binary-carry product differs from ordinary leaf join")
			}
		})
	}
}

func TestClosedEvaluatorReceiptInvalidAndOpaqueKeys(t *testing.T) {
	heap, values, invalid := closedSemanticFixture(t, `local x = nil; return { [x] = x }`)
	nilAtom, nilOK := values.OpaqueKind(runtimekind.Nil)
	nilValue, nilValueOK := values.Singleton(nilAtom)
	predecessor, predecessorOK := heap.EmptyObject(invalid.Key())
	if !nilOK || !nilValueOK || !predecessorOK {
		t.Fatal("invalid-key evaluator fixture")
	}
	if _, normal, resultOK := closed.EvaluateClosedForTest(heap, values, invalid, predecessor, []valuedomain.Value{nilValue}); !resultOK || normal {
		t.Fatalf("invalid key result=%t normal=%t, want valid no-candidate", resultOK, normal)
	}

	heap, values, opaqueOperand := closedSemanticFixture(t, `local key = {}; return { [key] = key }`)
	opaque, opaqueOK := values.OpaqueReference(valuedomain.ReferenceTable)
	opaqueValue, opaqueValueOK := values.Singleton(opaque)
	predecessor, predecessorOK = heap.EmptyObject(opaqueOperand.Key())
	if !opaqueOK || !opaqueValueOK || !predecessorOK {
		t.Fatal("opaque-key evaluator fixture")
	}
	result, normal, resultOK := closed.EvaluateClosedForTest(heap, values, opaqueOperand, predecessor, []valuedomain.Value{opaqueValue})
	if !resultOK || !normal {
		t.Fatalf("opaque key result=%t/%t", resultOK, normal)
	}
	selector, selectorOK := heap.KindSelector()
	seenPresent := false
	if !selectorOK || !heap.VisitRawAccess(opaqueOperand.Key(), result, materialization.Recent, selector, func(access heapdomain.RawAccess) bool {
		cell, cellOK := access.Cell()
		raw, rawOK := cell.Raw()
		if rawOK && raw == heapdomain.RawPresent {
			seenPresent = true
		}
		return cellOK && rawOK
	}) || !seenPresent {
		t.Fatal("opaque key/value containment was not retained")
	}
	foreignHeap, foreignValues, foreignOperand := closedSemanticFixture(t, `local x = nil; return { [x] = x }`)
	foreignPredecessor, foreignPredecessorOK := foreignHeap.EmptyObject(foreignOperand.Key())
	foreignNil, foreignNilOK := foreignValues.OpaqueKind(runtimekind.Nil)
	foreignNilValue, foreignNilValueOK := foreignValues.Singleton(foreignNil)
	if !foreignPredecessorOK || !foreignNilOK || !foreignNilValueOK {
		t.Fatal("foreign closed evaluator fixture")
	}
	if _, _, accepted := closed.EvaluateClosedForTest(heap, values, foreignOperand, foreignPredecessor, []valuedomain.Value{foreignNilValue}); accepted {
		t.Fatal("closed evaluator accepted a foreign operand/schema pair")
	}
}

func assertClosedWorldKind(t testing.TB, value heapdomain.Value, want heapdomain.WorldKind) {
	t.Helper()
	world, worldOK := value.WorldAt(0)
	if !worldOK || world.Kind() != want {
		t.Fatalf("closed world=%v/%t, want %v/true", world.Kind(), worldOK, want)
	}
}
