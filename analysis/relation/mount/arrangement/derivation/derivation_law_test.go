package derivation

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type shapeFixture struct {
	root        model.ExpressionID
	expression  algebra.Expression
	bindings    []Binding
	inputs      []InputBinding
	signature   signature.Signature
	physical    map[string]identity.ContentID
	relationA   model.RelationID
	relationB   model.RelationID
	relationC   model.RelationID
	destination model.RelationID
	columnA     model.ColumnID
	columnD     model.ColumnID
	keyD        model.KeyID
}

func TestBuildSealsProductionZipperStructureAndDigestLaws(t *testing.T) {
	fixture := newShapeFixture(t)
	one, ok := Build(fixture.root, fixture.expression, fixture.bindings, fixture.inputs, []signature.Signature{fixture.signature})
	if !ok || !one.Available() || one.Len() != 4 {
		t.Fatalf("first derivative = (%v, %v), want four input paths", ok, one.Len())
	}
	two, ok := Build(fixture.root, fixture.expression, fixture.bindings, fixture.inputs, []signature.Signature{fixture.signature})
	if !ok || !two.Available() || one.Digest() != two.Digest() {
		t.Fatal("repeat sealing was not deterministic")
	}

	wantKinds := [][]algebra.Kind{
		{algebra.KindPublish, algebra.KindMerge, algebra.KindApply, algebra.KindSelect, algebra.KindJoin, algebra.KindJoin},
		{algebra.KindPublish, algebra.KindMerge, algebra.KindApply, algebra.KindSelect, algebra.KindJoin, algebra.KindJoin, algebra.KindSelect},
		{algebra.KindPublish, algebra.KindMerge, algebra.KindApply, algebra.KindSelect, algebra.KindJoin, algebra.KindComplete, algebra.KindSelect},
		{algebra.KindPublish, algebra.KindMerge, algebra.KindColumnProject, algebra.KindSelect},
	}
	wantOrientations := [][]Orientation{
		{OrientationNone, OrientationNone, OrientationNone, OrientationNone, OrientationLeft, OrientationLeft},
		{OrientationNone, OrientationNone, OrientationNone, OrientationNone, OrientationLeft, OrientationRight, OrientationNone},
		{OrientationNone, OrientationNone, OrientationNone, OrientationNone, OrientationRight, OrientationNone, OrientationNone},
		{OrientationNone, OrientationNone, OrientationNone, OrientationNone},
	}
	wantSiblings := [][][]identity.ContentID{
		{{fixture.physical["publish-scan"], fixture.physical["publish-key"], fixture.physical["destination-row"]}, {fixture.physical["destination-row"]}, {fixture.physical["input-a-keyed"]}, {}, {fixture.physical["input-c"]}, {fixture.physical["input-b"]}},
		{{fixture.physical["publish-scan"], fixture.physical["publish-key"], fixture.physical["destination-row"]}, {fixture.physical["destination-row"]}, {fixture.physical["input-a-keyed"]}, {}, {fixture.physical["input-c"]}, {fixture.physical["input-a"]}, {}},
		{{fixture.physical["publish-scan"], fixture.physical["publish-key"], fixture.physical["destination-row"]}, {fixture.physical["destination-row"]}, {fixture.physical["input-a-keyed"]}, {}, {fixture.physical["input-b"]}, {fixture.physical["complete-key"]}, {}},
		{{fixture.physical["publish-scan"], fixture.physical["publish-key"], fixture.physical["destination-row"]}, {fixture.physical["destination-row"]}, {fixture.physical["destination-row"]}, {}},
	}
	for index := 0; index < one.Len(); index++ {
		path, pathOK := one.PathAt(index)
		if !pathOK || !path.Available() || path.Occurrence() != uint32(index) || path.Root() != fixture.root || path.LeafRelation() != []model.RelationID{fixture.relationA, fixture.relationB, fixture.relationC, fixture.destination}[index] {
			t.Fatalf("path %d identity/leaf mismatch", index)
		}
		if path.FrameCount() != len(wantKinds[index]) {
			t.Fatalf("path %d frame count = %d, want %d", index, path.FrameCount(), len(wantKinds[index]))
		}
		for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
			frame, frameOK := path.FrameAt(frameIndex)
			if !frameOK || frame.Kind() != wantKinds[index][frameIndex] || frame.Orientation() != wantOrientations[index][frameIndex] {
				t.Fatalf("path %d frame %d = (%v, %v), want (%v, %v)", index, frameIndex, frame.Kind(), frame.Orientation(), wantKinds[index][frameIndex], wantOrientations[index][frameIndex])
			}
			if frame.SiblingCount() != len(wantSiblings[index][frameIndex]) {
				t.Fatalf("path %d frame %d sibling count = %d, want %d", index, frameIndex, frame.SiblingCount(), len(wantSiblings[index][frameIndex]))
			}
			for siblingIndex, physical := range wantSiblings[index][frameIndex] {
				sibling, siblingOK := frame.SiblingAt(siblingIndex)
				if !siblingOK || sibling.Physical() != physical {
					t.Fatalf("path %d frame %d sibling %d physical digest changed", index, frameIndex, siblingIndex)
				}
			}
		}
		reversedKinds := make([]algebra.Kind, 0, path.FrameCount())
		reversedOrientations := make([]Orientation, 0, path.FrameCount())
		for frameIndex := path.FrameCount() - 1; frameIndex >= 0; frameIndex-- {
			frame, _ := path.FrameAt(frameIndex)
			reversedKinds = append(reversedKinds, frame.Kind())
			reversedOrientations = append(reversedOrientations, frame.Orientation())
		}
		for reverseIndex := range reversedKinds {
			forwardIndex := len(wantKinds[index]) - 1 - reverseIndex
			if reversedKinds[reverseIndex] != wantKinds[index][forwardIndex] || reversedOrientations[reverseIndex] != wantOrientations[index][forwardIndex] {
				t.Fatalf("path %d did not preserve reverse leaf-to-root order: kinds=%v orientations=%v", index, reversedKinds, reversedOrientations)
			}
		}
		if path.Leaf().Physical() != fixture.physical[[]string{"input-a", "input-b", "input-c", "destination-row"}[index]] {
			t.Fatalf("path %d leaf physical digest changed", index)
		}
		if index == 2 {
			complete, completeOK := path.FrameAt(5)
			if !completeOK || complete.Kind() != algebra.KindComplete {
				t.Fatal("Complete frame missing")
			}
			replay := complete.CompleteReplay()
			if !replay.Available() || replay.ParentNode() != complete.Node() || replay.Occurrence() != path.Occurrence() || replay.InputNode() != path.Node() || replay.Values().Physical() != fixture.physical["input-c"] || replay.Range().Physical() != fixture.physical["input-c-range"] || replay.Order().Physical() != fixture.physical["complete-key"] || replay.Denominator() != complete.Denominator() {
				t.Fatalf("Complete replay lost exact mount evidence: %#v", replay)
			}
		}
	}

	changedBindings := append([]Binding(nil), fixture.bindings...)
	for index, binding := range changedBindings {
		if binding.Access().Equal(mustAccess(t, fixture.bindings, "input-b")) {
			changedBindings[index] = mustBinding(t, binding.Access(), identity.ContentID{220})
		}
	}
	changed, ok := Build(fixture.root, fixture.expression, changedBindings, fixture.inputs, []signature.Signature{fixture.signature})
	if !ok || !changed.Available() || changed.Digest() == one.Digest() {
		t.Fatal("relevant physical binding did not change derivative digest")
	}

	missing := make([]Binding, 0, len(fixture.bindings)-1)
	for _, binding := range fixture.bindings {
		if !binding.Access().Equal(mustAccess(t, fixture.bindings, "complete-key")) {
			missing = append(missing, binding)
		}
	}
	if _, ok := Build(fixture.root, fixture.expression, missing, fixture.inputs, []signature.Signature{fixture.signature}); ok {
		t.Fatal("missing Complete sibling binding was accepted")
	}

	unsupported := algebra.NewPublish(algebra.NewProject(algebra.NewInput(fixture.relationA), algebra.NewProjectContract(fixture.destination, []algebra.ColumnMapping{algebra.NewColumnMapping(fixture.columnA, fixture.columnD)}, fixture.keyD)), algebra.NewPublishContract(fixture.destination, fixture.keyD, fixture.columnD))
	if _, ok := Build(fixture.root, unsupported, fixture.bindings, fixture.inputs, []signature.Signature{fixture.signature}); ok {
		t.Fatal("Project beneath Publish was accepted")
	}
}

