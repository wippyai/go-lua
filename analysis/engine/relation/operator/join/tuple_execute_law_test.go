package join_test

import (
	"reflect"
	"testing"

	physicaljoin "github.com/wippyai/go-lua/analysis/engine/relation/operator/join"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// inputSource uses the sealed Input node only for its producer range
// authority. Cells come from the exact declared correspondence vector reader
// and are admitted into that range through the canonical tuple constructor.
// This keeps tests from fabricating a range or a second row representation.
func inputSource(t testing.TB, fixture testfixture.Fixture, left bool) tuple.Batch {
	t.Helper()
	mounted := fixture.Mounted()
	var node arrangement.Node
	var reader read.Reader
	var ok bool
	var emptyScope witness.Scope
	if left {
		node, ok = fixture.LeftInputNode()
		reader, ok = fixture.ReaderLeftPayload(fixture.BothRoot())
		emptyScope, _ = fixture.OverlapScopes()
	} else {
		node, ok = fixture.RightInputNode()
		reader, ok = fixture.ReaderRightPayload(fixture.BothRoot())
		_, emptyScope = fixture.OverlapScopes()
	}
	if !ok || !node.Available() || !reader.Available() {
		t.Fatal("sealed input source")
	}
	inputBinding, bindingOK := node.Input()
	rangeBinding, rangeOK := inputBinding.Range()
	if !bindingOK || !inputBinding.Available() || !rangeOK || !rangeBinding.Available() {
		t.Fatal("sealed input binding")
	}
	values := make([]tuple.Tuple, 0, 2)
	scope := emptyScope
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(mounted, reader, row)
		if !valueOK {
			t.Fatal("tuple input")
		}
		if len(values) == 0 {
			scope = value.Scope()
		} else if !scope.Same(value.Scope()) {
			t.Fatal("fixture input crossed cofibers")
		}
		values = append(values, value)
		return true
	})
	if !completed || !valid || !scope.ValidFor(mounted.RuntimeFence()) {
		t.Fatalf("input scan=(%v,%v)", completed, valid)
	}
	batch, batchOK := tuple.NewRangeBatch(mounted, rangeBinding, scope, values, bindingpkg.DenominatorWitness{})
	if !batchOK || !batch.ValidFor(mounted) {
		t.Fatal("input range batch")
	}
	return batch
}

func exactRangeBatch(t testing.TB, mounted witness.Mounted, source tuple.Batch, values []tuple.Tuple) tuple.Batch {
	t.Helper()
	result, ok := tuple.NewRangeBatch(mounted, source.Range(), source.Scope(), values, bindingpkg.DenominatorWitness{})
	if !ok || !result.ValidFor(mounted) {
		t.Fatal("tuple range batch")
	}
	return result
}

func bindingFor(t testing.TB, fixture testfixture.Fixture) arrangement.JoinBinding {
	t.Helper()
	node, nodeOK := fixture.JoinNode()
	binding, ok := node.Join()
	if !nodeOK || !ok || !binding.Available() {
		t.Fatal("join binding")
	}
	return binding
}

func correspondenceMatch(mounted witness.Mounted, binding arrangement.JoinBinding, left, right tuple.Tuple) bool {
	leftColumns, rightColumns := binding.Left().Columns(), binding.Right().Columns()
	if len(leftColumns) == 0 || len(leftColumns) != len(rightColumns) {
		return false
	}
	for index, leftColumn := range leftColumns {
		leftCell, leftOK := left.CellFor(leftColumn)
		rightCell, rightOK := right.CellFor(rightColumns[index])
		if !leftOK || !rightOK || leftCell.Type() != rightCell.Type() || !tuple.SemanticEqual(mounted, leftCell.Type(), leftCell.Value(), rightCell.Value()) {
			return false
		}
	}
	return true
}

func matchingPair(t testing.TB, mounted witness.Mounted, binding arrangement.JoinBinding, left, right tuple.Batch) (tuple.Tuple, tuple.Tuple) {
	t.Helper()
	for leftIndex := 0; leftIndex < left.Len(); leftIndex++ {
		leftValue, _ := left.At(leftIndex)
		for rightIndex := 0; rightIndex < right.Len(); rightIndex++ {
			rightValue, _ := right.At(rightIndex)
			if correspondenceMatch(mounted, binding, leftValue, rightValue) {
				return leftValue, rightValue
			}
		}
	}
	t.Fatal("fixture has no matching correspondence")
	return tuple.Tuple{}, tuple.Tuple{}
}

func TestTupleJoinExactMatchConcatenatesAuthorities(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	leftBatch := inputSource(t, fixture, true)
	rightBatch := inputSource(t, fixture, false)
	binding := bindingFor(t, fixture)
	left, right := matchingPair(t, fixture.Mounted(), binding, leftBatch, rightBatch)
	result, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftBatch, rightBatch)
	if !ok || !result.ValidFor(fixture.Mounted()) || result.Len() != 1 {
		t.Fatalf("result=(%v,%v) len=%d", ok, result.Available(), result.Len())
	}
	if result.Range().Producer() != leftBatch.Range().Producer() {
		t.Fatal("join did not preserve left input range")
	}
	output, outputOK := result.At(0)
	if !outputOK || output.SourceLen() != 2 || output.Len() != left.Len()+right.Len() {
		t.Fatalf("output shape sources=%d cells=%d", output.SourceLen(), output.Len())
	}
	for index, source := range append(left.Sources(), right.Sources()...) {
		got, sourceOK := output.SourceAt(index)
		if !sourceOK || got != source {
			t.Fatalf("source %d=%v want=%v", index, got, source)
		}
	}
	for index := 0; index < left.Len(); index++ {
		got, _ := output.At(index)
		want, _ := left.At(index)
		if got.Column() != want.Column() || !got.Value().Same(want.Value()) {
			t.Fatalf("left cell %d was not retained", index)
		}
	}
	for index := 0; index < right.Len(); index++ {
		got, _ := output.At(left.Len() + index)
		want, _ := right.At(index)
		if got.Column() != want.Column() || !got.Value().Same(want.Value()) {
			t.Fatalf("right cell %d was not retained", index)
		}
	}
}

