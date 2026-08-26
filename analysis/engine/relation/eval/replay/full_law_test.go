package replay

import (
	"testing"

	physicalapply "github.com/wippyai/go-lua/analysis/engine/relation/apply"
	testfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func correlatedPlan(t *testing.T, fixture testfixture.Fixture) arrangement.ApplyBinding {
	t.Helper()
	node, ok := fixture.CorrelatedApplyNode()
	if !ok || !node.Available() {
		t.Fatal("correlated Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() || !plan.Correlation().Available() {
		t.Fatal("correlated Apply binding")
	}
	return plan
}

func denominatorRef(t *testing.T, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	ref, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator reference")
	}
	return ref
}

func TestFullStreamsQWitnessOrderAndAuthenticatedEmptyPosting(t *testing.T) {
	fixture := testfixture.New(t, 0xF0)
	plan := correlatedPlan(t, fixture)
	baseline := len(fixture.CorrelatedApplyFrames())

	var evidence []CoordinateEvidence
	var extents []physicalapply.Results
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		if !value.Available() || !results.Available() {
			t.Fatal("invalid replay callback evidence")
		}
		evidence = append(evidence, value)
		extents = append(extents, results)
		return true
	})
	if !completed || !valid || len(evidence) != 2 || len(extents) != 2 {
		t.Fatalf("full=(%v,%v), evidence=%d extents=%d", completed, valid, len(evidence), len(extents))
	}
	rows := fixture.RowsLeft()
	if evidence[0].RowID() != rows[0] || evidence[1].RowID() != rows[1] {
		t.Fatalf("population order=%v,%v want=%v,%v", evidence[0].RowID(), evidence[1].RowID(), rows[0], rows[1])
	}

	leftRef := denominatorRef(t, fixture.RelationLeft(), fixture.KeyLeft())
	rightRef := denominatorRef(t, fixture.RelationRight(), fixture.KeyRight())
	frames := fixture.CorrelatedApplyFrames()[baseline:]
	if len(frames) != 2 {
		t.Fatalf("worker frames=%d want declaration-order q frames=2", len(frames))
	}
	_, rightScope := fixture.OverlapScopes()
	for index, frame := range frames {
		qScope := evidence[index].Scope()
		if !frame.Available() || frame.Len() != 2 {
			t.Fatalf("frame %d unavailable/arity: available=%v len=%d", index, frame.Available(), frame.Len())
		}
		gotScope, ok := fixture.Mounted().ScopeForToken(frame.Scope())
		if !ok || !gotScope.Same(qScope) {
			t.Fatalf("frame %d did not carry q scope", index)
		}
		if !fixture.Geometry().Entails(qScope, rightScope) || fixture.Geometry().Entails(rightScope, qScope) {
			t.Fatalf("frame %d failed narrower-Q/wider-child scope law", index)
		}
		slotLeft, ok := frame.At(0)
		if !ok {
			t.Fatal("left declaration slot")
		}
		slotRight, ok := frame.At(1)
		if !ok {
			t.Fatal("right declaration slot")
		}
		if !slotLeft.IsSpan() || !slotRight.IsSpan() || !slotLeft.Witness().Matches(leftRef) || !slotRight.Witness().Matches(rightRef) {
			t.Fatalf("frame %d lost declaration-order denominator witnesses", index)
		}
		if index == 0 {
			if slotLeft.Len() != 1 || !slotLeft.Witness().Contains(rows[0]) || slotRight.Len() != 1 || !slotRight.Witness().Contains(fixture.RowsRight()[0]) {
				t.Fatal("q0 did not redeem exact child postings")
			}
		} else {
			if slotLeft.Len() != 0 || slotLeft.Witness().Contains(rows[0]) || slotRight.Len() != 1 || !slotRight.Witness().Contains(fixture.RowsRight()[1]) {
				t.Fatal("q1 did not preserve authenticated empty posting")
			}
		}
		application, ok := extents[index].At(0)
		if !ok || !application.Available() || !application.Invocation().Scope().Same(frame.Scope()) {
			t.Fatalf("extent %d lost q-scoped invocation", index)
		}
	}
}

