package arrangement_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// The physical binding is deliberately tested from the public package.  A
// runtime consumer can select only a compiler-issued root identity and then
// follow sealed node/layout data; neither raw expressions nor Access.Resolve
// participates in this surface.
func TestDeriveBindsEveryClosedLogicalNodeToExactLayouts(t *testing.T) {
	value := newCensusFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	first, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), slot: 501}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !first.Available() {
		t.Fatal("first derive")
	}
	execution := first.Execution()
	if !execution.Available() || !execution.Fence().Same(book.Fence()) || len(execution.ExpressionIDs()) != len(value.expressions) {
		t.Fatal("sealed execution binding unavailable")
	}

	want := []algebra.Kind{
		algebra.KindInput, algebra.KindSelect, algebra.KindProject,
		algebra.KindJoin, algebra.KindMerge, algebra.KindGroup,
		algebra.KindComplete, algebra.KindApply, algebra.KindPublish,
	}
	for index, expressionID := range value.expressions {
		node, found := execution.Entry(expressionID)
		if !found || !node.Available() || node.Kind() != want[index] || !node.Digest().Available() {
			t.Fatalf("entry %d = (%v, %v), want %v", index, found, node.Kind(), want[index])
		}
	}

	input, _ := execution.Entry(value.expressions[0])
	inputBinding, ok := input.Input()
	if !ok || inputBinding.Relation() != value.relationA {
		t.Fatal("input binding unavailable")
	}
	if scan := inputBinding.Scan(); scan.Access().Key().Available() || len(scan.Columns()) != 0 || scan.Access().Relation() != value.relationA {
		t.Fatal("input did not receive exact scan layout")
	}
	if values := inputBinding.Values(); values.Access().Key().Available() || !reflect.DeepEqual(values.Columns(), []model.ColumnID{value.columnA, value.columnA2}) || values.Access().Relation() != value.relationA {
		t.Fatal("input did not receive the complete authored row vector")
	}
	inputRange, inputRangeOK := input.Range()
	bindingInputRange, bindingInputRangeOK := inputBinding.Range()
	if !inputRangeOK || !bindingInputRangeOK || inputRange.Kind() != algebra.KindInput || inputRange.Producer() != input.Digest() || bindingInputRange.Producer() != inputRange.Producer() || !inputRange.Layout().Equal(inputBinding.Scan()) || inputRange.Denominator().Available() {
		t.Fatal("input range did not authenticate its exact producer contract")
	}

	project, _ := execution.Entry(value.expressions[2])
	projectBinding, ok := project.Project()
	if !ok || projectBinding.Target().Access().Relation() != value.relationB || projectBinding.Key().Access().Key() != value.keyB || len(projectBinding.Mappings()) != 1 {
		t.Fatal("project did not receive target/key/mapping bindings")
	}
	if mapping := projectBinding.Mappings()[0]; mapping.Source() != value.columnA || mapping.Target() != value.columnB || len(mapping.Layout().Columns()) != 1 || mapping.Layout().Columns()[0] != value.columnA {
		t.Fatal("project mapping did not retain exact source vector layout")
	}

	join, _ := execution.Entry(value.expressions[3])
	joinBinding, ok := join.Join()
	if !ok || joinBinding.Left().Columns()[0] != value.columnA || joinBinding.Right().Columns()[0] != value.columnB {
		t.Fatal("join did not retain oriented vector layouts")
	}

	complete, _ := execution.Entry(value.expressions[6])
	completeBinding, ok := complete.Complete()
	if !ok || completeBinding.Denominator() != value.denominatorA || completeBinding.Key().Access().Key() != value.keyA {
		t.Fatal("complete did not retain denominator key layout")
	}
	columns := completeBinding.Columns()
	if !reflect.DeepEqual(columns, []model.ColumnID{value.columnA, value.columnA2}) {
		t.Fatalf("complete columns = %v, want exact relation contract", columns)
	}
	columns[0] = value.columnB
	if got := completeBinding.Columns(); !reflect.DeepEqual(got, []model.ColumnID{value.columnA, value.columnA2}) {
		t.Fatalf("complete columns leaked mutable projection: %v", got)
	}
	completeRange, completeRangeOK := complete.Range()
	bindingCompleteRange, bindingCompleteRangeOK := completeBinding.Range()
	if !completeRangeOK || !bindingCompleteRangeOK || completeRange.Kind() != algebra.KindComplete || completeRange.Producer() != complete.Digest() || bindingCompleteRange.Producer() != completeRange.Producer() || !completeRange.Layout().Equal(completeBinding.Key()) || completeRange.Denominator() != completeBinding.Denominator() {
		t.Fatal("complete range did not authenticate its exact producer contract")
	}

	applyNode, _ := execution.Entry(value.expressions[7])
	applyBinding, ok := applyNode.Apply()
	if !ok || applyBinding.Operation() != value.operations[0] || len(applyBinding.Deliveries()) != 1 {
		t.Fatal("apply did not retain sealed delivery binding")
	}
	if delivery := applyBinding.Deliveries()[0]; !delivery.Available() || delivery.Requirement().Operation() != value.operations[0] || delivery.Layout().Access().Relation() != value.relationA {
		t.Fatal("apply delivery was not tied to its exact mounted layout")
	}
	delivery := applyBinding.Deliveries()[0]
	deliveryAccess, deliveryAccessOK := delivery.Requirement().Access()
	if !deliveryAccessOK || !deliveryAccess.Key().Available() || delivery.Layout().Access().Key() != deliveryAccess.Key() {
		t.Fatal("apply delivery did not retain its declared denominator-key coordinate")
	}
	if slotSource := applyBinding.SlotSource(); len(slotSource) != 1 || slotSource[0] != algebra.NewSlotSource(0, 0) {
		t.Fatalf("Apply binding did not retain the sealed child/cell slot address: %#v", slotSource)
	}

	publish, _ := execution.Entry(value.expressions[8])
	publishBinding, ok := publish.Publish()
	if !ok || publishBinding.Destination().Access().Relation() != value.relationB || publishBinding.Key().Access().Key() != value.keyB {
		t.Fatal("publish did not retain destination/key layouts")
	}

	second, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), slot: 701}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !second.Available() {
		t.Fatal("second derive")
	}
	if first.Execution().LogicalDigest() != second.Execution().LogicalDigest() || first.Execution().Digest() == second.Execution().Digest() {
		t.Fatal("physical layout assignment was omitted from execution binding identity")
	}
}

