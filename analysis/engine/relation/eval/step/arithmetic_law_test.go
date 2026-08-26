package step

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture/arithmetic"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// TestExecuteArithmeticPlanRedeemsEverySealedBoundary makes the first
// end-to-end physical specimen a law of the evaluator, rather than allowing
// runtime to collapse a failed child boundary into one opaque unavailable
// result. The fixture's only scheduled root is
// Publish(Apply(Join(Input, Input))); the Join supplies the candidate/source
// correspondence and the single composite child supplies all three scalar
// slots through the sealed slotSource map. A Cartesian Apply would produce
// four frames and rely on a worker-side NoSelection filter; the canonical
// plan produces exactly two matched applications.
func TestExecuteArithmeticPlanRedeemsEverySealedBoundary(t *testing.T) {
	fixture := arithmetic.New(t)
	session, ok := New(fixture.Mounted(), fixture.Base(), fixture.View())
	if !ok || !session.Available() {
		t.Fatal("arithmetic evaluator session")
	}
	execution := fixture.Mounted().Arrangement().Execution()
	schedules := execution.Schedules()
	if len(schedules) != 1 {
		t.Fatalf("arithmetic schedules=%d", len(schedules))
	}
	entry := schedules[0]
	node, ok := session.redeem(entry)
	if !ok || !node.Available() || node.Kind() != algebra.KindPublish {
		t.Fatal("arithmetic sealed publish root")
	}
	children := node.Children()
	if len(children) != 1 || children[0].Kind() != algebra.KindApply {
		t.Fatal("arithmetic sealed apply child")
	}
	applyNode := children[0]
	inputs := applyNode.Children()
	if len(inputs) != 1 || inputs[0].Kind() != algebra.KindJoin {
		t.Fatalf("arithmetic inputs=%d", len(inputs))
	}
	joinInputs := inputs[0].Children()
	if len(joinInputs) != 2 {
		t.Fatalf("arithmetic join inputs=%d", len(joinInputs))
	}
	for index, input := range joinInputs {
		value, inputOK := session.executeNode(input)
		if !inputOK || !value.available() || value.kind != algebra.KindInput {
			t.Fatalf("arithmetic join input %d did not redeem", index)
		}
	}
	application, applyOK := session.executeNode(applyNode)
	if !applyOK || !application.available() || application.kind != algebra.KindApply {
		t.Fatal("arithmetic Apply did not redeem")
	}
	if len(application.applications) != 1 || application.applications[0].Len() != 2 {
		t.Fatalf("arithmetic applications=%d results=%d", len(application.applications), application.applications[0].Len())
	}
	publishBinding, bindingOK := node.Publish()
	if !bindingOK || !publishBinding.Available() {
		t.Fatal("arithmetic sealed publication binding")
	}
	destination := publishBinding.Destination().Access().Relation()
	base := fixture.Base()
	for resultIndex, results := range application.applications {
		if !results.Available() {
			t.Fatalf("arithmetic Apply results %d unavailable", resultIndex)
		}
		for applicationIndex := 0; applicationIndex < results.Len(); applicationIndex++ {
			value, valueOK := results.At(applicationIndex)
			if !valueOK || !value.Available() {
				t.Fatalf("arithmetic application %d:%d unavailable", resultIndex, applicationIndex)
			}
			if value.Outcome().Code != outcome.Produced {
				t.Fatalf("arithmetic application %d:%d outcome=%v want Produced", resultIndex, applicationIndex, value.Outcome().Code)
			}
			permit, permitOK := session.door.WideningFor(entry, destination, value)
			if !permitOK {
				t.Fatalf("arithmetic application %d:%d widening admission", resultIndex, applicationIndex)
			}
			settlement := session.door.Publish(base, session.scratch, value, permit)
			if !settlement.Available() {
				t.Fatalf("arithmetic application %d:%d publish outcome=%v proposals=%d widening=%t", resultIndex, applicationIndex, value.Outcome().Code, value.Len(), permit.Available())
			}
			base = settlement.Next()
		}
	}
	publication, publishOK := session.execute(entry, node)
	if !publishOK || !publication.available() || publication.kind != algebra.KindPublish {
		t.Fatal("arithmetic Publish did not redeem")
	}
	if len(publication.applications) != len(application.applications) {
		t.Fatalf("arithmetic Publish dropped child Apply extents: got=%d want=%d", len(publication.applications), len(application.applications))
	}
	for index, retained := range publication.applications {
		if !retained.Available() || retained.Operation() != application.applications[index].Operation() || retained.Len() != application.applications[index].Len() {
			t.Fatalf("arithmetic Publish changed child Apply extent %d", index)
		}
	}
}

