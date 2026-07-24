package engine

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func nativeTestArtifact() equation.Artifact {
	var body equation.BodyID
	body[0] = 1
	entry := equation.EntryParameter{Body: body, Name: "entry"}
	write := equation.Equation{
		Target: equation.Coordinate{Body: body, Name: "op-00000003"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: "environment-write", ContractID: equation.ContentID{1}}, KernelID: "test/write",
		Operands: []equation.Operand{
			{Role: "target", Term: equation.ClosedTerm([]byte("path/sym2"))},
			{Role: "display", Term: equation.ClosedTerm([]byte("total"))},
			{Role: "value", Term: equation.ClosedTerm([]byte("scalar/number/7"))},
		},
	}
	apply := equation.Equation{
		Target: equation.Coordinate{Body: body, Name: "op-00000004"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: "apply", ContractID: equation.ContentID{2}}, KernelID: "test/apply",
		Operands: []equation.Operand{
			{Role: "callee", Term: equation.ClosedTerm([]byte("path/sym5"))},
			{Role: "callee-display", Term: equation.ClosedTerm([]byte("scale"))},
			{Role: "argument-00000000", Term: equation.ClosedTerm([]byte("path/sym2"))},
			{Role: "argument-display-00000000", Term: equation.ClosedTerm([]byte("total"))},
		},
	}
	hidden := equation.Equation{
		Target: equation.Coordinate{Body: body, Name: "op-00000005"}, Entry: entry,
		Occurrence: equation.Occurrence{Kind: "environment-write", ContractID: equation.ContentID{3}}, KernelID: "test/write",
		Operands: []equation.Operand{
			{Role: "target", Term: equation.ClosedTerm([]byte("path/sym9"))},
			{Role: "display", Term: equation.ClosedTerm([]byte("front/hidden/allocation/op-00000005"))},
			{Role: "value", Term: equation.ClosedTerm([]byte("scalar/number/1"))},
		},
	}
	return equation.Artifact{Equations: []equation.Equation{write, apply, hidden}}
}

func TestNativeFactIndexAnchorsTermsAndOccurrencesFromTheArtifact(t *testing.T) {
	index := publishedNativeFacts(nativeTestArtifact(),
		[]equation.Fact{
			{Key: "value/path/sym2/op-00000003", Value: []byte("scalar/number/7")},
			{Key: "value/path/sym5/op-00000004", Value: []byte("scalar/function/x")},
			{Key: "value/path/sym9/op-00000005", Value: []byte("scalar/number/1")},
			{Key: "heap/table-closed/aGVhcA/op-00000003", Value: []byte("closed")},
		},
		[]equation.Fact{{Key: "return/arity", Value: []byte("1")}},
		[]equation.Fact{{Key: "claim/unproven/op-00000004", Value: []byte("no witness")}},
	)
	byKey := make(map[string]NativeFact)
	for _, fact := range index.Facts() {
		byKey[fact.Key] = fact
	}
	for _, want := range []NativeFact{
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym2/op-00000003", Value: "scalar/number/7", Term: "path/sym2", Subject: "total", Occurrence: "op-00000003"},
		// A callee spelling is recovered from the paired display operand role.
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym5/op-00000004", Value: "scalar/function/x", Term: "path/sym5", Subject: "scale", Occurrence: "op-00000004"},
		// A hidden front display is not a source name, so the term stays unnamed.
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym9/op-00000005", Value: "scalar/number/1", Term: "path/sym9", Subject: "", Occurrence: "op-00000005"},
		// An identity-addressed key still anchors on its equation coordinate.
		{Lane: NativeLaneValues, Family: "heap", Key: "heap/table-closed/aGVhcA/op-00000003", Value: "closed", Term: "", Subject: "", Occurrence: "op-00000003"},
		{Lane: NativeLaneOutcomes, Family: "return", Key: "return/arity", Value: "1"},
		{Lane: NativeLaneDiagnostics, Family: "claim", Key: "claim/unproven/op-00000004", Value: "no witness", Occurrence: "op-00000004"},
	} {
		if got := byKey[want.Key]; got != want {
			t.Fatalf("row %s = %#v, want %#v", want.Key, got, want)
		}
	}
	if len(index.Facts()) != 6 {
		t.Fatalf("published %d rows, want 6", len(index.Facts()))
	}
}

