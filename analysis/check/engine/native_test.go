package engine

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestNativeContractBridgeProjectsOnlyPublishedSubstrate(t *testing.T) {
	root := "sealed-table/test/op-00000003"
	child := "sealed-table/test/op-00000004"
	rootID := base64.RawURLEncoding.EncodeToString([]byte(root))
	childID := base64.RawURLEncoding.EncodeToString([]byte(child))
	rows := projectNativeContracts([]NativeFact{
		{Lane: NativeLaneValues, Key: "heap/table-identity/path/sym2/op-00000003", Value: root, Term: "path/sym2", Subject: "config", Occurrence: "op-00000003"},
		{Lane: NativeLaneValues, Key: "heap/table-identity/path/sym2.child/op-00000004", Value: child, Term: "path/sym2.child", Occurrence: "op-00000004"},
		{Lane: NativeLaneValues, Key: "heap/table-closed/" + rootID + "/op-00000003", Value: "closed", Occurrence: "op-00000003"},
		{Lane: NativeLaneValues, Key: "heap/table-closed/" + childID + "/op-00000004", Value: "closed", Occurrence: "op-00000004"},
		{Lane: NativeLaneValues, Key: "heap/member/" + rootID + "/LmNoaWxk/op-00000003", Value: "shape/table/v1/example", Occurrence: "op-00000003"},
		{Lane: NativeLaneValues, Key: "heap/member-identity/" + rootID + "/LmNoaWxk/op-00000003", Value: child, Occurrence: "op-00000003"},
		{Lane: NativeLaneValues, Key: "effect.freeze/sym2/op-00000005", Value: "unconditional", Occurrence: "op-00000005"},
		{Lane: NativeLaneValues, Key: "heap/index-presence/" + rootID + "/cGF0aC9zeW0z/op-00000006", Value: "proven", Occurrence: "op-00000006"},
		{Lane: NativeLaneValues, Key: "branch-proof/1/op-00000007/true", Value: "proven", Occurrence: "op-00000007"},
		// The raw close fact remains insufficient after a genuinely new member.
		{Lane: NativeLaneValues, Key: "heap/table-identity/path/sym9/op-00000008", Value: "sealed-table/test/op-00000008", Term: "path/sym9", Subject: "changed", Occurrence: "op-00000008"},
		{Lane: NativeLaneValues, Key: "heap/table-closed/c2VhbGVkLXRhYmxlL3Rlc3Qvb3AtMDAwMDAwMDg/op-00000008", Value: "closed", Occurrence: "op-00000008"},
		{Lane: NativeLaneValues, Key: "heap/member/c2VhbGVkLXRhYmxlL3Rlc3Qvb3AtMDAwMDAwMDg/Lm9yaWdpbmFs/op-00000008", Value: "scalar/number/1", Occurrence: "op-00000008"},
		{Lane: NativeLaneValues, Key: "heap/member/c2VhbGVkLXRhYmxlL3Rlc3Qvb3AtMDAwMDAwMDg/LmFkZGVk/op-00000009", Value: "scalar/number/2", Occurrence: "op-00000009"},
	})

	var deep, element, branch bool
	for _, row := range rows {
		if row.Family == "sealed_table" && row.Subject == "changed" {
			t.Fatalf("new-member table projected as sealed: %#v", row)
		}
		switch {
		case row.Family == "sealed_table" && strings.Contains(row.Value, "depth=deep"):
			deep = true
		case row.Family == "table_element" && row.Value == "presence=proven result_nilability=non_nil":
			element = true
		case row.Family == "branch_partition" && row.Value == "partition=always_taken dead_arm=else dead_arm_reachable=false":
			branch = true
		}
	}
	if !deep || !element || !branch {
		t.Fatalf("deep=%v element=%v branch=%v, want every directly witnessed contract row", deep, element, branch)
	}
}

