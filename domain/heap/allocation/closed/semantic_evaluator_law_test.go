package closed_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	closed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	"github.com/wippyai/go-lua/domain/heap/allocation/internal/source"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestClosedEvaluatorReceiptSourceOrderNilDeletionAndCarry(t *testing.T) {
	heap, values, operand := closedSemanticFixture(t, `local child = {}; return { item = child, item = nil }`)
	child, childOK := firstTableAtom(t, heap, values)
	childValue, childValueOK := values.Singleton(child)
	// atomNil is Value's sole nil alternative, so the nil operand is the
	// sealed runtime-kind projection rather than an opaque-kind atom.
	nilValue, nilValueOK := values.ForRuntimeKinds(runtimekind.Bit(runtimekind.Nil))
	predecessor, predecessorOK := heap.EmptyObject(operand.Key())
	if !childOK || !childValueOK || !nilValueOK || !predecessorOK {
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
	first, outcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
	if outcome != structure.Concrete {
		t.Fatalf("closed source-order outcome=%v, want concrete", outcome)
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
	second, secondOutcome := closed.EvaluateClosedForTest(heap, values, operand, first, inputs)
	if secondOutcome != structure.Concrete {
		t.Fatalf("closed carry outcome=%v, want concrete", secondOutcome)
	}
	assertClosedWorldKind(t, second, heapdomain.WorldMany)
}

// TestClosedEvaluatorReceiptDiagonalAndIndependentProducts is the closed
// evaluator's complete product law.
//
// L1 keeps a shared coordinate enumerated: `{[x]=x}` over two alternatives has
// exactly two worlds and equals the ordinary join of its two singleton leaves.
// L2 declares the payload fold: a coordinate read once, in the stored-value
// role only, correlates no second cell, so its result is exactly the pointwise
// object merge of its singleton leaves — expressed as the public Widen fold,
// not as a world count. L3 is the non-collapse guard: a shared coordinate must
// stay strictly below that same merge, so no later lane can fold everything.
// L4 keeps binary-carry accumulation equal to the ordinary left-fold join on a
// still-enumerated coordinate, including one whose partner payload folds.
func TestClosedEvaluatorReceiptDiagonalAndIndependentProducts(t *testing.T) {
	t.Run("diagonal", func(t *testing.T) {
		heap, values, operand := closedSemanticFixture(t, `local a = {}; local x = a; return { [x] = x }`)
		if operand.CoordinateCount() != 1 {
			t.Fatalf("coordinates=%d, want 1", operand.CoordinateCount())
		}
		left, right := tableAtoms(t, heap, values, 2)
		alternatives, alternativesOK := values.Alternatives(left, right)
		predecessor, predecessorOK := heap.EmptyObject(operand.Key())
		if !alternativesOK || !predecessorOK {
			t.Fatal("diagonal product fixture")
		}
		result, outcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, []valuedomain.Value{alternatives})
		if outcome != structure.Concrete {
			t.Fatalf("diagonal outcome=%v, want concrete", outcome)
		}
		if result.WorldCount() != 2 {
			t.Fatalf("diagonal worlds=%d, want 2", result.WorldCount())
		}
		leaves := closedSingletonLeaves(t, heap, values, operand, predecessor, [][]valuedomain.Atom{{left}, {right}})

		// L1/L4: the shared coordinate's product is the ordinary leaf join.
		joined, joinedOK := closedJoinLeaves(leaves)
		if !joinedOK || !heap.Domain().Equal(result, joined) {
			t.Fatal("binary-carry product differs from ordinary leaf join")
		}

		// L3: it must remain strictly below the pointwise merge, which admits
		// the off-diagonal pairs the shared coordinate excludes.
		merged, mergedOK := closedWidenLeaves(leaves)
		if !mergedOK || !heapdomain.LessOrEq(result, merged) {
			t.Fatal("shared coordinate escaped the pointwise merge")
		}
		if heap.Domain().Equal(result, merged) {
			t.Fatal("shared coordinate collapsed into the pointwise merge")
		}
	})

	t.Run("payload", func(t *testing.T) {
		heap, values, operand := closedSemanticFixture(t, `local a = {}; local b = {}; local v = a; return { item = v }`)
		if operand.CoordinateCount() != 1 {
			t.Fatalf("coordinates=%d, want 1", operand.CoordinateCount())
		}
		left, right := tableAtoms(t, heap, values, 2)
		alternatives, alternativesOK := values.Alternatives(left, right)
		predecessor, predecessorOK := heap.EmptyObject(operand.Key())
		if !alternativesOK || !predecessorOK {
			t.Fatal("payload product fixture")
		}
		result, outcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, []valuedomain.Value{alternatives})
		if outcome != structure.Concrete {
			t.Fatalf("payload outcome=%v, want concrete", outcome)
		}

		// L2: the folded coordinate is exactly the pointwise merge of its
		// leaves — no more, and no less.
		leaves := closedSingletonLeaves(t, heap, values, operand, predecessor, [][]valuedomain.Atom{{left}, {right}})
		merged, mergedOK := closedWidenLeaves(leaves)
		if !mergedOK || !heap.Domain().Equal(result, merged) {
			t.Fatalf("payload fold differs from the pointwise merge merged=%t worlds=%d", mergedOK, result.WorldCount())
		}
		if result.WorldCount() != 1 {
			t.Fatalf("payload worlds=%d, want 1", result.WorldCount())
		}
	})

	t.Run("keyed-payload", func(t *testing.T) {
		heap, values, operand := closedSemanticFixture(t, `local a = {}; local b = {}; return { [a] = b }`)
		if operand.CoordinateCount() != 2 {
			t.Fatalf("coordinates=%d, want 2", operand.CoordinateCount())
		}
		field, fieldOK := operand.At(0)
		keyOrdinal, dynamic := field.DynamicKeyOrdinal()
		valueOrdinal := field.ValueOrdinal()
		if !fieldOK || !dynamic || keyOrdinal == valueOrdinal {
			t.Fatal("keyed-payload operand did not separate its key and payload coordinates")
		}
		left, right := tableAtoms(t, heap, values, 2)
		alternatives, alternativesOK := values.Alternatives(left, right)
		predecessor, predecessorOK := heap.EmptyObject(operand.Key())
		if !alternativesOK || !predecessorOK {
			t.Fatal("keyed-payload fixture")
		}
		inputs := make([]valuedomain.Value, operand.CoordinateCount())
		for index := range inputs {
			inputs[index] = alternatives
		}
		result, outcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
		if outcome != structure.Concrete {
			t.Fatalf("keyed-payload outcome=%v, want concrete", outcome)
		}

		// The key coordinate still enumerates one world per alternative while
		// its partner payload folds inside each of those worlds.
		if result.WorldCount() != 2 {
			t.Fatalf("keyed-payload worlds=%d, want 2", result.WorldCount())
		}

		// L4: the enumerated key axis composes by ordinary join.
		var expected heapdomain.Value
		have := false
		for _, keyAtom := range []valuedomain.Atom{left, right} {
			singleton, singletonOK := values.Singleton(keyAtom)
			if !singletonOK {
				t.Fatal("keyed-payload singleton")
			}
			branchInputs := make([]valuedomain.Value, len(inputs))
			copy(branchInputs, inputs)
			branchInputs[keyOrdinal] = singleton
			branch, branchOutcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, branchInputs)
			if branchOutcome != structure.Concrete {
				t.Fatalf("keyed-payload branch outcome=%v, want concrete", branchOutcome)
			}
			if !have {
				expected, have = branch, true
				continue
			}
			var joinedOK bool
			expected, joinedOK = heapdomain.Join(expected, branch)
			if !joinedOK {
				t.Fatal("keyed-payload branch join")
			}
		}
		if !have || !heap.Domain().Equal(result, expected) {
			t.Fatal("enumerated key axis differs from the ordinary branch join")
		}
	})
}

