package front

import (
	"reflect"
	"testing"
)

func TestNativeWIRTopologyDraftsPublishMutualRecursiveStructure(t *testing.T) {
	compilation, err := Compile(`
type Section = { title: string, body: Block? }
type Block = { text: string, owner: Section }
local function root_title(s: Section): string
    local b = s.body
    if b == nil then return s.title end
    return b.owner.title
end
return root_title`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var mutual uint32
	for _, draft := range compilation.nativeTopology {
		if draft.Recursive != nil && draft.Recursive.CycleRecordNodes > 1 {
			mutual += draft.Recursive.CycleRecordNodes
		}
	}
	if mutual < 2 {
		t.Fatalf("mutual recursive topology nodes = %d, want at least 2: %#v", mutual, compilation.nativeTopology)
	}
}

func TestNativeWIRTopologyDraftsAreByteStableAcrossCompilations(t *testing.T) {
	source := `
type Push = { kind: "push", value: number }
type Pop = { kind: "pop" }
type Op = Push | Pop
type Node = { value: number, next: Node? }
local function f(op: Op, node: Node): string
    if op.kind == "push" then return "push" end
    return "pop"
end
return f`
	first, err := Compile(source)
	if err != nil {
		t.Fatalf("first compile: %v", err)
	}
	second, err := Compile(source)
	if err != nil {
		t.Fatalf("second compile: %v", err)
	}
	if !reflect.DeepEqual(first.nativeTopology, second.nativeTopology) {
		t.Fatalf("native topology differs across identical compilations:\nfirst=%#v\nsecond=%#v", first.nativeTopology, second.nativeTopology)
	}
}

// record_construction has one publisher. The resolved lowering owns it because
// only it carries the constructor's destination, which the escape boundary of
// the allocation is read from.
func TestNativeWIRTopologyDraftsSolelyPublishRecordConstructionUses(t *testing.T) {
	compilation, err := Compile(`
local function sink(value) end
local message = { id = "m1", attempts = 0 }
sink(message)
return message`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []*NativeRecordTopologyDraft
	for _, draft := range compilation.nativeTopology {
		if draft.Record != nil {
			records = append(records, draft.Record)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction drafts = %#v, want exactly one WIR publication", records)
	}
	if len(records[0].CallUses) == 0 {
		t.Fatalf("record construction call uses = %#v, want an escape boundary", records[0].CallUses)
	}
}

// A method receiver reaches its callee exactly as an argument does, so it is
// the same publication boundary.
func TestNativeWIRTopologyDraftsPublishReceiverUse(t *testing.T) {
	compilation, err := Compile(`
local message = { id = "m1", attempts = 0 }
message:touch()
return message.id`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []*NativeRecordTopologyDraft
	for _, draft := range compilation.nativeTopology {
		if draft.Record != nil {
			records = append(records, draft.Record)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction drafts = %#v, want exactly one WIR publication", records)
	}
	if len(records[0].CallUses) == 0 {
		t.Fatalf("record construction call uses = %#v, want receiver use", records[0].CallUses)
	}
}

// A constructor that never reaches a call boundary keeps its unrevoked grant:
// the escape row is a proof about the resolved argument topology, not a default.
func TestNativeWIRTopologyDraftsWithholdAbsentCallUse(t *testing.T) {
	compilation, err := Compile(`
local message = { id = "m1", attempts = 0 }
return message.id`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []*NativeRecordTopologyDraft
	for _, draft := range compilation.nativeTopology {
		if draft.Record != nil {
			records = append(records, draft.Record)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction drafts = %#v, want exactly one WIR publication", records)
	}
	if len(records[0].CallUses) != 0 {
		t.Fatalf("record construction call uses = %#v, want none", records[0].CallUses)
	}
}

// The entry storage classes are read from the resolved producers of the entry
// operands, so the one surviving publisher keeps the boolean tag and the
// promoting numeric carrier the deleted syntax walk used to publish.
func TestNativeWIRTopologyDraftsPublishEntryProducerShapes(t *testing.T) {
	compilation, err := Compile(`
local function build(i: integer)
    return { doubled = i * 2, ok = true }
end
return build`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []*NativeRecordTopologyDraft
	for _, draft := range compilation.nativeTopology {
		if draft.Record != nil {
			records = append(records, draft.Record)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction drafts = %#v, want exactly one", records)
	}
	var literal, multiply bool
	for _, entry := range records[0].Entries {
		literal = literal || entry.Value.Shape == NativeOperandLiteral
		multiply = multiply || entry.ProducerOp == NativeProducerMultiply
	}
	if records[0].EntrySlots != 2 || !literal || !multiply {
		t.Fatalf("record topology = %#v, want two entries with literal and multiply producers", records[0])
	}
}
