package apply_test

import (
	"reflect"
	"testing"

	physicalapply "github.com/wippyai/go-lua/analysis/engine/relation/apply"
	physicalcomplete "github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	physicalinput "github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

func planWitnesses(t testing.TB, mounted witness.Mounted, plan arrangement.ApplyBinding) []binding.DenominatorWitness {
	t.Helper()
	deliveries := plan.Deliveries()
	result := make([]binding.DenominatorWitness, len(deliveries))
	for index, delivery := range deliveries {
		ref := delivery.Requirement().Input().Denominator
		witnessValue, ok := mounted.Denominator(ref)
		if !ok || !witnessValue.Available() || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(ref) {
			t.Fatalf("delivery %d denominator witness", index)
		}
		result[index] = witnessValue
	}
	return result
}

func denominatorWitness(t testing.TB, mounted witness.Mounted, ref model.DenominatorRef) binding.DenominatorWitness {
	t.Helper()
	witnessValue, ok := mounted.Denominator(ref)
	if !ok || !witnessValue.Available() || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(ref) {
		t.Fatal("denominator witness")
	}
	return witnessValue
}

// scalarBatch is the actual Input physical output. It does not reconstruct a
// payload reader or tuple stream: Input's sealed Values layout supplies the
// complete authored row vector and its Scan layout supplies the range proof.
func scalarBatch(t testing.TB, fixture testfixture.Fixture, left bool) tuple.Batch {
	t.Helper()
	mounted := fixture.Mounted()
	var node arrangement.Node
	var ok bool
	if left {
		node, ok = fixture.LeftInputNode()
	} else {
		node, ok = fixture.RightInputNode()
	}
	if !ok || !node.Available() {
		t.Fatal("sealed scalar source")
	}
	input, inputOK := node.Input()
	if !inputOK || !input.Available() {
		t.Fatal("sealed scalar range")
	}
	reader, readerOK := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !readerOK || !reader.Available() {
		t.Fatal("sealed scalar values reader")
	}
	batches, batchesOK := physicalinput.Execute(input, mounted, reader)
	if !batchesOK || len(batches) != 1 || !batches[0].ValidFor(mounted) || batches[0].Len() == 0 {
		t.Fatalf("Input scalar batch ok=%t batches=%d", batchesOK, len(batches))
	}
	return batches[0]
}

func twoScalarBinding(t testing.TB, fixture testfixture.Fixture) arrangement.ApplyBinding {
	t.Helper()
	node, nodeOK := fixture.TwoScalarApplyNode()
	bindingValue, bindingOK := node.Apply()
	if !nodeOK || !bindingOK || !bindingValue.Available() || len(bindingValue.Deliveries()) != 2 {
		t.Fatal("two-scalar Apply binding")
	}
	return bindingValue
}

func scalarCompleteBinding(t testing.TB, fixture testfixture.Fixture) arrangement.ApplyBinding {
	t.Helper()
	node, nodeOK := fixture.ScalarCompleteApplyNode()
	bindingValue, bindingOK := node.Apply()
	if !nodeOK || !bindingOK || !bindingValue.Available() || len(bindingValue.Deliveries()) != 2 {
		t.Fatal("scalar-complete Apply binding")
	}
	return bindingValue
}

func scalarEmptyCompleteBinding(t testing.TB, fixture testfixture.Fixture) arrangement.ApplyBinding {
	t.Helper()
	node, nodeOK := fixture.ScalarEmptyCompleteApplyNode()
	bindingValue, bindingOK := node.Apply()
	if !nodeOK || !bindingOK || !bindingValue.Available() || len(bindingValue.Deliveries()) != 2 {
		t.Fatal("scalar-empty-complete Apply binding")
	}
	return bindingValue
}

func completeLeftBatch(t testing.TB, fixture testfixture.Fixture) tuple.Batch {
	t.Helper()
	bindingValue, bindingOK := fixture.CompleteBinding()
	if !bindingOK || !bindingValue.Available() {
		t.Fatal("complete binding")
	}
	result, resultOK := physicalcomplete.Execute(bindingValue, fixture.Mounted(), scalarBatch(t, fixture, true), denominatorWitness(t, fixture.Mounted(), bindingValue.Denominator()))
	if !resultOK || !result.ValidFor(fixture.Mounted()) || result.Len() == 0 {
		t.Fatal("complete left batch")
	}
	return result
}

func emptyCompleteBatch(t testing.TB, fixture testfixture.Fixture) tuple.Batch {
	t.Helper()
	node, nodeOK := fixture.EmptyInputNode()
	input, inputOK := node.Input()
	rangeBinding, rangeOK := input.Range()
	if !nodeOK || !inputOK || !rangeOK || !rangeBinding.Available() {
		t.Fatal("empty sealed Input")
	}
	scope, _ := fixture.OverlapScopes()
	emptyInput, emptyOK := tuple.NewRangeBatch(fixture.Mounted(), rangeBinding, scope, []tuple.Tuple{}, binding.DenominatorWitness{})
	if !emptyOK || !emptyInput.ValidFor(fixture.Mounted()) {
		t.Fatal("empty authenticated Input range")
	}
	bindingValue, bindingOK := fixture.EmptyCompleteBinding()
	if !bindingOK || !bindingValue.Available() {
		t.Fatal("empty Complete binding")
	}
	result, resultOK := physicalcomplete.Execute(bindingValue, fixture.Mounted(), emptyInput, denominatorWitness(t, fixture.Mounted(), bindingValue.Denominator()))
	if !resultOK || !result.ValidFor(fixture.Mounted()) || result.Len() != 0 {
		t.Fatal("genuine empty Complete batch")
	}
	return result
}

func TestExecuteCartesianTwoScalarChildrenPreservesInputOrderAndLineage(t *testing.T) {
	fixture := testfixture.New(t, 0xc1)
	mounted := fixture.Mounted()
	left, right := scalarBatch(t, fixture, true), scalarBatch(t, fixture, false)
	plan := twoScalarBinding(t, fixture)
	framesBefore := len(fixture.TwoScalarApplyFrames())

	results, ok := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{left}, {right}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if !ok || !results.Available() || results.Len() != left.Len()*right.Len() {
		t.Fatalf("cartesian execute ok=%t available=%t len=%d", ok, results.Available(), results.Len())
	}
	if results.Operation() != plan.Operation() {
		t.Fatalf("cartesian result operation=%v want=%v", results.Operation(), plan.Operation())
	}
	frames := fixture.TwoScalarApplyFrames()[framesBefore:]
	if len(frames) != results.Len() {
		t.Fatalf("worker frames=%d results=%d", len(frames), results.Len())
	}
	leftScope, rightScope := fixture.OverlapScopes()
	wantScope, scopeOK := fixture.Geometry().Conjoin(leftScope, rightScope)
	if !scopeOK {
		t.Fatal("fixture overlap scope")
	}
	lineageAuthority, lineageOK := mounted.Lineage()
	if !lineageOK || lineageAuthority == nil {
		t.Fatal("mounted lineage authority")
	}
	for leftIndex := 0; leftIndex < left.Len(); leftIndex++ {
		leftTuple, leftOK := left.At(leftIndex)
		if !leftOK {
			t.Fatalf("left tuple %d", leftIndex)
		}
		if leftTuple.Len() != 4 {
			t.Fatalf("left Input tuple has %d cells, want full authored row", leftTuple.Len())
		}
		for rightIndex := 0; rightIndex < right.Len(); rightIndex++ {
			rightTuple, rightOK := right.At(rightIndex)
			if !rightOK {
				t.Fatalf("right tuple %d", rightIndex)
			}
			if rightTuple.Len() != 4 {
				t.Fatalf("right Input tuple has %d cells, want full authored row", rightTuple.Len())
			}
			index := leftIndex*right.Len() + rightIndex
			application, applicationOK := results.At(index)
			if !applicationOK || application.Outcome().Code != outcome.NoSelection {
				t.Fatalf("application %d=%v/%t", index, application.Outcome().Code, applicationOK)
			}
			expectedLineage, expectedOK := lineageAuthority.Join(leftTuple.Lineage(), rightTuple.Lineage())
			if !expectedOK || application.Lineage() != expectedLineage {
				t.Fatalf("application %d lineage was not exactly the selected pair", index)
			}
			frame := frames[index]
			frameScope, frameScopeOK := mounted.ScopeForToken(frame.Scope())
			if !frameScopeOK || !frameScope.Same(wantScope) {
				t.Fatalf("application %d scope did not conjoin input cofibers", index)
			}
			leftRow, leftRowOK := leftTuple.SourceFor(fixture.RelationLeft())
			rightRow, rightRowOK := rightTuple.SourceFor(fixture.RelationRight())
			first, firstOK := frame.At(0)
			second, secondOK := frame.At(1)
			firstCell, firstCellOK := first.At(0)
			secondCell, secondCellOK := second.At(0)
			leftInput := plan.Deliveries()[0].Requirement().Input()
			rightInput := plan.Deliveries()[1].Requirement().Input()
			if !leftRowOK || !rightRowOK || !firstOK || !secondOK || !firstCellOK || !secondCellOK || firstCell.Address().Row() != leftRow || secondCell.Address().Row() != rightRow || firstCell.Address().Column() != leftInput.Column || secondCell.Address().Column() != rightInput.Column {
				t.Fatalf("application %d lost authored input order", index)
			}
		}
	}
}

func TestExecuteRejectsArityAndOrderedRoleMismatchWithoutWorker(t *testing.T) {
	fixture := testfixture.New(t, 0xc2)
	left, right := scalarBatch(t, fixture, true), scalarBatch(t, fixture, false)
	plan := twoScalarBinding(t, fixture)
	before := len(fixture.TwoScalarApplyFrames())
	if result, ok := physicalapply.Execute(plan, fixture.Mounted(), [][]tuple.Batch{{left}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, fixture.Mounted(), plan)); ok || result.Available() {
		t.Fatal("arity mismatch was accepted")
	}
	if result, ok := physicalapply.Execute(plan, fixture.Mounted(), [][]tuple.Batch{{right}, {left}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, fixture.Mounted(), plan)); ok || result.Available() {
		t.Fatal("ordered relation roles were silently swapped")
	}
	if after := len(fixture.TwoScalarApplyFrames()); after != before {
		t.Fatalf("malformed calls invoked worker: before=%d after=%d", before, after)
	}
}

func TestExecuteEmptyScalarInputIsClosedNoSelectionAndDisjointScopesDoNotFabricateFrames(t *testing.T) {
	fixture := testfixture.New(t, 0xc3)
	right := scalarBatch(t, fixture, false)
	plan := twoScalarBinding(t, fixture)
	before := len(fixture.TwoScalarApplyFrames())
	result, ok := physicalapply.Execute(plan, fixture.Mounted(), [][]tuple.Batch{[]tuple.Batch{}, {right}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, fixture.Mounted(), plan))
	if !ok || !result.Available() || result.Len() != 0 {
		t.Fatalf("empty scalar result ok=%t available=%t len=%d", ok, result.Available(), result.Len())
	}
	if result.Operation() != plan.Operation() {
		t.Fatalf("empty scalar result operation=%v want=%v", result.Operation(), plan.Operation())
	}
	if after := len(fixture.TwoScalarApplyFrames()); after != before {
		t.Fatalf("empty scalar input invoked worker: before=%d after=%d", before, after)
	}
}

func TestExecuteEmptyScalarBatchRetainsOnlyItsAuthenticatedCommonScope(t *testing.T) {
	fixture := testfixture.New(t, 0xc7)
	mounted := fixture.Mounted()
	left, right := scalarBatch(t, fixture, true), scalarBatch(t, fixture, false)
	emptyLeft, emptyOK := tuple.PreserveRange(mounted, left, left.Scope(), []tuple.Tuple{})
	if !emptyOK || !emptyLeft.ValidFor(mounted) || emptyLeft.Len() != 0 {
		t.Fatal("empty scalar batch")
	}
	plan := twoScalarBinding(t, fixture)
	results, executeOK := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{emptyLeft}, {right}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if !executeOK || !results.Available() || results.Len() != 0 {
		t.Fatalf("empty extent ok=%t available=%t len=%d", executeOK, results.Available(), results.Len())
	}
	if results.Operation() != plan.Operation() {
		t.Fatalf("empty extent operation=%v", results.Operation())
	}
	if len(results.Scopes()) == 0 {
		t.Fatal("empty extent lost its authenticated common scope")
	}
	if _, scopeOK := mounted.ScopeForToken(results.Scopes()[0]); !scopeOK {
		t.Fatal("empty extent scope was not mounted")
	}
}

func TestExecuteScalarAndCompleteSpanKeepsTheRangeWholeAndUsesDenominatorLineageWhenEmpty(t *testing.T) {
	fixture := testfixture.New(t, 0xc5)
	mounted := fixture.Mounted()
	scalar, complete := scalarBatch(t, fixture, true), completeLeftBatch(t, fixture)
	plan := scalarCompleteBinding(t, fixture)
	framesBefore := len(fixture.ScalarCompleteApplyFrames())
	results, ok := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{scalar}, {complete}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if !ok || !results.Available() || results.Len() != scalar.Len() {
		t.Fatalf("scalar+complete execute ok=%t available=%t len=%d", ok, results.Available(), results.Len())
	}
	frames := fixture.ScalarCompleteApplyFrames()[framesBefore:]
	if len(frames) != scalar.Len() {
		t.Fatalf("scalar+complete frames=%d want=%d", len(frames), scalar.Len())
	}
	lineageAuthority, lineageOK := mounted.Lineage()
	if !lineageOK || lineageAuthority == nil {
		t.Fatal("mounted lineage authority")
	}
	for index := 0; index < scalar.Len(); index++ {
		scalarTuple, scalarOK := scalar.At(index)
		application, applicationOK := results.At(index)
		first, firstOK := frames[index].At(0)
		second, secondOK := frames[index].At(1)
		if !scalarOK || !applicationOK || !firstOK || !secondOK || !first.IsScalar() || first.Len() != 1 || !second.IsSpan() || second.Len() != complete.Len() {
			t.Fatalf("scalar+complete frame %d did not preserve delivery shapes", index)
		}
		expected := scalarTuple.Lineage()
		for spanIndex := 0; spanIndex < complete.Len(); spanIndex++ {
			spanTuple, spanOK := complete.At(spanIndex)
			if !spanOK {
				t.Fatalf("complete tuple %d", spanIndex)
			}
			var joinOK bool
			expected, joinOK = lineageAuthority.Join(expected, spanTuple.Lineage())
			if !joinOK {
				t.Fatalf("span lineage %d", spanIndex)
			}
		}
		if application.Outcome().Code != outcome.NoSelection || application.Lineage() != expected {
			t.Fatalf("scalar+complete application %d did not join the whole range once", index)
		}
	}

	emptyComplete, emptyOK := tuple.PreserveRange(mounted, complete, complete.Scope(), []tuple.Tuple{})
	if !emptyOK || !emptyComplete.ValidFor(mounted) || emptyComplete.Len() != 0 {
		t.Fatal("empty complete range")
	}
	framesBefore = len(fixture.ScalarCompleteApplyFrames())
	emptyResults, emptyExecuteOK := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{scalar}, {emptyComplete}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if emptyExecuteOK || emptyResults.Available() {
		t.Fatal("empty span over a nonempty Complete denominator was accepted")
	}
	if after := len(fixture.ScalarCompleteApplyFrames()); after != framesBefore {
		t.Fatalf("nonempty-denominator omission invoked worker: before=%d after=%d", framesBefore, after)
	}

	leftScope, _ := fixture.OverlapScopes()
	// DisjointScopes returns the two sides of the hostile pair. The first
	// happens to coincide with the left fixture scope, so the second is the
	// actual contradictory cofiber for this scalar-left invocation.
	_, disjointScope := fixture.DisjointScopes()
	disjointComplete, disjointOK := tuple.PreserveRange(mounted, complete, disjointScope, []tuple.Tuple{})
	if !disjointOK || !disjointComplete.ValidFor(mounted) {
		t.Fatal("disjoint empty complete range")
	}
	if _, overlap := fixture.Geometry().Conjoin(leftScope, disjointScope); overlap {
		t.Fatal("fixture scopes unexpectedly have a common cofiber")
	}
	framesBefore = len(fixture.ScalarCompleteApplyFrames())
	disjointResults, disjointExecuteOK := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{scalar}, {disjointComplete}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if !disjointExecuteOK || !disjointResults.Available() || disjointResults.Len() != 0 {
		t.Fatalf("disjoint complete execute ok=%t available=%t len=%d", disjointExecuteOK, disjointResults.Available(), disjointResults.Len())
	}
	if after := len(fixture.ScalarCompleteApplyFrames()); after != framesBefore {
		t.Fatalf("disjoint scopes invoked worker: before=%d after=%d", framesBefore, after)
	}
}

func TestExecuteGenuinelyEmptyCompleteSpanUsesMountedDenominatorLineage(t *testing.T) {
	fixture := testfixture.New(t, 0xc6)
	mounted := fixture.Mounted()
	scalar, complete := scalarBatch(t, fixture, true), emptyCompleteBatch(t, fixture)
	plan := scalarEmptyCompleteBinding(t, fixture)
	before := len(fixture.ScalarEmptyCompleteApplyFrames())
	results, ok := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{scalar}, {complete}}, fixture.Geometry(), witness.Scope{}, planWitnesses(t, mounted, plan))
	if !ok || !results.Available() || results.Len() != scalar.Len() {
		t.Fatalf("genuine empty Complete execute ok=%t available=%t len=%d", ok, results.Available(), results.Len())
	}
	frames := fixture.ScalarEmptyCompleteApplyFrames()[before:]
	if len(frames) != results.Len() {
		t.Fatalf("genuine empty Complete frames=%d want=%d", len(frames), results.Len())
	}
	lineageAuthority, lineageOK := mounted.Lineage()
	if !lineageOK || lineageAuthority == nil {
		t.Fatal("mounted lineage authority")
	}
	denominator := plan.Deliveries()[1].Requirement().Input().Denominator
	denominatorLineage, denominatorOK := mounted.DenominatorLineage(denominator)
	if !denominatorOK {
		t.Fatal("genuine empty denominator lineage")
	}
	for index := 0; index < results.Len(); index++ {
		scalarTuple, scalarOK := scalar.At(index)
		application, applicationOK := results.At(index)
		span, spanOK := frames[index].At(1)
		expected, expectedOK := lineageAuthority.Join(scalarTuple.Lineage(), denominatorLineage)
		if !scalarOK || !applicationOK || !spanOK || !span.IsSpan() || span.Len() != 0 || !expectedOK || application.Lineage() != expected {
			t.Fatalf("genuine empty Complete application %d did not use the mounted denominator lineage", index)
		}
	}
}

