package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
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
		"value/", "closure/", "declared-type/", epochFactPrefix,
		indexReadDisplayPrefix, indexReadScalarPrefix, affineIndexPrefix, booleanResultPrefix,
		summaryTypePrefix, methodReturnSummaryPrefix,
	}
	closure := equation.OutputClosure{}
	for _, family := range families {
		closure.Values = append(closure.Values, equation.Fact{Key: family + term + "/op-00000001", Value: []byte("x")})
	}
	tableIdentityKey := factkey.BuildKey(
		factkey.HeapTableIdentity, []factkey.Part{factkey.TermPart(term)}, "op-00000001",
	).String()
	closure.Values = append(closure.Values, equation.Fact{Key: tableIdentityKey, Value: []byte("x")})
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	for _, family := range families {
		key := family + term + "/op-00000001"
		if !kept[key] {
			t.Errorf("recursive summary dropped %q, a fact about its own boundary term", key)
		}
	}
	if !kept[tableIdentityKey] {
		t.Errorf("recursive summary dropped %q, a fact about its own boundary term", tableIdentityKey)
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
	closure := equation.OutputClosure{Values: []equation.Fact{
		{Key: factkey.BuildKey(factkey.HeapTableIdentity, []factkey.Part{factkey.TermPart(term)}, "op-00000001").String(), Value: identity},
		{Key: factkey.BuildKey(factkey.HeapTableClosed, []factkey.Part{factkey.IdentityPart(identity)}, "op-00000001").String(), Value: []byte("closed")},
		{Key: factkey.BuildKey(factkey.HeapMember, []factkey.Part{factkey.IdentityPart(identity), factkey.EncodedOpaquePart(".a")}, "op-00000001").String(), Value: []byte("scalar/number/1")},
	}}
	closure.Values = append(closure.Values,
		equation.Fact{
			Key: factkey.BuildKey(factkey.HeapTableEscape, []factkey.Part{factkey.TaggedIdentityPart(identity)}, "op-00000001").String(), Value: []byte("escaped"),
		},
		equation.Fact{
			Key: factkey.BuildKey(factkey.HeapLengthFloor, []factkey.Part{factkey.TaggedIdentityPart(identity)}, "op-00000001").String(), Value: []byte("2"),
		},
		equation.Fact{
			Key: factkey.BuildKey(factkey.HeapIndexPresence, []factkey.Part{
				factkey.TaggedIdentityPart(identity), factkey.EncodedTermPart([]byte("path/sym3")),
			}, "op-00000001").String(), Value: []byte("proven"),
		},
		equation.Fact{
			Key: factkey.BuildKey(factkey.HeapKeyPresence, []factkey.Part{
				factkey.TaggedIdentityPart(identity), factkey.EncodedTermPart([]byte(".a")),
			}, "op-00000001").String(), Value: []byte("proven"),
		},
	)
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
	foreignTerm := []byte(boundaryTerm(9))
	foreignClosed := factkey.BuildKey(
		factkey.HeapTableClosed, []factkey.Part{factkey.IdentityPart([]byte("sealed-table/test/op-00000009"))}, "op-00000001",
	).String()
	closure := equation.OutputClosure{Values: []equation.Fact{
		{Key: factkey.BuildKey(factkey.HeapTableIdentity, []factkey.Part{factkey.TermPart(term)}, "op-00000001").String(), Value: identity},
		{Key: foreignClosed, Value: []byte("closed")},
		{Key: factkey.BuildKey(factkey.HeapLengthFloor, []factkey.Part{factkey.TaggedTermPart(foreignTerm)}, "op-00000001").String(), Value: []byte("2")},
	}}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	if kept[foreignClosed] {
		t.Error("recursive summary carried a fact about an allocation it never reached")
	}
	foreignFloor := factkey.BuildKey(
		factkey.HeapLengthFloor, []factkey.Part{factkey.TaggedTermPart(foreignTerm)}, "op-00000001",
	).String()
	if kept[foreignFloor] {
		t.Error("recursive summary followed a term-spelled subject as if it named an allocation")
	}
}
