package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestCallRowsExposeOnlyAvailableChildRanges(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "call-rows.lua",
		Text: []byte(`
local function identity(value) return value end
return identity(1)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewExecutionSchemaID(identity.ContentID{3}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("call fixture did not compile: %s", failure.Error())
	}
	program := artifact.Program()
	catalog, coldPublished := programcatalog.CatalogID(program.SchemaID)
	if !program.Available() || !coldPublished || !catalog.Available() {
		t.Fatal("call rows cold program")
	}
	callCount, callsOK := program.CallCount()
	bodyCount, bodiesOK := program.BodyCount()
	if !callsOK || callCount == 0 || !bodiesOK {
		t.Fatal("call fixture published no call family")
	}
	publishedDirect := false
	for callIndex := 0; callIndex < callCount; callIndex++ {
		row, rowOK := program.CallAt(callIndex)
		if !rowOK {
			t.Fatalf("CallAt(%d) unavailable", callIndex)
		}
		if target, targetOK := row.DirectTargetBody(); targetOK {
			found := false
			for bodyIndex := 0; bodyIndex < bodyCount; bodyIndex++ {
				body, bodyOK := program.BodyAt(bodyIndex)
				if bodyOK && body.Callable() && body.ID() == target {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("CallAt(%d) target %x is not a callable body", callIndex, target[:4])
			}
			publishedDirect = true
		}
	}
	if !publishedDirect {
		t.Fatal("direct identity(1) call published no target body")
	}
	for callIndex := 0; callIndex < callCount; callIndex++ {
		row, rowOK := program.CallAt(callIndex)
		if !rowOK {
			t.Fatalf("CallAt(%d) unavailable", callIndex)
		}
		for childIndex := 0; childIndex < row.OperandCount(); childIndex++ {
			child, childOK := program.CallOperandFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallOperandFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		for childIndex := 0; childIndex < row.ArgumentCount(); childIndex++ {
			child, childOK := program.CallArgumentFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallArgumentFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		for childIndex := 0; childIndex < row.TypeArgumentCount(); childIndex++ {
			child, childOK := program.CallTypeArgumentFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallTypeArgumentFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		if _, childOK := program.CallOperandFor(callIndex, row.OperandCount()); childOK {
			t.Fatal("CallOperandFor exposed a child beyond the sealed range")
		}
		if _, childOK := program.CallArgumentFor(callIndex, row.ArgumentCount()); childOK {
			t.Fatal("CallArgumentFor exposed a child beyond the sealed range")
		}
		if _, childOK := program.CallTypeArgumentFor(callIndex, row.TypeArgumentCount()); childOK {
			t.Fatal("CallTypeArgumentFor exposed a child beyond the sealed range")
		}
	}
}

func TestBoundedCallTailPublishesConsumerBackedOrdinalSlot(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "bounded-call-result-slot.lua",
		Text: []byte(`
local function identity(value) return value end
local result = identity(1)
return result
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewExecutionSchemaID(identity.ContentID{7}, identity.ContentID{6}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("bounded result fixture did not compile: %s", failure.Error())
	}
	program := artifact.Program()
	resultCount, resultsOK := program.CallResultCount()
	slotCount, slotsOK := program.CallResultSlotCount()
	if !resultsOK || resultCount != 1 || !slotsOK || slotCount != 1 {
		t.Fatalf("result/slot counts = %d/%d (%v/%v), want 1/1", resultCount, slotCount, resultsOK, slotsOK)
	}
	result, resultOK := program.CallResultAt(0)
	offset, width, spanOK := result.SlotSpan()
	tail, tailOK := result.ValuesTailID()
	if !resultOK || !spanOK || offset != 0 || width != 1 || !tailOK || result.Form() != programschema.CallResultValues {
		t.Fatal("bounded tail parent did not publish its exact child span")
	}
	slot, slotOK := program.CallResultSlotForCallOrdinal(result.CallID(), 0)
	value, valueOK := slot.ValueID()
	position, positionOK := slot.ConsumerPosition()
	if !slotOK || slot.SourceKind() != programschema.CallResultSlotSourceValuesTail ||
		slot.ConsumerKind() != programschema.CallResultSlotConsumerCell || !valueOK || value == tail ||
		!positionOK || position != 0 {
		t.Fatal("bounded tail slot is not an ordinal, consumer-backed Cell coordinate")
	}
}

func TestScalarExpressionCallPublishesFixedOrdinalSlot(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "scalar-call-result-slot.lua",
		Text: []byte(`
local function classify(value) return "number" end
local expected = "number"
if classify(5) == expected then
	return true
end
return false
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewExecutionSchemaID(identity.ContentID{9}, identity.ContentID{8}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("scalar result fixture did not compile: %s", failure.Error())
	}
	program := artifact.Program()
	resultCount, resultsOK := program.CallResultCount()
	slotCount, slotsOK := program.CallResultSlotCount()
	if !resultsOK || resultCount != 1 || !slotsOK || slotCount != 1 {
		t.Fatalf("result/slot counts = %d/%d (%v/%v), want 1/1", resultCount, slotCount, resultsOK, slotsOK)
	}
	result, resultOK := program.CallResultAt(0)
	slot, slotOK := program.CallResultSlotForCallOrdinal(result.CallID(), 0)
	call, callOK := program.CallForID(result.CallID())
	value, valueOK := slot.ValueID()
	resultValue, resultValueOK := result.ValueID()
	position, positionOK := slot.ConsumerPosition()
	if !resultOK || result.Form() != programschema.CallResultDirectValue || !slotOK || !callOK ||
		slot.SourceKind() != programschema.CallResultSlotSourceCallValue ||
		slot.ConsumerKind() != programschema.CallResultSlotConsumerStructural ||
		!valueOK || !resultValueOK || value != resultValue || value != call.SpanID() || !positionOK || position != 0 {
		t.Fatal("scalar expression call did not publish its direct evaluation-span result slot")
	}
}

func TestRightScalarExpressionCallRetainsConsumerPosition(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "right-scalar-call-result-slot.lua",
		Text: []byte(`
local function classify(value) return "number" end
local expected = "number"
return expected == classify(5)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewExecutionSchemaID(identity.ContentID{11}, identity.ContentID{10}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("right scalar result fixture did not compile: %s", failure.Error())
	}
	program := artifact.Program()
	resultCount, resultsOK := program.CallResultCount()
	slotCount, slotsOK := program.CallResultSlotCount()
	if !resultsOK || resultCount != 1 || !slotsOK || slotCount != 1 {
		t.Fatalf("right scalar result/slot counts = %d/%d, want 1/1", resultCount, slotCount)
	}
	result, _ := program.CallResultAt(0)
	slot, slotOK := program.CallResultSlotForCallOrdinal(result.CallID(), 0)
	position, positionOK := slot.ConsumerPosition()
	if !slotOK || !positionOK || position != 1 {
		t.Fatalf("right scalar Call consumer position = %d/%v, want 1/true", position, positionOK)
	}
}

func TestDiscardedStatementCallPublishesNoResultSlot(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "discarded-call-result-slot.lua",
		Text: []byte(`
local function consume(value) return value end
consume(1)
return true
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewExecutionSchemaID(identity.ContentID{13}, identity.ContentID{12}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := artifactcompiler.CompileDetailed(published, grammar, testfixture.EmptyProgramIssuancePlan(t))
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("discarded result fixture did not compile: %s", failure.Error())
	}
	program := artifact.Program()
	resultCount, resultsOK := program.CallResultCount()
	slotCount, slotsOK := program.CallResultSlotCount()
	if !resultsOK || resultCount != 0 || !slotsOK || slotCount != 0 {
		t.Fatalf("discarded result/slot counts = %d/%d (%v/%v), want 0/0", resultCount, slotCount, resultsOK, slotsOK)
	}
}
