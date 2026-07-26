package engine

import (
	"encoding/base64"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// sccBoundary builds the compilation shape lexicalSCCSummary reads: the terms a
// recursive body's boundary owns.
func sccBoundary(symbols ...wir.SymbolID) front.Compilation {
	parameters := make([]wir.BoundaryParameter, 0, len(symbols))
	for _, symbol := range symbols {
		parameters = append(parameters, wir.BoundaryParameter{Symbol: symbol})
	}
	return front.Compilation{Boundary: wir.BodyBoundary{Parameters: parameters}}
}

func summaryKeys(closure equation.OutputClosure) map[string]bool {
	keys := make(map[string]bool, len(closure.Values))
	for _, fact := range closure.Values {
		keys[fact.Key] = true
	}
	return keys
}

// TestRecursiveSummaryCarriesEveryBoundaryAnchoredFamily pins that what a
// recursive summary retains is decided by what a fact is about, not by which
// family states it. Each of these families writes its subject in the same
// place, so a fact about a boundary term survives the summary whichever family
// published it — the families a recursive body may not carry are the ones about
// something else.
func TestRecursiveSummaryCarriesEveryBoundaryAnchoredFamily(t *testing.T) {
	term := boundaryTerm(2)
	families := []string{
		"value/", "closure/", "declared-type/", epochFactPrefix, heapTableIdentityPrefix,
		indexReadDisplayPrefix, indexReadScalarPrefix, affineIndexPrefix, booleanResultPrefix,
		summaryTypePrefix, methodReturnSummaryPrefix,
	}
	closure := equation.OutputClosure{}
	for _, family := range families {
		closure.Values = append(closure.Values, equation.Fact{Key: family + term + "/op-00000001", Value: []byte("x")})
	}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	for _, family := range families {
		key := family + term + "/op-00000001"
		if !kept[key] {
			t.Errorf("recursive summary dropped %q, a fact about its own boundary term", key)
		}
	}
}

// TestRecursiveSummaryRefusesForeignSubjects pins the other side. A fact about a
// term this boundary does not own, or one that states no subject at all, is not
// the recursive body's to republish.
func TestRecursiveSummaryRefusesForeignSubjects(t *testing.T) {
	closure := equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/" + boundaryTerm(9) + "/op-00000001", Value: []byte("x")},
		{Key: "value/temp/4/op-00000001", Value: []byte("x")},
		{Key: boundaryTerm(2) + "/op-00000001", Value: []byte("x")},
		{Key: "branch/op-00000001", Value: []byte("x")},
	}}
	for key := range summaryKeys(lexicalSCCSummary(sccBoundary(2), closure)) {
		t.Errorf("recursive summary carried %q, which is not about its boundary", key)
	}
}

// TestRecursiveSummaryFollowsEveryHeapFamily pins the allocation half. Once the
// summary is following an identity, every fact published about that identity
// belongs to it — the index bounds and escape state as much as the members —
// because the identity is what the fact is about, whatever family says it.
func TestRecursiveSummaryFollowsEveryHeapFamily(t *testing.T) {
	term := boundaryTerm(2)
	identity := []byte("sealed-table/test/op-00000001")
	encoded := base64.RawURLEncoding.EncodeToString(identity)
	closure := equation.OutputClosure{Values: []equation.Fact{
		{Key: heapTableIdentityPrefix + term + "/op-00000001", Value: identity},
		{Key: heapTableClosedPrefix + encoded + "/op-00000001", Value: []byte("closed")},
		{Key: heapMemberPrefix + encoded + "/LmE/op-00000001", Value: []byte("scalar/number/1")},
		{Key: heapTableEscapePrefix + heapSubjectIdentityPrefix + encoded + "/op-00000001", Value: []byte("escaped")},
		{Key: heapLengthFloorPrefix + heapSubjectIdentityPrefix + encoded + "/op-00000001", Value: []byte("2")},
		{Key: heapIndexPresencePrefix + heapSubjectIdentityPrefix + encoded + "/cGF0aC9zeW0z/op-00000001", Value: []byte("proven")},
		{Key: heapKeyPresencePrefix + heapSubjectIdentityPrefix + encoded + "/LmE/op-00000001", Value: []byte("proven")},
	}}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	for _, fact := range closure.Values {
		if !kept[fact.Key] {
			t.Errorf("recursive summary dropped %q, a fact about an allocation it carries", fact.Key)
		}
	}
}

// TestRecursiveSummaryRefusesForeignAllocations pins that following an identity
// carries only that identity's own facts, and that a subject naming a path
// rather than an allocation is not followed as one.
func TestRecursiveSummaryRefusesForeignAllocations(t *testing.T) {
	term := boundaryTerm(2)
	identity := []byte("sealed-table/test/op-00000001")
	other := base64.RawURLEncoding.EncodeToString([]byte("sealed-table/test/op-00000009"))
	foreignTerm := base64.RawURLEncoding.EncodeToString([]byte(boundaryTerm(9)))
	closure := equation.OutputClosure{Values: []equation.Fact{
		{Key: heapTableIdentityPrefix + term + "/op-00000001", Value: identity},
		{Key: heapTableClosedPrefix + other + "/op-00000001", Value: []byte("closed")},
		{Key: heapLengthFloorPrefix + heapSubjectTermPrefix + foreignTerm + "/op-00000001", Value: []byte("2")},
	}}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	if kept[heapTableClosedPrefix+other+"/op-00000001"] {
		t.Error("recursive summary carried a fact about an allocation it never reached")
	}
	if kept[heapLengthFloorPrefix+heapSubjectTermPrefix+foreignTerm+"/op-00000001"] {
		t.Error("recursive summary followed a term-spelled subject as if it named an allocation")
	}
}
