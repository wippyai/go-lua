package engine

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	result, err := entryKernel(equation.BoundEquation{Operands: []equation.BoundOperand{{Role: equation.MustOperandRole("entry"), Value: entry}}}, equation.Partition{})
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

// TestChildEntryPublishesInstantiatedFormalType pins the entry lane the call
// site uses to instantiate a formal the callee leaves undeclared: the seed's
// type becomes that root's entry type, so every reader that consults the term's
// static type inside the body answers with the caller's argument type.
func TestChildEntryPublishesInstantiatedFormalType(t *testing.T) {
	stringTarget, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatal("encode string target")
	}
	entry, err := encodeChildEntryWithPlacementCapabilities(
		[]entrySeed{{Term: "path/sym4", Value: []byte("scalar/top"), Type: stringTarget}},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("encode child entry: %v", err)
	}
	result, err := entryKernel(equation.BoundEquation{Operands: []equation.BoundOperand{{Role: equation.MustOperandRole("entry"), Value: entry}}}, equation.Partition{})
	if err != nil {
		t.Fatalf("entry kernel: %v", err)
	}
	values := make(map[string][]byte, len(result.Closure.Values))
	for _, fact := range result.Closure.Values {
		values[fact.Key] = fact.Value
	}
	if got := string(values["type/path/sym4/entry"]); got != string(stringTarget) {
		t.Fatalf("instantiated entry type = %q, want %q", got, stringTarget)
	}
	if got := string(values["value/path/sym4/entry"]); got != "scalar/top" {
		t.Fatalf("seed value = %q, want the caller's seeded value unchanged", got)
	}
}

// TestChildEntryDeclarationOutranksInstantiatedType pins the boundary
// precedence: a formal the callee declares keeps its declaration as the sole
// type authority, so a caller-supplied instantiation never reaches it.
func TestChildEntryDeclarationOutranksInstantiatedType(t *testing.T) {
	stringTarget, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatal("encode string target")
	}
	numberTarget, ok := shapefact.EncodeTarget(typ.Number)
	if !ok {
		t.Fatal("encode number target")
	}
	entry, err := encodeChildEntryWithPlacementCapabilities(
		[]entrySeed{{Term: "path/sym4", Value: []byte("scalar/top"), Type: stringTarget}},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("encode child entry: %v", err)
	}
	result, err := entryKernel(equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: equation.MustOperandRole("entry"), Value: entry},
		{Role: equation.MustOperandRole("declared-root-00000000"), Value: []byte("path/sym4")},
		{Role: equation.MustOperandRole("declared-type-00000000"), Value: numberTarget},
	}}, equation.Partition{})
	if err != nil {
		t.Fatalf("entry kernel: %v", err)
	}
	for _, fact := range result.Closure.Values {
		if fact.Key == "type/path/sym4/entry" && string(fact.Value) != string(numberTarget) {
			t.Fatalf("declared root entry type = %q, want the declaration %q", fact.Value, numberTarget)
		}
	}
}

// TestChildEntryRejectsInstantiatedTypeBelowItsSchema pins the wire discipline:
// the per-seed type lane opens at version 7, and an older packet carrying one
// is malformed rather than reinterpreted.
func TestChildEntryRejectsInstantiatedTypeBelowItsSchema(t *testing.T) {
	stringTarget, ok := shapefact.EncodeTarget(typ.String)
	if !ok {
		t.Fatal("encode string target")
	}
	if _, err := encodeChildEntryWithCapabilities(
		[]entrySeed{{Term: "path/sym4", Value: []byte("scalar/top"), Type: stringTarget}},
		nil, nil, nil, nil,
	); err == nil {
		t.Fatal("version 4 packet accepted an instantiated seed type")
	}
}