func TestExecutionBindingStoresNoResolverOrCallback(t *testing.T) {
	for _, value := range []reflect.Type{
		reflect.TypeOf(arrangement.Execution{}),
		reflect.TypeOf(arrangement.Node{}),
		reflect.TypeOf(arrangement.InputBinding{}),
		reflect.TypeOf(arrangement.ProjectBinding{}),
		reflect.TypeOf(arrangement.JoinBinding{}),
		reflect.TypeOf(arrangement.ApplyBinding{}),
	} {
		for index := 0; index < value.NumField(); index++ {
			if value.Field(index).Type.Kind() == reflect.Func {
				t.Fatalf("%s stores callback field %q", value, value.Field(index).Name)
			}
		}
	}
	var zero arrangement.Execution
	if zero.Available() {
		t.Fatal("zero execution binding redeemed an unchecked plan")
	}
	if node, ok := zero.Entry(model.ExpressionID{}); ok || node.Available() {
		t.Fatal("unchecked expression entry redeemed")
	}
}

func TestExecutionLogicalNodeDirectoryRedeemsDerivationOwners(t *testing.T) {
	value := newCensusFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	plan, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), slot: 911}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !plan.Available() {
		t.Fatal("derive")
	}
	execution := plan.Execution()
	for _, expressionID := range execution.ExpressionIDs() {
		paths, pathsOK := execution.Derivation(expressionID)
		if !pathsOK {
			t.Fatal("derivation")
		}
		for pathIndex := 0; pathIndex < paths.Len(); pathIndex++ {
			path, pathOK := paths.PathAt(pathIndex)
			if !pathOK {
				t.Fatal("path")
			}
			leaf, leafOK := execution.LogicalNode(path.Node())
			if !leafOK || !leaf.Available() || leaf.Kind() != algebra.KindInput {
				t.Fatal("logical leaf directory lookup")
			}
			for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
				frame, frameOK := path.FrameAt(frameIndex)
				if !frameOK {
					t.Fatal("frame")
				}
				node, nodeOK := execution.LogicalNode(frame.Node())
				if !nodeOK || !node.Available() || node.Kind() != frame.Kind() {
					t.Fatalf("logical frame lookup kind=%v", frame.Kind())
				}
			}
		}
	}
}