// closedSingletonLeaves evaluates one complete leaf per coordinate selection.
func closedSingletonLeaves(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, operand source.Closed, predecessor heapdomain.Value, selections [][]valuedomain.Atom) []heapdomain.Value {
	t.Helper()
	leaves := make([]heapdomain.Value, 0, len(selections))
	for _, selection := range selections {
		inputs := make([]valuedomain.Value, len(selection))
		for index, atom := range selection {
			singleton, singletonOK := values.Singleton(atom)
			if !singletonOK {
				t.Fatal("leaf singleton")
			}
			inputs[index] = singleton
		}
		leaf, leafOutcome := closed.EvaluateClosedForTest(heap, values, operand, predecessor, inputs)
		if leafOutcome != structure.Concrete {
			t.Fatalf("ordinary leaf outcome=%v, want concrete", leafOutcome)
		}
		leaves = append(leaves, leaf)
	}
	return leaves
}

func closedJoinLeaves(leaves []heapdomain.Value) (heapdomain.Value, bool) {
	if len(leaves) == 0 {
		return heapdomain.Value{}, false
	}
	result := leaves[0]
	for _, leaf := range leaves[1:] {
		joined, joinedOK := heapdomain.Join(result, leaf)
		if !joinedOK {
			return heapdomain.Value{}, false
		}
		result = joined
	}
	return result, true
}