func TestExecuteSeedScopeRestrictsAuthenticatedEmptyCompleteExtent(t *testing.T) {
	fixture := testfixture.New(t, 0xc8)
	mounted := fixture.Mounted()
	scalar, complete := scalarBatch(t, fixture, true), emptyCompleteBatch(t, fixture)
	plan := scalarEmptyCompleteBinding(t, fixture)
	leftScope, rightScope := fixture.OverlapScopes()
	seed, seedOK := fixture.Geometry().Conjoin(leftScope, rightScope)
	if !seedOK || !seed.ValidFor(mounted.RuntimeFence()) || seed.Same(rightScope) {
		t.Fatal("q seed was not strictly narrower than the child scope")
	}
	emptyComplete, emptyOK := tuple.PreserveRange(mounted, complete, rightScope, []tuple.Tuple{})
	if !emptyOK || !emptyComplete.ValidFor(mounted) || emptyComplete.Len() != 0 {
		t.Fatal("authenticated empty Complete posting")
	}
	before := len(fixture.ScalarEmptyCompleteApplyFrames())
	results, executeOK := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{scalar}, {emptyComplete}}, fixture.Geometry(), seed, planWitnesses(t, mounted, plan))
	if !executeOK || !results.Available() || results.Len() != scalar.Len() {
		t.Fatalf("seeded empty Complete execute ok=%t available=%t len=%d", executeOK, results.Available(), results.Len())
	}
	frames := fixture.ScalarEmptyCompleteApplyFrames()[before:]
	if len(frames) != results.Len() {
		t.Fatalf("seeded empty Complete frames=%d want=%d", len(frames), results.Len())
	}
	for index, frame := range frames {
		frameScope, scopeOK := mounted.ScopeForToken(frame.Scope())
		if !scopeOK || !frameScope.Same(seed) {
			t.Fatalf("frame %d escaped q seed", index)
		}
	}
}

