package engine

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestChildEntryTransportsPublishedDescendantAndMemberCapability(t *testing.T) {
	handle := closureHandle{Prototype: "chunk.member", Captures: []string{"path/root"}}
	wire := memberClosureWire{Suffix: ".get", Handle: handle}
	entry, err := encodeChildEntryWithCapabilities(
		[]entrySeed{
			{Term: "path/root", Value: []byte("scalar/top")},
			{Term: "path/root.dep", Value: []byte("shape/table/v1/published")},
		},
		nil,
		[]entryMemberClosureSeed{{Term: "path/root.dep", Wire: wire}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("encode child entry: %v", err)
	}
	result, err := entryKernel(equation.BoundEquation{Operands: []equation.BoundOperand{{Role: "entry", Value: entry}}}, equation.Partition{})
	if err != nil {
		t.Fatalf("entry kernel: %v", err)
	}
	values := make(map[string][]byte, len(result.Closure.Values))
	for _, fact := range result.Closure.Values {
		values[fact.Key] = fact.Value
	}
	if got := string(values["value/path/root.dep/entry"]); got != "shape/table/v1/published" {
		t.Fatalf("descendant entry value = %q, want published value", got)
	}
	member, ok := values["member-closure/path/root.dep/entry/00000000"]
	if !ok || !strings.Contains(string(member), `"prototype":"chunk.member"`) {
		t.Fatalf("member capability = %q, want sealed handle", member)
	}
}

func TestChildEntryDescendantsChooseLatestExactFact(t *testing.T) {
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/path/root.dep/op-00000001", Value: []byte("scalar/old")},
		{Key: "value/path/root.dep/op-00000002", Value: []byte("scalar/new")},
		{Key: "value/path/rootish.dep/op-00000002", Value: []byte("scalar/foreign")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	seeds := childEntryDescendantSeeds([]entrySeed{{Term: "path/root", Value: []byte("scalar/top")}}, partition)
	if len(seeds) != 1 || seeds[0].Term != "path/root.dep" || string(seeds[0].Value) != "scalar/new" {
		t.Fatalf("descendant seeds = %#v, want latest exact descendant", seeds)
	}
}