func TestCheckPublishesNativeEntryContractsDeterministically(t *testing.T) {
	source := `
local function scale(x: number): number
    return x * 2
end
local function fib(n: number): number
    if n < 2 then return n end
    return fib(n - 1) + fib(n - 2)
end
return scale(fib(4))`
	first, err := Check(source)
	if err != nil {
		t.Fatalf("first check: %v", err)
	}
	second, err := Check(source)
	if err != nil {
		t.Fatalf("second check: %v", err)
	}
	if first.Native == nil || second.Native == nil {
		t.Fatal("checked module did not publish native facts")
	}
	if !reflect.DeepEqual(first.Native.Facts(), second.Native.Facts()) {
		t.Fatalf("native contract publication differs across identical runs:\nfirst=%#v\nsecond=%#v", first.Native.Facts(), second.Native.Facts())
	}
	var callee, entry, scc bool
	for _, fact := range first.Native.Facts() {
		switch fact.Family {
		case "callee_set":
			callee = true
		case "function_entry":
			entry = true
		case "call_scc":
			scc = true
		}
	}
	if !callee || !entry || !scc {
		t.Fatalf("missing native entry-contract families: callee=%t entry=%t scc=%t", callee, entry, scc)
	}
}

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
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym2/op-00000003", Value: "scalar/number/7", Term: "path/sym2", Subject: "total", Occurrence: "op-00000003", Trust: NativeTrustProven},
		// A callee spelling is recovered from the paired display operand role.
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym5/op-00000004", Value: "scalar/function/x", Term: "path/sym5", Subject: "scale", Occurrence: "op-00000004", Trust: NativeTrustProven},
		// A hidden front display is not a source name, so the term stays unnamed.
		{Lane: NativeLaneValues, Family: "value", Key: "value/path/sym9/op-00000005", Value: "scalar/number/1", Term: "path/sym9", Subject: "", Occurrence: "op-00000005", Trust: NativeTrustProven},
		// An identity-addressed key still anchors on its equation coordinate.
		{Lane: NativeLaneValues, Family: "heap", Key: "heap/table-closed/aGVhcA/op-00000003", Value: "closed", Term: "", Subject: "", Occurrence: "op-00000003", Trust: NativeTrustProven},
		// Outcome and diagnostic rows carry no value encoding, so they carry no
		// proof provenance either.
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

// The epoch chain a term already carries is its validity interval: a fact
// established at one epoch stops holding at the next, and the operation that
// bumps the epoch names the event.
func TestNativeFactIndexBindsEachRowToItsPublishedEpochInterval(t *testing.T) {
	index := publishedNativeFacts(nativeTestArtifact(),
		[]equation.Fact{
			{Key: "epoch/path/sym2/op-00000003", Value: []byte("op-00000003")},
			{Key: "epoch/path/sym2/op-00000005", Value: []byte("op-00000005")},
			{Key: "value/path/sym2/op-00000003", Value: []byte("scalar/number/7")},
			{Key: "value/path/sym2/op-00000005", Value: []byte("scalar/number/8")},
			// A term the closure never versioned carries no interval at all.
			{Key: "value/path/sym5/op-00000004", Value: []byte("scalar/function/x")},
			// A key anchored somewhere other than one of the term's epochs is
			// not epoch-gated and must not borrow an interval from one.
			{Key: "value/path/sym2/op-00000004", Value: []byte("scalar/number/7")},
		}, nil, nil)
	byKey := make(map[string]NativeFact)
	for _, fact := range index.Facts() {
		byKey[fact.Key] = fact
	}
	for _, want := range []struct{ key, established, revoked, event string }{
		{"value/path/sym2/op-00000003", "op-00000003", "op-00000005", "environment-write"},
		{"epoch/path/sym2/op-00000003", "op-00000003", "op-00000005", "environment-write"},
		{"value/path/sym2/op-00000005", "op-00000005", "", ""},
		{"value/path/sym5/op-00000004", "", "", ""},
		{"value/path/sym2/op-00000004", "", "", ""},
	} {
		got := byKey[want.key]
		if got.Established != want.established || got.Revoked != want.revoked || got.Event != want.event {
			t.Fatalf("row %s validity = (%q, %q, %q), want (%q, %q, %q)",
				want.key, got.Established, got.Revoked, got.Event, want.established, want.revoked, want.event)
		}
	}
}

