package contract

import (
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsAreCanonicalAndDeterministic(t *testing.T) {
	factor := testFactor(1)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	if !firstOK || !secondOK || !reflect.DeepEqual(first, second) {
		t.Fatal("Heap contracts changed across identical calls")
	}
	if got, ok := Contracts(engine.SemanticKey{}); ok || got != nil {
		t.Fatal("Heap contracts accepted an unavailable factor")
	}
	for _, contract := range first {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor || !contract.Conclusion.Available() {
			t.Fatal("Heap contract has malformed factor ownership")
		}
	}
	canonical, canonicalOK := coverage.SealContracts(first)
	if !canonicalOK || !reflect.DeepEqual(first, canonical) {
		t.Fatal("Heap contracts are not in generic canonical order")
	}
}

func TestContractsHaveExactRelevantSourceInventory(t *testing.T) {
	contracts, ok := Contracts(testFactor(2))
	if !ok {
		t.Fatal("Heap contracts")
	}
	want := []sourceCount{
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, 1},
		{semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, 1},
		{semanticsource.OriginProgramFlowConstructors, 0, 3},
		{semanticsource.OriginProgramFlowFunction, 0, 2},
		{semanticsource.OriginProgramFlowCall, 0, 1},
		{semanticsource.OriginProgramFlowOutcome, 0, 1},
		{semanticsource.OriginTargetOperation, 0, 1},
		{semanticsource.OriginTargetBoot, 0, 1},
		{semanticsource.OriginLinkProjectBaseApplication, 0, 1},
		{semanticsource.OriginLinkBoundary, 0, 1},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot, 1},
	}
	assertSourceInventory(t, contracts, want)
}

func TestContractsRetainOneFreshConclusionAcrossItsRequiredSources(t *testing.T) {
	factor := testFactor(3)
	contracts, ok := Contracts(factor)
	if !ok {
		t.Fatal("Heap contracts")
	}
	fresh, freshOK := coverage.DeriveConclusion(factor, uint16(freshRoot), revision)
	if !freshOK {
		t.Fatal("Heap fresh conclusion")
	}
	for _, source := range []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}{
		{semanticsource.OriginProgramFlowCall, 0},
		{semanticsource.OriginProgramFlowOutcome, 0},
		{semanticsource.OriginTargetOperation, 0},
		{semanticsource.OriginLinkProjectBaseApplication, 0},
		{semanticsource.OriginLinkBoundary, 0},
	} {
		definition, found := semanticsource.Definition(source.origin, source.facet)
		if !found {
			t.Fatal("missing generated source definition")
		}
		if !containsPair(contracts, definition.Token(), fresh) {
			t.Fatal("Heap fresh root lost a required selected-execution operand")
		}
	}
}

func TestConstructorHasDistinctObjectAndContentsConclusions(t *testing.T) {
	factor := testFactor(4)
	contracts, ok := Contracts(factor)
	definition, found := semanticsource.Definition(semanticsource.OriginProgramFlowConstructors, 0)
	object, objectOK := coverage.DeriveConclusion(factor, uint16(objectRoot), revision)
	contents, contentsOK := coverage.DeriveConclusion(factor, uint16(objectContents), revision)
	if !ok || !found || !objectOK || !contentsOK || object == contents || !containsPair(contracts, definition.Token(), object) || !containsPair(contracts, definition.Token(), contents) {
		t.Fatal("constructor did not retain its two Heap judgments")
	}
}

func TestContractsContainNoDuplicateRequirement(t *testing.T) {
	seen := make(map[coverage.CoverageContract]struct{})
	contracts, ok := Contracts(testFactor(5))
	if !ok {
		t.Fatal("Heap contracts")
	}
	for _, contract := range contracts {
		if _, duplicate := seen[contract]; duplicate {
			t.Fatal("duplicate Heap coverage contract")
		}
		seen[contract] = struct{}{}
	}
}

type sourceCount struct {
	origin semanticsource.Origin
	facet  semanticsource.Facet
	count  int
}

func assertSourceInventory(t testing.TB, contracts []coverage.CoverageContract, want []sourceCount) {
	t.Helper()
	got := make(map[semanticsource.Token]int, len(contracts))
	for _, contract := range contracts {
		got[contract.Source]++
	}
	if len(got) != len(want) {
		t.Fatalf("Heap source count = %d, want %d", len(got), len(want))
	}
	for _, expected := range want {
		definition, found := semanticsource.Definition(expected.origin, expected.facet)
		if !found || got[definition.Token()] != expected.count {
			t.Fatalf("Heap source %d/%d count = %d, want %d", expected.origin, expected.facet, got[definition.Token()], expected.count)
		}
	}
}

func containsPair(contracts []coverage.CoverageContract, source semanticsource.Token, conclusion engine.SemanticKey) bool {
	for _, contract := range contracts {
		if contract.Source == source && contract.Conclusion == conclusion {
			return true
		}
	}
	return false
}

func testFactor(n byte) engine.SemanticKey {
	key, ok := engine.NewSemanticKey(sha256.Sum256([]byte{n}), 1)
	if !ok {
		panic("test factor")
	}
	return key
}