func TestSealedDerivativeTypesRetainNoExpressionOrCallback(t *testing.T) {
	expressionType := reflect.TypeOf((*algebra.Expression)(nil)).Elem()
	for _, value := range []reflect.Type{
		reflect.TypeOf(Access{}), reflect.TypeOf(Binding{}), reflect.TypeOf(InputBinding{}),
		reflect.TypeOf(SiblingAccess{}), reflect.TypeOf(ChildWitness{}), reflect.TypeOf(CompleteReplay{}), reflect.TypeOf(Frame{}), reflect.TypeOf(Path{}), reflect.TypeOf(Plan{}),
	} {
		if derivativeTypeContains(value, expressionType, map[reflect.Type]bool{}) {
			t.Fatalf("%v retains algebra.Expression, resolver, or callback state", value)
		}
	}
}

func TestRowInputForRelationDefersNarrowProjectionAmbiguity(t *testing.T) {
	fixture := newShapeFixture(t)
	columnE, ok := model.IssueColumnID(fixture.destination, identity.ContentID{18})
	if !ok {
		t.Fatal("destination second column")
	}
	narrowA, ok := NewBinding(fixture.destination, model.KeyID{}, []model.ColumnID{fixture.columnD}, identity.ContentID{201})
	if !ok {
		t.Fatal("first narrow binding")
	}
	narrowB, ok := NewBinding(fixture.destination, model.KeyID{}, []model.ColumnID{columnE}, identity.ContentID{202})
	if !ok {
		t.Fatal("second narrow binding")
	}
	full, ok := NewBinding(fixture.destination, model.KeyID{}, []model.ColumnID{fixture.columnD, columnE}, identity.ContentID{203})
	if !ok {
		t.Fatal("full row binding")
	}
	value := &builder{bindings: []Binding{narrowA, narrowB, full}}
	input, ok := value.rowInputForRelation(fixture.destination)
	if !ok || !input.Available() || input.Relation() != fixture.destination || input.Physical() != (identity.ContentID{203}) {
		t.Fatalf("row input = (%v, %v, %v), want widest sealed binding", ok, input.Relation(), input.Physical())
	}
	columns := input.Columns()
	if len(columns) != 2 || columns[0] != fixture.columnD || columns[1] != columnE {
		t.Fatalf("row input columns = %v, want authored full vector", columns)
	}
}