func TestFullDisjointQProducesNoChildProductAndVisitorStopIsValid(t *testing.T) {
	fixture := testfixture.New(t, 0xF1)
	plan := correlatedPlan(t, fixture)
	var evidence []CoordinateEvidence
	var extents []physicalapply.Results
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		evidence = append(evidence, value)
		extents = append(extents, results)
		return true
	})
	if !completed || !valid || len(evidence) != 2 || len(extents) != 2 || extents[0].Len() != 1 || extents[1].Len() != 0 {
		t.Fatalf("disjoint-q full=(%v,%v), evidence=%d extents=%d/%d", completed, valid, len(evidence), len(extents), func() int {
			if len(extents) == 2 {
				return extents[1].Len()
			}
			return -1
		}())
	}
	_, rightScope := fixture.OverlapScopes()
	if _, ok := fixture.Geometry().Conjoin(evidence[1].Scope(), rightScope); ok {
		t.Fatal("disjoint q scope was allowed to reach wider child product")
	}

	var stopped int
	completed, valid = Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		stopped++
		return false
	})
	if completed || !valid || stopped != 1 {
		t.Fatalf("visitor stop=(%v,%v), callbacks=%d", completed, valid, stopped)
	}

	foreign := testfixture.New(t, 0xF2)
	called := false
	completed, valid = Full(plan, fixture.Mounted(), foreign.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(CoordinateEvidence, physicalapply.Results) bool {
		called = true
		return true
	})
	if completed || valid || called {
		t.Fatalf("foreign root was admitted: completed=%v valid=%v called=%v", completed, valid, called)
	}
}

func TestFullFiltersCoordinateSupersetAgainstExactQPosting(t *testing.T) {
	fixture := testfixture.New(t, 0xF3)
	plan := correlatedPlan(t, fixture)
	baseline := len(fixture.CorrelatedApplyFrames())
	var evidence []CoordinateEvidence
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		if !value.Available() || !results.Available() {
			t.Fatal("invalid coordinate evidence or result")
		}
		evidence = append(evidence, value)
		return true
	})
	if !completed || !valid || len(evidence) != 2 {
		t.Fatalf("superset replay=(%v,%v), evidence=%d", completed, valid, len(evidence))
	}
	frames := fixture.CorrelatedApplyFrames()[baseline:]
	if len(frames) != 2 {
		t.Fatalf("superset worker frames=%d want=2", len(frames))
	}
	rowA, rowB := fixture.RowsRight()[0], fixture.RowsRight()[1]
	q0, ok := frames[0].At(1)
	if !ok || q0.Len() != 1 || !q0.Witness().Contains(rowA) || q0.Witness().Contains(rowB) {
		t.Fatalf("q0 did not filter coordinate superset to exact posting: len=%d a=%v b=%v", q0.Len(), q0.Witness().Contains(rowA), q0.Witness().Contains(rowB))
	}
	q1, ok := frames[1].At(1)
	if !ok || q1.Len() != 0 || q1.Witness().Contains(rowA) || q1.Witness().Contains(rowB) {
		t.Fatalf("q1 authenticated empty posting was not preserved: len=%d", q1.Len())
	}
}

