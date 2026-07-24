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
	for _, contract := range compilation.NativeContracts {
		if contract.Family == "recursive_type_identity" && strings.Contains(contract.Value, "mutual=true") {
			mutual++
		}
	}
	if mutual < 2 {
		t.Fatalf("mutual recursive contracts = %d, want at least 2: %#v", mutual, compilation.NativeContracts)
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
	if !reflect.DeepEqual(first.NativeContracts, second.NativeContracts) {
		t.Fatalf("native contracts differ across identical compilations:\nfirst=%#v\nsecond=%#v", first.NativeContracts, second.NativeContracts)
	}
}