func TestNativeFactIndexOrdersRowsDeterministically(t *testing.T) {
	values := []equation.Fact{
		{Key: "value/path/sym2/op-00000003", Value: []byte("b")},
		{Key: "heap/table-closed/aGVhcA/op-00000003", Value: []byte("closed")},
		{Key: "value/path/sym2/op-00000003", Value: []byte("a")},
	}
	first := publishedNativeFacts(nativeTestArtifact(), values, nil, nil).Facts()
	reversed := []equation.Fact{values[2], values[0], values[1]}
	second := publishedNativeFacts(nativeTestArtifact(), reversed, nil, nil).Facts()
	if len(first) != len(second) {
		t.Fatalf("row counts %d and %d differ", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("row %d = %#v and %#v differ under a permuted input", index, first[index], second[index])
		}
	}
	want := []string{"heap/table-closed/aGVhcA/op-00000003", "value/path/sym2/op-00000003", "value/path/sym2/op-00000003"}
	for index, key := range want {
		if first[index].Key != key {
			t.Fatalf("row %d key = %q, want %q", index, first[index].Key, key)
		}
	}
	if first[1].Value != "a" || first[2].Value != "b" {
		t.Fatalf("equal keys are ordered %q then %q, want \"a\" then \"b\"", first[1].Value, first[2].Value)
	}
}

func TestNativeFactIndexRendersNonTextValuesLosslessly(t *testing.T) {
	raw := []byte{0xff, 0xfe, 'a'}
	facts := publishedNativeFacts(nativeTestArtifact(), []equation.Fact{{Key: "placement/binding/aGVhcA/op-00000003", Value: raw}}, nil, nil).Facts()
	if len(facts) != 1 {
		t.Fatalf("published %d rows, want 1", len(facts))
	}
	if !strings.HasPrefix(facts[0].Value, NativeValuePrefixBase64) {
		t.Fatalf("value = %q, want a %s rendering", facts[0].Value, NativeValuePrefixBase64)
	}
	if facts[0].Value != NativeValuePrefixBase64+"__5h" {
		t.Fatalf("value = %q, want the canonical encoding of the published bytes", facts[0].Value)
	}
}

// A body rejected before publication has no index at all. Reading it must not
// look like a publication that closed nothing.
func TestNativeFactIndexIsAbsentForARejectedBody(t *testing.T) {
	result := diagnosticResult("lint.analysis.conservative", errNativeTestRejection{})
	if result.Native != nil {
		t.Fatalf("rejected body published %#v, want no fact index", result.Native)
	}
	var absent *NativeFactIndex
	if facts := absent.Facts(); facts != nil {
		t.Fatalf("absent index published %#v, want nothing", facts)
	}
}

type errNativeTestRejection struct{}

func (errNativeTestRejection) Error() string { return "rejected" }

func TestCheckPublishesNativeFactsForACheckedModule(t *testing.T) {
	result, err := Check("local config = { retries = 3 }\nlocal function scale(factor: integer): integer\nreturn factor * 2\nend\nreturn scale(config.retries)\n")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Native == nil {
		t.Fatal("checked module published no fact index")
	}
	var closed, constant bool
	for _, fact := range result.Native.Facts() {
		closed = closed || strings.HasPrefix(fact.Key, "heap/table-closed/") && fact.Value == "closed"
		constant = constant || fact.Lane == NativeLaneValues && fact.Subject == "config.retries" && fact.Value == "scalar/number/3"
	}
	if !closed || !constant {
		t.Fatalf("closed-table fact %v, constant member fact %v; want both published", closed, constant)
	}
}