func derivativeTypeContains(value, expressionType reflect.Type, seen map[reflect.Type]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if value == expressionType || value.Kind() == reflect.Func {
		return true
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return derivativeTypeContains(value.Elem(), expressionType, seen)
	case reflect.Map:
		return derivativeTypeContains(value.Key(), expressionType, seen) || derivativeTypeContains(value.Elem(), expressionType, seen)
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if derivativeTypeContains(value.Field(index).Type, expressionType, seen) {
				return true
			}
		}
	}
	return false
}

func newShapeFixture(t *testing.T) shapeFixture {
	t.Helper()
	owner, _ := model.IssueOwnerID(identity.ContentID{1})
	schema, _ := model.IssueSchemaID(owner, identity.ContentID{2})
	typeID, _ := model.IssueTypeID(owner, identity.ContentID{3})
	relationA, _ := model.IssueRelationID(owner, identity.ContentID{4})
	relationB, _ := model.IssueRelationID(owner, identity.ContentID{5})
	relationC, _ := model.IssueRelationID(owner, identity.ContentID{6})
	destination, _ := model.IssueRelationID(owner, identity.ContentID{7})
	columnA, _ := model.IssueColumnID(relationA, identity.ContentID{8})
	columnB, _ := model.IssueColumnID(relationB, identity.ContentID{9})
	columnC, _ := model.IssueColumnID(relationC, identity.ContentID{10})
	columnD, _ := model.IssueColumnID(destination, identity.ContentID{11})
	keyA, _ := model.IssueKeyID(relationA, identity.ContentID{12})
	keyC, _ := model.IssueKeyID(relationC, identity.ContentID{13})
	keyD, _ := model.IssueKeyID(destination, identity.ContentID{14})
	scope, _ := model.IssueScopeID(owner, identity.ContentID{15})
	denominatorA, _ := model.NewDenominatorRef(relationA, keyA)
	denominatorC, _ := model.NewDenominatorRef(relationC, keyC)
	denominatorD, _ := model.NewDenominatorRef(destination, keyD)
	operationID, _ := model.IssueOperationID(owner, identity.ContentID{16})
	expressionID, _ := model.IssueExpressionID(owner, identity.ContentID{17})
	selectContract := algebra.NewSelectContract(algebra.SelectByScope, scope)
	inputA := algebra.Expression(algebra.NewInput(relationA))
	inputB := algebra.Expression(algebra.NewInput(relationB))
	inputC := algebra.Expression(algebra.NewInput(relationC))
	inputD := algebra.Expression(algebra.NewInput(destination))
	inner := algebra.NewJoin(inputA, algebra.NewSelect(inputB, selectContract), algebra.NewJoinContract([]model.ColumnID{columnA}, []model.ColumnID{columnB}))
	outer := algebra.NewJoin(inner, algebra.NewComplete(algebra.NewSelect(inputC, selectContract), denominatorC), algebra.NewJoinContract([]model.ColumnID{columnB}, []model.ColumnID{columnC}))
	main := algebra.NewApply([]algebra.Expression{algebra.NewSelect(outer, selectContract)}, algebra.NewApplyContract(signature.Identity{Operation: operationID, Version: 1}, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	carry := algebra.NewColumnProject(algebra.NewSelect(inputD, selectContract), algebra.NewColumnProjectContract([]algebra.ColumnSlot{algebra.NewColumnSlot(columnD, 0)}))
	merged := algebra.NewMerge([]algebra.Expression{main, carry}, algebra.NewMergeContract(keyD))
	expression := algebra.NewPublish(merged, algebra.NewPublishContract(destination, keyD, columnD))
	cardinality, _ := model.NewCardinality(model.ExactlyOne, 0)
	delivery, _ := signature.NewScalarDelivery()
	accepted, _ := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.Refused)
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1}, Fence: signature.Fence{Owner: owner, Schema: schema},
		Inputs:      []signature.Input{{Relation: relationA, Column: columnA, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominatorA}},
		Outputs:     []signature.Output{{Relation: destination, Column: columnD, Type: typeID, Presence: signature.ProducePresent, Denominator: denominatorD}},
		Cardinality: cardinality, Outcomes: accepted,
	})
	if !ok || !operation.Available() {
		t.Fatal("signature")
	}
	physical := map[string]identity.ContentID{
		"input-a": {101}, "input-b": {102}, "input-c": {103}, "destination-row": {104},
		"input-a-keyed": {105}, "complete-key": {106}, "publish-scan": {107}, "publish-key": {108}, "input-c-range": {109},
	}
	bindings := []Binding{
		mustBindingAccess(t, relationA, model.KeyID{}, []model.ColumnID{columnA}, physical["input-a"]),
		mustBindingAccess(t, relationB, model.KeyID{}, []model.ColumnID{columnB}, physical["input-b"]),
		mustBindingAccess(t, relationC, model.KeyID{}, []model.ColumnID{columnC}, physical["input-c"]),
		mustBindingAccess(t, relationC, model.KeyID{}, nil, physical["input-c-range"]),
		mustBindingAccess(t, destination, model.KeyID{}, []model.ColumnID{columnD}, physical["destination-row"]),
		mustBindingAccess(t, relationA, keyA, nil, physical["input-a-keyed"]),
		mustBindingAccess(t, relationC, keyC, nil, physical["complete-key"]),
		mustBindingAccess(t, destination, model.KeyID{}, nil, physical["publish-scan"]),
		mustBindingAccess(t, destination, keyD, nil, physical["publish-key"]),
		mustBindingAccess(t, relationA, keyA, []model.ColumnID{columnA}, physical["input-a-keyed"]),
	}
	inputs := []InputBinding{
		mustInput(t, relationA, []model.ColumnID{columnA}, physical["input-a"]),
		mustInput(t, relationB, []model.ColumnID{columnB}, physical["input-b"]),
		mustInput(t, relationC, []model.ColumnID{columnC}, physical["input-c"]),
		mustInput(t, destination, []model.ColumnID{columnD}, physical["destination-row"]),
	}
	return shapeFixture{root: expressionID, expression: expression, bindings: bindings, inputs: inputs, signature: operation, physical: physical, relationA: relationA, relationB: relationB, relationC: relationC, destination: destination, columnA: columnA, columnD: columnD, keyD: keyD}
}