func TestRepeatedRelationSourcesRetainDistinctPositionalProvenance(t *testing.T) {
	fixture := testfixture.New(t, 0xc4)
	left := scalarBatch(t, fixture, true)
	value, ok := left.At(0)
	if !ok {
		t.Fatal("left tuple")
	}
	combined, ok := tuple.Combine(fixture.Mounted(), fixture.Geometry(), value, value)
	if !ok || !combined.Available() || combined.SourceLen() != 2 || combined.Len() != 2*value.Len() {
		t.Fatal("same-relation tuple did not retain both sealed source occurrences")
	}
	for index := 0; index < value.Len(); index++ {
		leftCell, leftOK := combined.At(index)
		rightCell, rightOK := combined.At(value.Len() + index)
		if !leftOK || !rightOK || leftCell.Source() != 0 || rightCell.Source() != 1 || leftCell.Column() != rightCell.Column() {
			t.Fatalf("cell %d lost side-qualified source provenance", index)
		}
	}
}

func TestExecuteIsOneOrderedBatchVectorPerChild(t *testing.T) {
	typ := reflect.TypeOf(physicalapply.Execute)
	if typ.NumIn() != 6 || typ.In(2) != reflect.TypeOf([][]tuple.Batch{}) || typ.In(4) != reflect.TypeOf(witness.Scope{}) || typ.In(5) != reflect.TypeOf([]binding.DenominatorWitness{}) || typ.NumOut() != 2 || typ.Out(0) != reflect.TypeOf(physicalapply.Results{}) || typ.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Execute shape=%v", typ)
	}
}