func closedWidenLeaves(leaves []heapdomain.Value) (heapdomain.Value, bool) {
	if len(leaves) == 0 {
		return heapdomain.Value{}, false
	}
	result := leaves[0]
	for _, leaf := range leaves[1:] {
		widened, widenedOK := heapdomain.Widen(result, leaf)
		if !widenedOK {
			return heapdomain.Value{}, false
		}
		result = widened
	}
	return result, true
}

func TestClosedEvaluatorReceiptInvalidAndOpaqueKeys(t *testing.T) {
	heap, values, invalid := closedSemanticFixture(t, `local x = nil; return { [x] = x }`)
	nilValue, nilValueOK := values.ForRuntimeKinds(runtimekind.Bit(runtimekind.Nil))
	predecessor, predecessorOK := heap.EmptyObject(invalid.Key())
	if !nilValueOK || !predecessorOK {
		t.Fatal("invalid-key evaluator fixture")
	}
	if _, outcome := closed.EvaluateClosedForTest(heap, values, invalid, predecessor, []valuedomain.Value{nilValue}); outcome != structure.NoCandidate {
		t.Fatalf("invalid key outcome=%v, want no-candidate", outcome)
	}

	heap, values, opaqueOperand := closedSemanticFixture(t, `local key = {}; return { [key] = key }`)
	opaque, opaqueOK := values.OpaqueReference(valuedomain.ReferenceTable)
	opaqueValue, opaqueValueOK := values.Singleton(opaque)
	predecessor, predecessorOK = heap.EmptyObject(opaqueOperand.Key())
	if !opaqueOK || !opaqueValueOK || !predecessorOK {
		t.Fatal("opaque-key evaluator fixture")
	}
	result, opaqueOutcome := closed.EvaluateClosedForTest(heap, values, opaqueOperand, predecessor, []valuedomain.Value{opaqueValue})
	if opaqueOutcome != structure.Concrete {
		t.Fatalf("opaque key outcome=%v, want concrete", opaqueOutcome)
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
	foreignNilValue, foreignNilValueOK := foreignValues.ForRuntimeKinds(runtimekind.Bit(runtimekind.Nil))
	if !foreignPredecessorOK || !foreignNilValueOK {
		t.Fatal("foreign closed evaluator fixture")
	}
	if _, foreignOutcome := closed.EvaluateClosedForTest(heap, values, foreignOperand, foreignPredecessor, []valuedomain.Value{foreignNilValue}); foreignOutcome != structure.Refuse {
		t.Fatalf("closed evaluator outcome=%v on a foreign operand/schema pair, want refuse", foreignOutcome)
	}
}

func assertClosedWorldKind(t testing.TB, value heapdomain.Value, want heapdomain.WorldKind) {
	t.Helper()
	world, worldOK := value.WorldAt(0)
	if !worldOK || world.Kind() != want {
		t.Fatalf("closed world=%v/%t, want %v/true", world.Kind(), worldOK, want)
	}
}