// The fixture stores IDs used by structural assertions without widening the
// production Plan/Path surface.
func mustAccess(t *testing.T, bindings []Binding, name string) Access {
	t.Helper()
	for _, binding := range bindings {
		if name == "input-b" && binding.Access().Relation().Content() == (identity.ContentID{5}) && len(binding.Access().Columns()) == 1 {
			return binding.Access()
		}
		if name == "complete-key" && binding.Access().Key().Content() == (identity.ContentID{13}) {
			return binding.Access()
		}
	}
	t.Fatalf("missing access %s", name)
	return Access{}
}

func mustBinding(t *testing.T, access Access, physical identity.ContentID) Binding {
	t.Helper()
	binding, ok := NewBinding(access.Relation(), access.Key(), access.Columns(), physical)
	if !ok {
		t.Fatal("binding")
	}
	return binding
}

func mustBindingAccess(t *testing.T, relation model.RelationID, key model.KeyID, columns []model.ColumnID, physical identity.ContentID) Binding {
	t.Helper()
	access, ok := NewAccess(relation, key, columns)
	if !ok {
		t.Fatal("access")
	}
	return mustBinding(t, access, physical)
}

func mustInput(t *testing.T, relation model.RelationID, columns []model.ColumnID, physical identity.ContentID) InputBinding {
	t.Helper()
	inputExpression := algebra.NewInput(relation)
	input, ok := NewInputBinding(inputExpression.Digest(), relation, columns, physical)
	if !ok {
		t.Fatal("input binding")
	}
	return input
}
