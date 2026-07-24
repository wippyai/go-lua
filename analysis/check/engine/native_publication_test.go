package engine

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// A failed evaluation must cross the public boundary as its single conservative
// diagnostic, never as a partly evaluated closure. In particular, a native
// consumer must not be able to observe facts which were produced before the
// operation that made the transaction incomplete.
func TestCheckPublishesNoPartialClosureOnConservativeFailure(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "mixed call targets",
			source: `local f = function() return 1 end; local g = unknown and f or provider; return g()`,
		},
		{
			name:   "incomplete entry",
			source: `local x = 0; local f = function(a) x = a end; f(provider())`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Check(test.source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].Key != "analysis/conservative" {
				t.Fatalf("diagnostics = %#v, want only the conservative diagnostic", result.Diagnostics)
			}
			assertNoClosurePublication(t, result)
		})
	}
}

func TestCheckNativePublicationIsDeterministicAndIncludesDiagnosticClosure(t *testing.T) {
	const source = `local value: string = 1`
	first, err := Check(source)
	if err != nil {
		t.Fatalf("first Check: %v", err)
	}
	second, err := Check(source)
	if err != nil {
		t.Fatalf("second Check: %v", err)
	}
	if first.Native == nil || second.Native == nil {
		t.Fatalf("checked results published Native indexes %#v and %#v, want both", first.Native, second.Native)
	}
	if !reflect.DeepEqual(first.Native.Facts(), second.Native.Facts()) {
		t.Fatalf("same-source Native publication differs:\nfirst:  %#v\nsecond: %#v", first.Native.Facts(), second.Native.Facts())
	}
	if !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("same-source diagnostic closure differs:\nfirst:  %#v\nsecond: %#v", first.Diagnostics, second.Diagnostics)
	}
	assertNativeLaneExactlyProjects(t, first.Native, NativeLaneDiagnostics, first.Diagnostics)
}

// ValueFacts and Outcomes are the complete value and outcome partitions at the
// public cut. For a non-numeric, non-contract program (which has no additional
// WIR-derived native rows), the index must be an exact, one-to-one projection:
// every closed row appears once, with neither an omission nor an invention.
func TestCheckNativeFactIndexIsCompleteForClosedValueAndOutcomePartitions(t *testing.T) {
	result, err := Check(`local greeting = "hello"; return greeting`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Native == nil {
		t.Fatal("checked result published no Native fact index")
	}
	assertNativeLaneExactlyProjects(t, result.Native, NativeLaneValues, result.ValueFacts)
	assertNativeLaneExactlyProjects(t, result.Native, NativeLaneOutcomes, result.Outcomes)
}

// A non-nil assertion is a source claim, not a diagnostic nor a proof. The
// quiet diagnostic lane and claimed value row are deliberately independent
// publication lanes for native consumers.
func TestCheckNonNilAssertionKeepsDiagnosticsQuietAndPublishesClaimedFact(t *testing.T) {
	result, err := Check(`
local optional: string? = "found"
local asserted = optional!
`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("non-nil assertion diagnostics = %#v, want none", result.Diagnostics)
	}
	if result.Native == nil {
		t.Fatal("checked result published no Native fact index")
	}
	for _, fact := range result.Native.Facts() {
		if fact.Lane == NativeLaneValues && fact.Family == "value" && fact.Subject == "asserted" && fact.Trust == NativeTrustClaimed {
			return
		}
	}
	t.Fatalf("Native facts = %#v, want a claimed value row for asserted", result.Native.Facts())
}

func assertNoClosurePublication(t *testing.T, result Result) {
	t.Helper()
	if result.Native != nil || len(result.Artifact.Equations) != 0 ||
		len(result.Values) != 0 || len(result.Outcomes) != 0 || len(result.ReturnCandidates) != 0 || len(result.ValueFacts) != 0 ||
		len(result.PublishedDiagnostics) != 0 || len(result.PolicyDiagnostics) != 0 || len(result.DiagnosticSpans) != 0 ||
		result.Placement != nil || len(result.TypeDefinitions) != 0 || result.Transactions != 0 {
		t.Fatalf("conservative failure leaked a publication: %#v", result)
	}
}

func assertNativeLaneExactlyProjects(t *testing.T, index *NativeFactIndex, lane string, closure []equation.Fact) {
	t.Helper()
	want := make(map[string]int, len(closure))
	for _, fact := range closure {
		want[nativePublicationKey(fact.Key, nativeFactValue(fact.Value))]++
	}
	got := make(map[string]int)
	for _, fact := range index.Facts() {
		if fact.Lane == lane {
			got[nativePublicationKey(fact.Key, fact.Value)]++
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s lane = %#v, want exact closure projection %#v", lane, got, want)
	}
}

func nativePublicationKey(key, value string) string { return key + "\x00" + value }
