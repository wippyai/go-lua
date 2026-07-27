package front

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeWIRContractsPublishMutualRecursiveFixpoint(t *testing.T) {
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
	var mutual int
	for _, contract := range compilation.nativeContracts {
		if contract.Family == "recursive_type_identity" && strings.Contains(contract.Value, "mutual=true") {
			mutual++
		}
	}
	if mutual < 2 {
		t.Fatalf("mutual recursive contracts = %d, want at least 2: %#v", mutual, compilation.nativeContracts)
	}
}

func TestNativeWIRContractsAreByteStableAcrossCompilations(t *testing.T) {
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
	if !reflect.DeepEqual(first.nativeContracts, second.nativeContracts) {
		t.Fatalf("native contracts differ across identical compilations:\nfirst=%#v\nsecond=%#v", first.nativeContracts, second.nativeContracts)
	}
}

// record_construction has one publisher. The resolved lowering owns it because
// only it carries the constructor's destination, which the escape boundary of
// the allocation is read from.
func TestNativeWIRContractsSolelyPublishEscapingRecordConstruction(t *testing.T) {
	compilation, err := Compile(`
local function sink(value) end
local message = { id = "m1", attempts = 0 }
sink(message)
return message`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []NativeContract
	for _, contract := range compilation.nativeContracts {
		if contract.Family == "record_construction" {
			records = append(records, contract)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction contracts = %#v, want exactly one WIR publication", records)
	}
	if !reflect.DeepEqual(records[0].Revocations, []string{"escape"}) {
		t.Fatalf("record construction revocations = %#v, want escape", records[0].Revocations)
	}
}

// A method receiver reaches its callee exactly as an argument does, so it is
// the same publication boundary.
func TestNativeWIRContractsPublishEscapeForRecordConstructionReceiver(t *testing.T) {
	compilation, err := Compile(`
local message = { id = "m1", attempts = 0 }
message:touch()
return message.id`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []NativeContract
	for _, contract := range compilation.nativeContracts {
		if contract.Family == "record_construction" {
			records = append(records, contract)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction contracts = %#v, want exactly one WIR publication", records)
	}
	if !reflect.DeepEqual(records[0].Revocations, []string{"escape"}) {
		t.Fatalf("record construction revocations = %#v, want escape", records[0].Revocations)
	}
}

// A constructor that never reaches a call boundary keeps its unrevoked grant:
// the escape row is a proof about the resolved argument topology, not a default.
func TestNativeWIRContractsWithholdEscapeForLocalRecordConstruction(t *testing.T) {
	compilation, err := Compile(`
local message = { id = "m1", attempts = 0 }
return message.id`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var records []NativeContract
	for _, contract := range compilation.nativeContracts {
		if contract.Family == "record_construction" {
			records = append(records, contract)
		}
	}
	if len(records) != 1 {
		t.Fatalf("record construction contracts = %#v, want exactly one WIR publication", records)
	}
	if len(records[0].Revocations) != 0 {
		t.Fatalf("record construction revocations = %#v, want none", records[0].Revocations)
	}
}

// The entry storage classes are read from the resolved producers of the entry
// operands, so the one surviving publisher keeps the boolean tag and the
// promoting numeric carrier the deleted syntax walk used to publish.
func TestNativeWIRContractsPublishEntryStorageClassesFromResolvedProducers(t *testing.T) {
	compilation, err := Compile(`
local function build(i: integer)
    return { doubled = i * 2, ok = true }
end
return build`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var values []string
	for _, contract := range compilation.nativeContracts {
		if contract.Family == "record_construction" {
			values = append(values, contract.Value)
		}
	}
	if len(values) != 1 {
		t.Fatalf("record construction contracts = %#v, want exactly one WIR publication", values)
	}
	for _, want := range []string{"entries=2", "boolean_storage=canonical_tag", "field_carrier=numeric_union", "overflow=promote_integer_to_number"} {
		if !strings.Contains(values[0], want) {
			t.Fatalf("record construction value = %q, want it to carry %q", values[0], want)
		}
	}
}