// A valid Input over the unseeded right relation returns a non-nil empty
// batch vector. Apply must preserve that closed empty extent exactly: nil
// means a malformed/missing child and must be refused by the Apply kernel,
// while a non-nil empty vector means the cartesian product has no selections.
func TestExecuteApplyPreservesClosedEmptyChildVector(t *testing.T) {
	fixture := testfixture.New(t, 0xD3)
	session, ok := New(fixture.Mounted(), fixture.LeftRoot(), fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("empty-child evaluator session")
	}
	node, ok := fixture.TwoScalarApplyNode()
	if !ok || !node.Available() || node.Kind() != algebra.KindApply {
		t.Fatal("two-scalar Apply node")
	}
	framesBefore := len(fixture.TwoScalarApplyFrames())
	value, executeOK := session.executeNode(node)
	if !executeOK || !value.available() || value.kind != algebra.KindApply || len(value.applications) != 1 {
		t.Fatal("closed empty Apply did not redeem")
	}
	results := value.applications[0]
	if !results.Available() || results.Len() != 0 {
		t.Fatalf("closed empty Apply results available=%t len=%d", results.Available(), results.Len())
	}
	if framesAfter := len(fixture.TwoScalarApplyFrames()); framesAfter != framesBefore {
		t.Fatalf("closed empty Apply invoked worker: before=%d after=%d", framesBefore, framesAfter)
	}
}

// The evaluator hands Apply one ordered batch vector per authored child. This
// law proves that it neither flattens the two input streams nor reorders the
// delivery positions before the multi-child kernel forms its cartesian frames.
func TestExecuteApplyDeliversTwoScalarCartesianFrames(t *testing.T) {
	fixture := testfixture.New(t, 0xD4)
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("two-scalar evaluator session")
	}
	node, ok := fixture.TwoScalarApplyNode()
	if !ok || !node.Available() || node.Kind() != algebra.KindApply {
		t.Fatal("two-scalar Apply node")
	}
	framesBefore := len(fixture.TwoScalarApplyFrames())
	value, executeOK := session.executeNode(node)
	if !executeOK || !value.available() || value.kind != algebra.KindApply || len(value.applications) != 1 {
		t.Fatal("two-scalar Apply did not redeem")
	}
	results := value.applications[0]
	if !results.Available() || results.Len() != 4 {
		t.Fatalf("two-scalar Apply results available=%t len=%d", results.Available(), results.Len())
	}
	frames := fixture.TwoScalarApplyFrames()[framesBefore:]
	if len(frames) != results.Len() {
		t.Fatalf("two-scalar worker frames=%d results=%d", len(frames), results.Len())
	}
	for index := 0; index < results.Len(); index++ {
		application, applicationOK := results.At(index)
		if !applicationOK || !application.Available() || application.Outcome().Code != outcome.NoSelection {
			t.Fatalf("two-scalar application %d", index)
		}
		leftSlot, leftSlotOK := frames[index].At(0)
		rightSlot, rightSlotOK := frames[index].At(1)
		leftCell, leftCellOK := leftSlot.At(0)
		rightCell, rightCellOK := rightSlot.At(0)
		if !leftSlotOK || !rightSlotOK || !leftSlot.IsScalar() || !rightSlot.IsScalar() || !leftCellOK || !rightCellOK || leftCell.Address().Row().Relation() != fixture.RelationLeft() || rightCell.Address().Row().Relation() != fixture.RelationRight() {
			t.Fatalf("two-scalar frame %d lost authored delivery order", index)
		}
	}
}
