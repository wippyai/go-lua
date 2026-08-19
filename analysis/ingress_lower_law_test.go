package analysis

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestLowerProjectsSealedColumnsWithoutRetainingTheOwner(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "ingress-lower.lua", Text: []byte(`
local function identity(value)
  return value
end
return identity(1)
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("ingress compilation failed: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := ingress.Lower(artifact, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("Lower refused a sealed artifact")
	}
	if snapshot.ArtifactID() != artifact.ID() || snapshot.ProgramID() != artifact.CompileKey().ProgramID() || snapshot.SchemaID() != artifact.CompileKey().SchemaDigest() {
		t.Fatal("Lower lost the sealed artifact identity")
	}
	if snapshot.PointCount() != artifact.PointCount() || snapshot.StructuralEdgeCount() != artifact.EnvironmentEdgeCount() ||
		snapshot.LocalTransferCount() != artifact.LocalTransferCount() || snapshot.RegionCount() != artifact.RegionCount() ||
		snapshot.EventCount() != artifact.WTOEventCount() || snapshot.RulePlacementCount() != artifact.RulePlacementCount() ||
		snapshot.BodyTransportCount() != artifact.BodyCount() || snapshot.FunctionBoundaryCount() != artifact.FunctionBoundaryCount() ||
		snapshot.CallCount() != artifact.CallCount() ||
		snapshot.HeapAllocationCount() != artifact.HeapAllocationCount() || snapshot.ValuesCount() != artifact.ValuesCount() ||
		snapshot.HeapIndexCount() != artifact.HeapIndexCount() || snapshot.OccurrenceCount() != artifact.OccurrenceCount() ||
		snapshot.StaticTypeValueCount() != artifact.StaticTypeValueCount() ||
		snapshot.CallArgumentCount() != artifact.CallArgumentCount() ||
		snapshot.DiagnosticObservationCount() != artifact.DiagnosticObservationCount() ||
		snapshot.StaticTypeNodeCount() != artifact.StaticTypeNodeCount() ||
		snapshot.StaticTypeArgumentCount() != artifact.StaticTypeArgumentCount() ||
		snapshot.StaticExpressionCount() != artifact.StaticExpressionCount() ||
		snapshot.StaticInputCount() != artifact.StaticInputCount() {
		t.Fatalf("Lower column counts drifted from the sealed artifact")
	}
	for index := 0; index < snapshot.PointCount(); index++ {
		got, gotOK := snapshot.PointAt(index)
		want, wantOK := artifact.PointAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() {
			t.Fatalf("point %d drifted from the sealed artifact", index)
		}
	}
	for index := 0; index < snapshot.StaticTypeArgumentCount(); index++ {
		got, gotOK := snapshot.StaticTypeArgumentAt(index)
		want, wantOK := artifact.StaticTypeArgumentAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.CallID() != want.CallID() || got.TypesID() != want.TypesID() ||
			got.ReferenceID() != want.ReferenceID() || got.Index() != want.Index() {
			t.Fatalf("static type argument %d drifted", index)
		}
	}
	for index := 0; index < snapshot.StaticTypeNodeCount(); index++ {
		got, gotOK := snapshot.StaticTypeNodeAt(index)
		want, wantOK := artifact.StaticTypeNodeAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.Owner() != want.Owner() || got.Kind() != uint8(want.Kind()) ||
			got.LiteralKind() != want.LiteralKind() {
			t.Fatalf("static type node %d drifted", index)
		}
	}
	for index := 0; index < snapshot.StaticTypeValueCount(); index++ {
		got, gotOK := snapshot.StaticTypeValueAt(index)
		want, wantOK := artifact.StaticTypeValueAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.BodyPathID() != want.BodyPathID() || got.ReferenceID() != want.ReferenceID() ||
			got.RootID() != want.RootID() || got.Name() != want.Name() {
			t.Fatalf("static type value %d drifted from the sealed artifact", index)
		}
	}
	for index := 0; index < snapshot.StaticExpressionCount(); index++ {
		got, gotOK := snapshot.StaticExpressionAt(index)
		want, wantOK := artifact.StaticExpressionAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.ReferenceID() != want.ReferenceID() || got.Owner() != want.Owner() {
			t.Fatalf("static expression %d drifted", index)
		}
	}
	for index := 0; index < snapshot.StaticInputCount(); index++ {
		got, gotOK := snapshot.StaticInputAt(index)
		want, wantOK := artifact.StaticInputAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.Owner() != want.Owner() || got.Kind() != uint8(want.Kind()) ||
			got.ExpressionID() != want.ExpressionID() || got.SourceID() != want.SourceID() ||
			got.TargetID() != want.TargetID() || got.OperandID() != want.OperandID() ||
			got.FrontierID() != want.FrontierID() || got.Cursor() != want.Cursor() ||
			got.OperandKind() != uint8(want.OperandKind()) || got.OperandLiteral() != want.OperandLiteral() ||
			got.OperandReferenceID() != want.OperandReferenceID() ||
			got.OperandSubjectID() != want.OperandSubjectID() ||
			got.OperandBodyPathID() != want.OperandBodyPathID() {
			t.Fatalf("static input %d drifted", index)
		}
	}
	for index := 0; index < snapshot.CallCount(); index++ {
		got, gotOK := snapshot.CallAt(index)
		want, wantOK := artifact.CallAt(index)
		gotTarget, gotTargetOK := got.DirectTargetBody()
		wantTarget, wantTargetOK := want.DirectTargetBody()
		if !gotOK || !wantOK || got.ID() != want.ID() || got.BodyID() != want.BodyID() ||
			got.CalleeID() != want.CalleeID() || got.FormalID() != want.FormalID() ||
			got.ValuesID() != want.ValuesID() || got.TypeArgumentsID() != want.TypeArgumentsID() ||
			got.Form() != uint8(want.Form()) || got.OperandCount() != want.OperandCount() ||
			got.ArgumentCount() != want.ArgumentCount() ||
			gotTargetOK != wantTargetOK || (wantTargetOK && gotTarget != wantTarget) {
			t.Fatalf("call %d drifted", index)
		}
		for child := 0; child < got.OperandCount(); child++ {
			gotOperand, gotOperandOK := got.OperandAt(child)
			wantOperand, wantOperandOK := artifact.CallOperandFor(index, child)
			if !gotOperandOK || !wantOperandOK || gotOperand.ID() != wantOperand.ID() ||
				gotOperand.CallID() != wantOperand.CallID() || gotOperand.ValueID() != wantOperand.ValueID() ||
				gotOperand.SpanID() != wantOperand.SpanID() {
				t.Fatalf("call %d operand %d drifted", index, child)
			}
		}
		for child := 0; child < got.ArgumentCount(); child++ {
			gotArgument, gotArgumentOK := got.ArgumentAt(child)
			wantArgument, wantArgumentOK := artifact.CallArgumentFor(index, child)
			if !gotArgumentOK || !wantArgumentOK || !gotArgument.Available() ||
				gotArgument.ID() != wantArgument.ID() || gotArgument.CallID() != wantArgument.CallID() ||
				gotArgument.ValuesID() != wantArgument.ValuesID() || gotArgument.MemberID() != wantArgument.MemberID() ||
				gotArgument.SpanID() != wantArgument.SpanID() || gotArgument.Index() != wantArgument.Index() {
				t.Fatalf("call %d argument %d drifted", index, child)
			}
		}
	}
	if snapshot.FunctionBoundaryCount() != artifact.FunctionBoundaryCount() {
		t.Fatal("function boundary count drifted from the sealed artifact")
	}
	for index := 0; index < snapshot.FunctionBoundaryCount(); index++ {
		got, gotOK := snapshot.FunctionBoundaryAt(index)
		want, wantOK := artifact.FunctionBoundaryAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.OutcomeCount() != want.OutcomeCount() || got.CaptureCount() != want.CaptureCount() ||
			got.FormalCount() != want.FormalCount() {
			t.Fatalf("function boundary %d drifted", index)
		}
		for formalIndex := 0; formalIndex < got.FormalCount(); formalIndex++ {
			gotPort, gotPortOK := got.FormalAt(formalIndex)
			wantPort, wantPortOK := want.FormalAt(formalIndex)
			if !gotPortOK || !wantPortOK || gotPort.ID() != wantPort.ID() ||
				gotPort.CellID() != wantPort.CellID() || gotPort.StorageCellID() != wantPort.StorageCellID() {
				t.Fatalf("function formal %d/%d drifted", index, formalIndex)
			}
		}
	}
	for index := 0; index < snapshot.CallArgumentCount(); index++ {
		got, gotOK := snapshot.CallArgumentAt(index)
		want, wantOK := artifact.CallArgumentAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() ||
			got.CallID() != want.CallID() || got.ValuesID() != want.ValuesID() ||
			got.MemberID() != want.MemberID() || got.SpanID() != want.SpanID() ||
			got.Index() != want.Index() {
			t.Fatalf("call argument %d lost sealed columns", index)
		}
		byID, byIDOK := snapshot.CallArgumentForID(want.ID())
		if !byIDOK || byID.ID() != got.ID() || byID.SpanID() != got.SpanID() {
			t.Fatalf("call argument %d is not recoverable by id", index)
		}
	}
	for index := 0; index < snapshot.DiagnosticObservationCount(); index++ {
		got, gotOK := snapshot.DiagnosticObservationAt(index)
		want, wantOK := artifact.DiagnosticObservationAt(index)
		if !gotOK || !wantOK || !got.Available() || got.ID() != want.ID() || got.Kind() != want.Kind() {
			t.Fatalf("diagnostic observation %d drifted", index)
		}
		if got.Kind() == structure.DiagnosticObservationTypeConformance {
			gotConformance, gotConformanceOK := got.TypeConformance()
			wantConformance, wantConformanceOK := want.TypeConformance()
			wantPoints, wantPointsOK := wantConformance.EvidencePoints()
			gotPoints, gotPointsOK := gotConformance.EvidencePoints()
			if !gotConformanceOK || !wantConformanceOK || !wantPointsOK || !gotPointsOK ||
				gotConformance.Site() != wantConformance.Site() || len(gotPoints) != len(wantPoints) {
				t.Fatalf("diagnostic observation %d lost conformance evidence", index)
			}
		}
	}
	for index := 0; index < snapshot.OccurrenceCount(); index++ {
		got, gotOK := snapshot.OccurrenceAt(index)
		want, wantOK := artifact.OccurrenceAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() || got.PointCount() != want.PointCount() {
			t.Fatalf("occurrence %d lost points", index)
		}
		for pointIndex := 0; pointIndex < got.PointCount(); pointIndex++ {
			gotPoint, gotPointOK := got.PointAt(pointIndex)
			wantPoint, wantPointOK := want.PointAt(pointIndex)
			if !gotPointOK || !wantPointOK || gotPoint != wantPoint {
				t.Fatalf("occurrence %d point %d drifted", index, pointIndex)
			}
		}
	}
	if snapshot.RulePlacementCount() != artifact.RulePlacementCount() {
		t.Fatal("rule placement count drifted from the sealed artifact")
	}
	for index := 0; index < snapshot.RulePlacementCount(); index++ {
		got, gotOK := snapshot.RulePlacementAt(index)
		want, wantOK := artifact.RulePlacementAt(index)
		if !gotOK || !wantOK {
			t.Fatalf("rule placement %d", index)
		}
		wantOutput, wantOutputOK := want.OutputSemanticID()
		gotOutput, gotOutputOK := got.OutputSemanticID()
		if got.OccurrenceID() != want.ID() || gotOutputOK != wantOutputOK || (wantOutputOK && gotOutput != wantOutput) ||
			got.SpanResult() != programartifact.SpanResultOccurrence(want.OccurrenceKind()) ||
			got.InputKind() != uint8(want.InputKind()) {
			t.Fatalf("rule placement %d lost output semantic", index)
		}
	}
	if snapshot.BodyTransportCount() == 0 {
		t.Fatal("fixture issued no body transports")
	}
	if snapshot.BodyCount() != artifact.BodyCount() {
		t.Fatal("body count drifted from the sealed artifact")
	}
	for index := 0; index < snapshot.BodyCount(); index++ {
		got, gotOK := snapshot.BodyAt(index)
		want, wantOK := artifact.BodyAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() || got.RootCount() != want.RootCount() {
			t.Fatalf("body %d lost roots", index)
		}
		for rootIndex := 0; rootIndex < got.RootCount(); rootIndex++ {
			gotRoot, gotRootOK := got.RootAt(rootIndex)
			wantRoot, wantRootOK := want.RootAt(rootIndex)
			if !gotRootOK || !wantRootOK || gotRoot.ID() != wantRoot.ID() || gotRoot.Family() != wantRoot.Family() {
				t.Fatalf("body %d root %d drifted", index, rootIndex)
			}
		}
	}
	body, bodyOK := snapshot.BodyTransportAt(0)
	if !bodyOK || !body.BodyID().Available() {
		t.Fatal("body transport")
	}
	if body.ExitCount() == 0 {
		t.Fatal("body transport issued no accepted exits")
	}
	points := make(map[identity.ContentID]struct{}, snapshot.PointCount())
	for index := 0; index < snapshot.PointCount(); index++ {
		point, ok := snapshot.PointAt(index)
		if !ok {
			t.Fatal("point column")
		}
		points[point.ID()] = struct{}{}
	}
	for index := 0; index < body.ExitCount(); index++ {
		exit, ok := body.ExitAt(index)
		if !ok || !exit.Available() {
			t.Fatalf("exit %d", index)
		}
		if _, known := points[exit]; !known {
			t.Fatalf("exit %d is not a sealed point", index)
		}
	}
	snapshotType := reflect.TypeOf(*snapshot)
	for index := 0; index < snapshotType.NumField(); index++ {
		if strings.Contains(snapshotType.Field(index).Type.String(), "programartifact") {
			t.Fatalf("Snapshot retained owner field %s", snapshotType.Field(index).Name)
		}
	}
	bodyType := reflect.TypeOf(body)
	for index := 0; index < bodyType.NumField(); index++ {
		if strings.Contains(bodyType.Field(index).Type.String(), "programartifact") {
			t.Fatalf("BodyTransport retained owner field %s", bodyType.Field(index).Name)
		}
	}
}

func TestLowerProjectsHeapIndexWriteGeometry(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "ingress-index.lua", Text: []byte(`
local t = {}
local k = 1
t[1] = 2
t[1.0] = 3
t[k] = 4
local a = t[1]
local b = t[1.0]
local c = t[k]
return t, a, b, c
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("ingress compilation failed: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := ingress.Lower(artifact, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil {
		t.Fatal("Lower refused a sealed artifact")
	}
	if snapshot.HeapIndexCount() != artifact.HeapIndexCount() || snapshot.HeapIndexCount() == 0 {
		t.Fatalf("HeapIndex count=%d artifact=%d", snapshot.HeapIndexCount(), artifact.HeapIndexCount())
	}
	var writePositions int
	for index := 0; index < snapshot.HeapIndexCount(); index++ {
		got, gotOK := snapshot.HeapIndexAt(index)
		want, wantOK := artifact.HeapIndexAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() || got.Read() != want.Read() || got.BaseSpan() != want.BaseSpan() || got.ResultSpan() != want.ResultSpan() || got.DynamicKeySpan() != want.DynamicKeySpan() || got.ValuesID() != want.ValuesID() {
			t.Fatalf("HeapIndex %d identity drifted from the sealed artifact", index)
		}
		gotExact, gotExactOK := got.ExactKey()
		wantExact, wantExactOK := want.ExactKey()
		if gotExactOK != wantExactOK || (wantExactOK && gotExact != uint64(wantExact)) {
			t.Fatalf("HeapIndex %d exact key drifted from the sealed artifact", index)
		}
		gotSpan, gotPosition, gotValuesOK := got.Values()
		wantSpan, wantPosition, wantValuesOK := want.Values()
		if gotValuesOK != wantValuesOK || gotSpan != wantSpan || gotPosition != wantPosition {
			t.Fatalf("HeapIndex %d values drifted snapshot=%v/%d/%t artifact=%v/%d/%t", index, gotSpan, gotPosition, gotValuesOK, wantSpan, wantPosition, wantValuesOK)
		}
		if !want.Read() {
			if !wantValuesOK || !wantSpan.Available() || !want.ValuesID().Available() {
				t.Fatalf("HeapIndex %d write lost Values geometry", index)
			}
			writePositions++
		}
	}
	if writePositions == 0 {
		t.Fatal("fixture issued no index writes")
	}
}

func TestLowerProjectsHeapFieldSameReadDiagonal(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "ingress-field.lua", Text: []byte(`local a = {}; local b = {}; local x = a; return { [x] = x }`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Global()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("ingress compilation failed: %s", failure.Error())
	}
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	snapshot, lowered := ingress.Lower(artifact, vocabulary)
	if !vocabularyOK || !lowered || snapshot == nil {
		t.Fatal("Lower refused a sealed artifact")
	}
	if snapshot.HeapAllocationCount() != artifact.HeapAllocationCount() {
		t.Fatalf("HeapAllocation count=%d artifact=%d", snapshot.HeapAllocationCount(), artifact.HeapAllocationCount())
	}
	var shared int
	for index := 0; index < snapshot.HeapAllocationCount(); index++ {
		got, gotOK := snapshot.HeapAllocationAt(index)
		want, wantOK := artifact.HeapAllocationAt(index)
		if !gotOK || !wantOK || got.ID() != want.ID() || got.FieldCount() != want.FieldCount() {
			t.Fatalf("HeapAllocation %d drifted from the sealed artifact", index)
		}
		for fieldIndex := 0; fieldIndex < got.FieldCount(); fieldIndex++ {
			gotField, gotFieldOK := got.FieldAt(fieldIndex)
			wantField, wantFieldOK := want.FieldAt(fieldIndex)
			if !gotFieldOK || !wantFieldOK || gotField.ID() != wantField.ID() || gotField.Kind() != uint8(wantField.Kind()) || gotField.ValuesID() != wantField.ValuesID() || gotField.SelectorSpan() != wantField.SelectorSpan() || gotField.SharesFirstValueCell() != wantField.SharesFirstValueCell() {
				t.Fatalf("HeapField %d/%d drifted from the sealed artifact share snapshot=%t artifact=%t", index, fieldIndex, gotField.SharesFirstValueCell(), wantField.SharesFirstValueCell())
			}
			gotSpan, gotWidth, gotOpen, gotValuesOK := gotField.Values()
			wantSpan, wantWidth, wantOpen, wantValuesOK := wantField.Values()
			if gotValuesOK != wantValuesOK || gotSpan != wantSpan || gotWidth != wantWidth || gotOpen != wantOpen {
				t.Fatalf("HeapField %d/%d Values drifted snapshot=%v/%d/%t/%t artifact=%v/%d/%t/%t", index, fieldIndex, gotSpan, gotWidth, gotValuesOK, gotOpen, wantSpan, wantWidth, wantValuesOK, wantOpen)
			}
			if wantField.SharesFirstValueCell() {
				shared++
			}
		}
	}
	if shared == 0 {
		t.Fatal("same-read fixture issued no SharesFirstValueCell field")
	}
}