func TestFullMixedPopulationScalarAndThreeSpanSlotsNoCartesianProduct(t *testing.T) {
	fixture := testfixture.New(t, 0xF4)
	node, ok := fixture.MixedPopulationApplyNode()
	if !ok || !node.Available() {
		t.Fatal("mixed population Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() || !plan.Correlation().Available() {
		t.Fatal("mixed population Apply binding")
	}
	replay, ok := plan.Replay()
	if !ok || !replay.Available() {
		t.Fatal("mixed population replay")
	}
	driverChild, childOK := replay.ChildAt(0)
	if !childOK || !driverChild.Available() {
		t.Fatal("population driver subtree")
	}
	driverInput, inputOK := driverChild.InputAt(0)
	if !inputOK || !driverInput.Available() {
		t.Fatal("population driver input extent")
	}
	driverLayout, populationSource, sourceOK := driverInput.Source().PopulationDriver()
	coordinate, coordinateOK := replay.Coordinate()
	if !sourceOK || !coordinateOK || populationSource.Child() != 0 || populationSource.Cell() != 2 || len(driverLayout.Columns()) != 1 || driverLayout.Columns()[0] != coordinate {
		t.Fatalf("population driver source=(%d,%d), want child 0/cell 2", populationSource.Child(), populationSource.Cell())
	}
	if driverLayout.Equal(driverInput.Binding().Values()) {
		t.Fatal("population driver retained unrelated child payload columns")
	}
	if _, partitionOK := driverInput.Source().Partition(); partitionOK {
		t.Fatal("population driver unexpectedly carried a partition source")
	}
	spanChild, childOK := replay.ChildAt(1)
	if !childOK || !spanChild.Available() {
		t.Fatal("span child replay")
	}

	baseline := len(fixture.MixedPopulationApplyFrames())
	var evidence []CoordinateEvidence
	var extents []physicalapply.Results
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		if !value.Available() || !results.Available() {
			t.Fatal("invalid mixed replay callback")
		}
		evidence = append(evidence, value)
		extents = append(extents, results)
		return true
	})
	if !completed || !valid || len(evidence) != 2 || len(extents) != 2 {
		t.Fatalf("mixed full=(%v,%v), evidence=%d extents=%d", completed, valid, len(evidence), len(extents))
	}
	rowsLeft, rowsRight := fixture.RowsLeft(), fixture.RowsRight()
	rightRef := denominatorRef(t, fixture.RelationRight(), fixture.KeyRight())
	frames := fixture.MixedPopulationApplyFrames()[baseline:]
	if len(frames) != 2 {
		t.Fatalf("mixed worker frames=%d, want one frame per population row", len(frames))
	}
	wantColumns := [3]model.ColumnID{
		fixture.PayloadColumnsRight()[0],
		fixture.PayloadColumnsRight()[1],
		fixture.KeyColumnsRight()[0],
	}
	for index, frame := range frames {
		if !frame.Available() || frame.Len() != 4 || extents[index].Len() != 1 {
			t.Fatalf("q%d frame/result arity: frame=(%v,%d), results=%d", index, frame.Available(), frame.Len(), extents[index].Len())
		}
		scalar, scalarOK := frame.At(0)
		if !scalarOK || !scalar.IsScalar() || scalar.Len() != 1 || scalar.Witness().Available() {
			t.Fatalf("q%d scalar delivery was not one witness-free scalar", index)
		}
		scalarCell, scalarCellOK := scalar.At(0)
		if !scalarCellOK || scalarCell.Address().Row() != rowsLeft[index] || scalarCell.Address().Column() != fixture.PayloadColumnsLeft()[0] || !scalarCell.Value().Same(evidence[index].Value()) {
			t.Fatalf("q%d scalar cell did not redeem the population row", index)
		}
		for slotIndex, wantColumn := range wantColumns {
			slot, slotOK := frame.At(slotIndex + 1)
			if !slotOK || !slot.IsSpan() || slot.Len() != 1 || !slot.Witness().Matches(rightRef) {
				t.Fatalf("q%d span slot %d was not one exact right posting", index, slotIndex)
			}
			cell, cellOK := slot.At(0)
			if !cellOK || cell.Address().Row() != rowsRight[index] || cell.Address().Column() != wantColumn || !cell.Value().Same(evidence[index].Value()) {
				t.Fatalf("q%d span slot %d did not redeem distinct column %v from one row", index, slotIndex, wantColumn)
			}
		}
	}
}

