package engine

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestPathEqualityKeyIsOperandOrderIndependent(t *testing.T) {
	left := pathEqualityKey("path/sym2", "path/sym3")
	right := pathEqualityKey("path/sym3", "path/sym2")
	if left != right {
		t.Fatalf("keys differ by operand order: %q vs %q", left, right)
	}
}

func equalityPartition(t *testing.T, extra ...equation.Fact) equation.Partition {
	t.Helper()
	values := append([]equation.Fact{{
		Key:   pathEqualityKey("path/sym2", "path/sym3") + "/op-00000005",
		Value: []byte("proven"),
	}}, extra...)
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: values})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

func TestProvenPathEqualitiesAreSymmetric(t *testing.T) {
	equal := provenPathEqualities(equalityPartition(t))
	if !equal["sym2"]["sym3"] || !equal["sym3"]["sym2"] {
		t.Fatalf("equality class = %#v, want both directions", equal)
	}
}

func TestProvenPathEqualitiesDropAReplacedSymbol(t *testing.T) {
	partition := equalityPartition(t, equation.Fact{
		Key: epochFactPrefix + "path/sym3/op-00000009", Value: []byte("op-00000009"),
	})
	if equal := provenPathEqualities(partition); len(equal) != 0 {
		t.Fatalf("equality survived a reassignment of one side: %#v", equal)
	}
}

func TestProvenPathEqualitiesKeepAnEarlierEpoch(t *testing.T) {
	partition := equalityPartition(t, equation.Fact{
		Key: epochFactPrefix + "path/sym3/op-00000001", Value: []byte("op-00000001"),
	})
	if equal := provenPathEqualities(partition); !equal["sym2"]["sym3"] {
		t.Fatalf("equality dropped by an epoch that precedes it: %#v", equal)
	}
}

func TestCongruenceOperandSealedRefusesAnAttachedMetatable(t *testing.T) {
	identity := []byte("sealed-table/test/op-00000002")
	term := []byte("path/sym2")
	epoch := equation.Fact{Key: epochFactPrefix + string(term) + "/op-00000002", Value: []byte("op-00000002")}
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		epoch,
		heapIdentityFact(string(term), "op-00000002", identity),
		heapMetaAttachedFact(identity, "op-00000003"),
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if congruenceOperandSealed(term, partition) {
		t.Fatalf("a table carrying an installed metatable was admitted as a reference identity")
	}
	clean, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		epoch,
		heapIdentityFact(string(term), "op-00000002", identity),
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if !congruenceOperandSealed(term, clean) {
		t.Fatalf("a table with no metatable was refused")
	}
}

func TestCorrelatedEqualityTargetsCoverBothTheValueAndItsMembers(t *testing.T) {
	equal := map[string]map[string]bool{
		"p": {"q": true},
		"q": {"p": true},
	}
	whole := correlatedEqualityTargets("p", equal)
	if len(whole) != 1 || whole[0] != "q" {
		t.Fatalf("targets of p = %#v, want [q]", whole)
	}
	member := correlatedEqualityTargets("p.f", equal)
	if len(member) != 1 || member[0] != "q.f" {
		t.Fatalf("targets of p.f = %#v, want [q.f]", member)
	}
}

func TestProvenPathEqualitiesIgnoreAMalformedKey(t *testing.T) {
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: pathEqualityPrefix + "not-base64/" + base64.RawURLEncoding.EncodeToString([]byte("path/sym3")) + "/op-00000005", Value: []byte("proven")},
		{Key: pathEqualityPrefix + base64.RawURLEncoding.EncodeToString([]byte("temp/1")) + "/" + base64.RawURLEncoding.EncodeToString([]byte("path/sym3")) + "/op-00000006", Value: []byte("proven")},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	if equal := provenPathEqualities(partition); len(equal) != 0 {
		t.Fatalf("a malformed relation entered the equality classes: %#v", equal)
	}
}

// assertRefusedAssignment requires a refuted assignment on the given line: the
// congruence transfer must not reach a read the equality does not cover.
func assertRefusedAssignment(t *testing.T, line int, source string) {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == line {
			return
		}
	}
	t.Fatalf("no refutation at line %d: %#v", line, result.PublishedDiagnostics)
}

const congruenceUnionSource = `
type A = {tag: "a", value: string}
type B = {tag: "b", value: number}
%s
`

func TestCongruenceDiscriminantStopsAtTheGuardedArm(t *testing.T) {
	assertRefusedAssignment(t, 9, fmt.Sprintf(congruenceUnionSource, `
local function after(p: A | B, q: A | B): string
    if p == q and p.tag == "a" then
        return ""
    end
    local s: string = q.value
    return s
end
return after
`))
}

func TestCongruenceDiscriminantRefusesAnUnrelatedPeer(t *testing.T) {
	assertRefusedAssignment(t, 7, fmt.Sprintf(congruenceUnionSource, `
local function unrelated(p: A | B, q: A | B): string
    if p.tag == "a" then
        local s: string = q.value
        return s
    end
    return ""
end
return unrelated
`))
}

func TestCongruencePersistsAcrossArmsAndStopsAtAReassignment(t *testing.T) {
	const source = `
local function nested(p: { f: number? }, q: { f: number? }, other: { f: number? }): number
    if p == q then
        if p.f ~= nil then
            local carried: number = q.f
            q = other
            local stale: number = q.f -- expect-error
            return carried + stale
        end
    end
    return 0
end
return nested
`
	result, err := Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	lines := make(map[int]bool)
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" {
			lines[diagnostic.Span.StartLine] = true
		}
	}
	if lines[5] {
		t.Fatalf("equality did not carry into the inner arm: %#v", result.PublishedDiagnostics)
	}
	if !lines[7] {
		t.Fatalf("a reassigned target kept the stale congruence: %#v", result.PublishedDiagnostics)
	}
}