// A claim the closure could not discharge stays a claim wherever its value
// travels, because the refusal is carried inside the published value.
func TestNativeFactIndexClassifiesProofProvenance(t *testing.T) {
	claimed := "scalar/claim/claim-kind/1/claim-type/\"number\""
	index := publishedNativeFacts(nativeTestArtifact(),
		[]equation.Fact{
			{Key: "value/path/sym2/op-00000003", Value: []byte("scalar/number/7")},
			{Key: "value/path/sym5/op-00000004", Value: []byte(claimed)},
			{Key: "value/path/sym9/op-00000005", Value: []byte("scalar/top")},
			{Key: "value/temp/1/op-00000004", Value: []byte("scalar/external-callback-any")},
		},
		[]equation.Fact{{Key: "return/0", Value: []byte(claimed)}},
		[]equation.Fact{{Key: "claim/unproven/op-00000004", Value: []byte("no witness")}},
	)
	byKey := make(map[string]NativeFact)
	for _, fact := range index.Facts() {
		byKey[fact.Key] = fact
	}
	for key, want := range map[string]string{
		"value/path/sym2/op-00000003": NativeTrustProven,
		"value/path/sym5/op-00000004": NativeTrustClaimed,
		"value/path/sym9/op-00000005": NativeTrustUnknown,
		"value/temp/1/op-00000004":    NativeTrustUnknown,
		"return/0":                    "",
		"claim/unproven/op-00000004":  "",
	} {
		if got := byKey[key].Trust; got != want {
			t.Fatalf("row %s trust = %q, want %q", key, got, want)
		}
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

// The two halves a speculative consumer needs, on a real checked module: a
// binding's first value is revoked at the write that replaces it, and an
// undischarged cast reaches that write still claimed.
func TestCheckPublishesValidityAndProvenanceForACheckedModule(t *testing.T) {
	result, err := Check("local raw: any = os.clock()\nlocal total = 0\nif type(raw) == \"number\" then\n\tlocal checked = raw :: number\n\ttotal = checked\nend\nreturn total\n")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Native == nil {
		t.Fatal("checked module published no fact index")
	}
	var initial, replacement NativeFact
	for _, fact := range result.Native.Facts() {
		if fact.Lane != NativeLaneValues || fact.Family != "value" || fact.Subject != "total" {
			continue
		}
		if fact.Value == "scalar/number/0" {
			initial = fact
			continue
		}
		replacement = fact
	}
	if initial.Established == "" || initial.Revoked == "" {
		t.Fatalf("the replaced constant carries validity %#v, want an established and a revoked epoch", initial)
	}
	if initial.Revoked != replacement.Established {
		t.Fatalf("the constant is revoked at %q but its replacement is established at %q", initial.Revoked, replacement.Established)
	}
	if initial.Event != "environment-write" {
		t.Fatalf("the revoking event is %q, want the write that replaced the binding", initial.Event)
	}
	if initial.Trust != NativeTrustProven {
		t.Fatalf("the literal constant is %q, want %q", initial.Trust, NativeTrustProven)
	}
	// The cast was never discharged, so the value it produced stays claimed
	// through the assignment that carries it to another binding.
	if replacement.Trust != NativeTrustClaimed {
		t.Fatalf("the value assigned from an undischarged cast is %q, want %q", replacement.Trust, NativeTrustClaimed)
	}
}

func TestCheckNonNilAssertionAndItsCopyStayClaimed(t *testing.T) {
	result, err := Check(`
local optional: string? = "found"
local asserted = optional!
local downstream = asserted
`)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Native == nil {
		t.Fatal("checked module published no fact index")
	}
	trust := make(map[string][]string)
	for _, fact := range result.Native.Facts() {
		if fact.Lane == NativeLaneValues && fact.Family == "value" {
			trust[fact.Subject] = append(trust[fact.Subject], fact.Trust)
		}
	}
	for _, subject := range []string{"asserted", "downstream"} {
		if !containsTrust(trust[subject], NativeTrustClaimed) || containsTrust(trust[subject], NativeTrustProven) {
			t.Fatalf("%s trust = %#v, want claimed and no proven publication", subject, trust[subject])
		}
	}
}

func containsTrust(trust []string, want string) bool {
	for _, got := range trust {
		if got == want {
			return true
		}
	}
	return false
}

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