func TestFullMixedPopulationDriverSourceIsPositional(t *testing.T) {
	fixture := testfixture.New(t, 0xF4)
	node, ok := fixture.MixedPopulationApplyNode()
	if !ok || !node.Available() {
		t.Fatal("mixed population Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() {
		t.Fatal("mixed population Apply binding")
	}
	replay, ok := plan.Replay()
	if !ok || !replay.Available() {
		t.Fatal("mixed population replay")
	}
	driverChild, childOK := replay.ChildAt(0)
	if !childOK || !driverChild.Available() || driverChild.InputCount() != 1 || driverChild.CompleteCount() != 0 {
		t.Fatal("population driver child extent")
	}
	extent, extentOK := driverChild.InputAt(0)
	if !extentOK || !extent.Available() {
		t.Fatal("population driver input extent")
	}
	layout, source, sourceOK := extent.Source().PopulationDriver()
	coordinate, coordinateOK := replay.Coordinate()
	if !sourceOK || !coordinateOK || source.Child() != 0 || source.Cell() != 2 || len(layout.Columns()) != 1 || layout.Columns()[0] != coordinate {
		t.Fatalf("driver source=(%d,%d), want child 0/cell 2", source.Child(), source.Cell())
	}
	if layout.Equal(extent.Binding().Values()) {
		t.Fatal("population driver retained unrelated child payload columns")
	}
	denominator, denominatorOK := extent.Source().Denominator()
	if !denominatorOK || denominator != replay.Population() {
		t.Fatal("driver source widened its population denominator")
	}
}

// TestFullBroadcastsOneSharedCompleteVectorToEveryPopulationRow is the
// end-to-end generic law for an empty correlation projection. The two Q rows
// receive the same globally authenticated right vector, but only two Q
// callbacks and two global rows exist: replay neither stamps right rows with
// a site nor materializes a Q×right relation.
func TestFullBroadcastsOneSharedCompleteVectorToEveryPopulationRow(t *testing.T) {
	fixture := testfixture.New(t, 0xF5)
	node, ok := fixture.SharedCompleteApplyNode()
	if !ok || !node.Available() {
		t.Fatal("shared Complete Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() {
		t.Fatal("shared Complete Apply binding")
	}
	replay, ok := plan.Replay()
	correlation := replay.Correlation()
	projection, projectionOK := correlation.ProjectionAt(1)
	if !ok || !replay.Available() || !projectionOK || len(projection) != 0 {
		t.Fatal("shared Complete replay")
	}
	shared, ok := replay.ChildAt(1)
	if !ok || !shared.Available() || shared.InputCount() == 0 || shared.CompleteCount() == 0 {
		t.Fatal("shared child subtree")
	}
	for index := 0; index < shared.InputCount(); index++ {
		input, inputOK := shared.InputAt(index)
		if !inputOK || !input.Available() {
			t.Fatal("shared input extent")
		}
		if _, partitionOK := input.Source().Partition(); partitionOK {
			t.Fatal("shared input unexpectedly has a partition source")
		}
	}
	for index := 0; index < shared.CompleteCount(); index++ {
		complete, completeOK := shared.CompleteAt(index)
		if !completeOK || !complete.Available() {
			t.Fatal("shared Complete extent")
		}
		if _, partitionOK := complete.Source().Partition(); partitionOK {
			t.Fatal("shared Complete unexpectedly has a partition source")
		}
	}
	complete, ok := shared.CompleteAt(0)
	if !ok || !complete.Available() {
		t.Fatal("shared complete binding")
	}
	global, ok := fixture.Mounted().Denominator(complete.Binding().Denominator())
	if !ok || !global.Available() || global.Len() != 2 {
		t.Fatal("global Complete witness")
	}

	baseline := len(fixture.SharedCompleteApplyFrames())
	var evidence []CoordinateEvidence
	var extents []physicalapply.Results
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(value CoordinateEvidence, results physicalapply.Results) bool {
		if !value.Available() || !results.Available() {
			t.Fatal("invalid shared replay callback")
		}
		evidence = append(evidence, value)
		extents = append(extents, results)
		return true
	})
	if !completed || !valid || len(evidence) != 2 || len(extents) != 2 {
		t.Fatalf("shared full=(%v,%v), callbacks=%d, extents=%d", completed, valid, len(evidence), len(extents))
	}

	frames := fixture.SharedCompleteApplyFrames()[baseline:]
	if len(frames) != 2 {
		t.Fatalf("shared worker frames=%d, want one per Q row", len(frames))
	}
	wantRows := fixture.RowsRight()
	for index, frame := range frames {
		if !frame.Available() || frame.Len() != 2 || extents[index].Len() != 1 {
			t.Fatalf("q%d shared frame/result arity: frame=(%v,%d), results=%d", index, frame.Available(), frame.Len(), extents[index].Len())
		}
		span, spanOK := frame.At(1)
		if !spanOK || !span.IsSpan() || span.Len() != len(wantRows) || !span.Witness().Same(global) {
			t.Fatalf("q%d did not retain exactly one global Complete vector", index)
		}
		for rowIndex, want := range wantRows {
			cell, cellOK := span.At(rowIndex)
			if !cellOK || cell.Address().Row() != want {
				t.Fatalf("q%d global span row %d=%v, want %v", index, rowIndex, cell.Address().Row(), want)
			}
		}
		application, applicationOK := extents[index].At(0)
		if !applicationOK || !application.Available() {
			t.Fatalf("q%d shared application", index)
		}
		vector, vectorOK := application.Invocation().ChildAt(1)
		if !vectorOK || vector.Len() != len(wantRows) {
			t.Fatalf("q%d shared invocation vector len=%d", index, vector.Len())
		}
		for rowIndex, want := range wantRows {
			tuple, tupleOK := vector.At(rowIndex)
			row, rowOK := tuple.At(0)
			if !tupleOK || !rowOK || tuple.Len() != 1 || row != want {
				t.Fatalf("q%d shared invocation row %d did not reuse global source", index, rowIndex)
			}
		}
	}
}

// TestFullBroadcastsAuthenticatedEmptyCompleteVector confirms the empty
// shared denominator remains one real Complete span for each Q row. It must
// not ask for a partition directory or collapse into a missing child.
func TestFullBroadcastsAuthenticatedEmptyCompleteVector(t *testing.T) {
	fixture := testfixture.New(t, 0xF6)
	node, ok := fixture.SharedEmptyApplyNode()
	if !ok || !node.Available() {
		t.Fatal("shared empty Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() {
		t.Fatal("shared empty Apply binding")
	}
	replay, ok := plan.Replay()
	projection, projectionOK := replay.Correlation().ProjectionAt(1)
	if !ok || !replay.Available() || !projectionOK || len(projection) != 0 {
		t.Fatal("shared empty replay")
	}
	shared, ok := replay.ChildAt(1)
	if !ok || !shared.Available() || shared.InputCount() == 0 || shared.CompleteCount() == 0 {
		t.Fatal("shared empty child")
	}
	complete, ok := shared.CompleteAt(0)
	if !ok {
		t.Fatal("shared empty complete binding")
	}
	empty, ok := fixture.Mounted().Denominator(complete.Binding().Denominator())
	if !ok || !empty.Available() || empty.Len() != 0 {
		t.Fatal("empty global witness")
	}

	baseline := len(fixture.SharedEmptyApplyFrames())
	var extents []physicalapply.Results
	completed, valid := Full(plan, fixture.Mounted(), fixture.BothRoot(), fixture.Geometry(), fixture.Scratch(), func(_ CoordinateEvidence, results physicalapply.Results) bool {
		extents = append(extents, results)
		return true
	})
	if !completed || !valid || len(extents) != 2 {
		t.Fatalf("shared empty full=(%v,%v), extents=%d", completed, valid, len(extents))
	}
	frames := fixture.SharedEmptyApplyFrames()[baseline:]
	if len(frames) != 2 {
		t.Fatalf("shared empty frames=%d, want=2", len(frames))
	}
	for index, frame := range frames {
		if !frame.Available() || frame.Len() != 2 || extents[index].Len() != 1 {
			t.Fatalf("q%d empty shared frame/result arity", index)
		}
		span, spanOK := frame.At(1)
		if !spanOK || !span.IsSpan() || span.Len() != 0 || !span.Witness().Same(empty) {
			t.Fatalf("q%d lost authenticated empty global Complete span", index)
		}
		application, applicationOK := extents[index].At(0)
		if !applicationOK || !application.Available() {
			t.Fatalf("q%d empty shared application", index)
		}
		vector, vectorOK := application.Invocation().ChildAt(1)
		if !vectorOK || vector.Len() != 0 {
			t.Fatalf("q%d empty shared invocation did not retain an empty vector", index)
		}
	}
}