func TestTupleJoinNoSelectionAndContradictoryScope(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	leftBatch := inputSource(t, fixture, true)
	rightBatch := inputSource(t, fixture, false)
	binding := bindingFor(t, fixture)
	left, rightMatch := matchingPair(t, fixture.Mounted(), binding, leftBatch, rightBatch)
	var rightMiss tuple.Tuple
	for index := 0; index < rightBatch.Len(); index++ {
		candidate, _ := rightBatch.At(index)
		if !correspondenceMatch(fixture.Mounted(), binding, left, candidate) {
			rightMiss = candidate
			break
		}
	}
	if !rightMiss.Available() || !correspondenceMatch(fixture.Mounted(), binding, left, rightMatch) {
		t.Fatal("fixture unmatched correspondence")
	}
	rightMissBatch := exactRangeBatch(t, fixture.Mounted(), rightBatch, []tuple.Tuple{rightMiss})
	leftMatchBatch := exactRangeBatch(t, fixture.Mounted(), leftBatch, []tuple.Tuple{left})
	if result, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftMatchBatch, rightMissBatch); !ok || result.Len() != 0 || !result.Scope().Same(left.Scope()) {
		t.Fatalf("nonmatching tuple=(%v,%v) len=%d", ok, result.Available(), result.Len())
	}
	_, disjoint := fixture.DisjointScopes()
	empty, ok := tuple.NewRangeBatch(fixture.Mounted(), rightBatch.Range(), disjoint, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
	if !ok || !empty.ValidFor(fixture.Mounted()) {
		t.Fatal("empty contradictory batch")
	}
	if result, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftBatch, empty); !ok || result.Len() != 0 || !result.Scope().Same(left.Scope()) {
		t.Fatalf("contradictory scope=(%v,%v) len=%d", ok, result.Available(), result.Len())
	}
}

func TestTupleJoinUnavailableBatchRefuses(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	_ = inputSource(t, fixture, true)
	rightBatch := inputSource(t, fixture, false)
	var unavailable tuple.Batch
	if batch, batchOK := physicaljoin.Join(bindingFor(t, fixture), fixture.Mounted(), fixture.Geometry(), unavailable, rightBatch); batchOK || batch.Available() {
		t.Fatal("unavailable input batch was accepted")
	}
}

func TestTupleJoinPreservesLineageAndAuthoredPermutation(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	leftBatch := inputSource(t, fixture, true)
	rightBatch := inputSource(t, fixture, false)
	binding := bindingFor(t, fixture)
	leftMatch, rightMatch := matchingPair(t, fixture.Mounted(), binding, leftBatch, rightBatch)
	forward, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), leftBatch, rightBatch)
	if !ok || forward.Len() != 1 {
		t.Fatalf("forward=(%v,%v) len=%d", ok, forward.Available(), forward.Len())
	}
	leftValues, rightValues := leftBatch.Tuples(), rightBatch.Tuples()
	for first, last := 0, len(leftValues)-1; first < last; first, last = first+1, last-1 {
		leftValues[first], leftValues[last] = leftValues[last], leftValues[first]
	}
	for first, last := 0, len(rightValues)-1; first < last; first, last = first+1, last-1 {
		rightValues[first], rightValues[last] = rightValues[last], rightValues[first]
	}
	backwardLeft := exactRangeBatch(t, fixture.Mounted(), leftBatch, leftValues)
	backwardRight := exactRangeBatch(t, fixture.Mounted(), rightBatch, rightValues)
	backward, ok := physicaljoin.Join(binding, fixture.Mounted(), fixture.Geometry(), backwardLeft, backwardRight)
	if !ok || backward.Len() != 1 {
		t.Fatalf("backward=(%v,%v) len=%d", ok, backward.Available(), backward.Len())
	}
	forwardValue, forwardOK := forward.At(0)
	backwardValue, backwardOK := backward.At(0)
	authority, authorityOK := fixture.Mounted().Lineage()
	expected, expectedOK := authority.Join(leftMatch.Lineage(), rightMatch.Lineage())
	if !forwardOK || !backwardOK || !forwardValue.Lineage().Available() || !backwardValue.Lineage().Available() || !authorityOK || authority == nil || !expectedOK || forwardValue.Lineage() != expected {
		t.Fatal("lineage authority did not join matched sources")
	}
	if !forwardValue.Same(backwardValue) {
		t.Fatal("join did not preserve authored batch permutation")
	}
}

func TestTupleJoinHasNoCallbackStreamSurface(t *testing.T) {
	joinType := reflect.TypeOf(physicaljoin.Join)
	if joinType.Kind() != reflect.Func || joinType.NumIn() != 5 || joinType.NumOut() != 2 {
		t.Fatalf("Join shape=%v", joinType)
	}
	for index := 0; index < joinType.NumIn(); index++ {
		if joinType.In(index).Kind() == reflect.Func {
			t.Fatalf("Join input %d is callback-shaped", index)
		}
	}
	if joinType.In(3) != reflect.TypeOf(tuple.Batch{}) || joinType.In(4) != reflect.TypeOf(tuple.Batch{}) {
		t.Fatal("Join does not consume concrete tuple.Batch values")
	}
}
