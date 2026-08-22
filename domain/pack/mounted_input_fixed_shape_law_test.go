package pack_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	packtransfer "github.com/wippyai/go-lua/domain/pack/transfer"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestMountedInputFixedZeroMemberClosedIsProvenNil pins the Lua
// under-application shape. The call authors no actual at the selected fixed
// formal position and has no actual tail that could reach it, so the formal
// holds nil. That is knowledge about the call, not a malformed artifact: the
// row is issued, carries no member, and is not open.
func TestMountedInputFixedZeroMemberClosedIsProvenNil(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchemaSource(t, contract, "fixed_under_applied", "local receiver = {}\nreceiver:send()\n")
	actual, actualOK := fixture.schema.MountedActualProjection(fixture.module, fixture.callID)
	if _, tail := actual.TailID(); !actualOK || actual.ActualCount() != 1 || tail {
		t.Fatalf("under-applied actual projection = count %d tail %t", actual.ActualCount(), tail)
	}
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !inputOK || !input.Valid() {
		t.Fatal("under-applied fixed formal refused a mounted input")
	}
	if input.Kind() != packtransfer.MountedInputFixed || input.MemberCount() != 0 || input.IsOpen() || !input.IsProvenNil() {
		t.Fatalf("kind=%v members=%d open=%t provenNil=%t, want fixed/0/false/true", input.Kind(), input.MemberCount(), input.IsOpen(), input.IsProvenNil())
	}
	if _, ok := input.SemanticID(); ok {
		t.Fatal("proven-nil fixed formal published a semantic member")
	}
	if _, ok := input.MemberAt(0); ok {
		t.Fatal("proven-nil fixed formal published a member row")
	}
}

// TestMountedInputFixedZeroMemberOpenIsUnknown pins the tail-fed shape. The
// call authors no fixed actual at the selected formal position, but its actual
// tail may populate it, so the value is statically unknown. The row is issued
// and open; it never fabricates the tail identity as a Value member.
func TestMountedInputFixedZeroMemberOpenIsUnknown(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchemaSource(t, contract, "fixed_tail_fed", "local receiver = {}\nlocal function outer(...)\n  receiver:send(...)\nend\nouter(1)\n")
	actual, actualOK := fixture.schema.MountedActualProjection(fixture.module, fixture.callID)
	tailID, tail := actual.TailID()
	if !actualOK || actual.ActualCount() != 1 || !tail || !tailID.Available() {
		t.Fatalf("tail-fed actual projection = count %d tail %t", actual.ActualCount(), tail)
	}
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !inputOK || !input.Valid() {
		t.Fatal("tail-fed fixed formal refused a mounted input")
	}
	if input.Kind() != packtransfer.MountedInputFixed || input.MemberCount() != 0 || !input.IsOpen() || input.IsProvenNil() {
		t.Fatalf("kind=%v members=%d open=%t provenNil=%t, want fixed/0/true/false", input.Kind(), input.MemberCount(), input.IsOpen(), input.IsProvenNil())
	}
	if _, ok := input.SemanticID(); ok {
		t.Fatal("unknown fixed formal published a semantic member")
	}
	for index := 0; index < input.MemberCount(); index++ {
		member, ok := input.MemberAt(index)
		if ok && member == tailID {
			t.Fatal("unknown fixed formal published the actual tail as a member")
		}
	}
}

// TestMountedInputFixedSingleMemberStaysClosed keeps the fully applied shape
// exactly as it was: one closed member, never open, never proven nil.
func TestMountedInputFixedSingleMemberStaysClosed(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchemaSource(t, contract, "fixed_applied", "local receiver = {}\nreceiver:send(1, 2)\n")
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !inputOK || !input.Valid() || input.MemberCount() != 1 || input.IsOpen() || input.IsProvenNil() {
		t.Fatalf("applied fixed formal = valid=%t members=%d open=%t provenNil=%t", input.Valid(), input.MemberCount(), input.IsOpen(), input.IsProvenNil())
	}
	semantic, semanticOK := input.SemanticID()
	if !semanticOK || semantic != fixture.argument0 {
		t.Fatal("applied fixed formal lost its exact semantic member")
	}
}

// TestMountedInputProvenNilIsNotAValueCoordinate keeps the two zero-member
// shapes out of Value's coordinate plane. A proven-nil formal has no mounted
// semantic source, so no coordinate may be manufactured for it; the aggregate
// summary read must not report a readable present fact either.
func TestMountedInputProvenNilIsNotAValueCoordinate(t *testing.T) {
	contract, operation := selectorLawContract(t)
	fixture := selectorLawSchemaSource(t, contract, "fixed_nil_coordinate", "local receiver = {}\nreceiver:send()\n")
	input, inputOK := packtransfer.NewMountedInput(fixture.schema, fixture.module, fixture.callID, operation, vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1})
	if !inputOK || !input.IsProvenNil() {
		t.Fatal("proven-nil mounted input")
	}
	if _, ok := packtransfer.CoordinateForInput(fixture.values, input); ok {
		t.Fatal("proven-nil formal acquired a Value coordinate")
	}
	if _, ok := packtransfer.CoordinateForInputMember(fixture.values, input, 0); ok {
		t.Fatal("proven-nil formal acquired a member coordinate")
	}
	coordinates, coordinatesOK := packtransfer.CoordinatesForInput(fixture.values, input)
	if !coordinatesOK || len(coordinates) != 0 {
		t.Fatalf("proven-nil coordinate vector = %d/%t, want 0/true", len(coordinates), coordinatesOK)
	}
	summary := valuedomain.BeginValueSummary(fixture.values)
	if _, present, _ := packtransfer.SummaryValuesAtInput(fixture.values, summary, input); present {
		t.Fatal("proven-nil formal reported a present summary fact")
	}
}
