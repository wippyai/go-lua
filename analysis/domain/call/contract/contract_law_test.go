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
		t.Fatal("Call contracts changed across identical calls")
	}
	if got, ok := Contracts(engine.SemanticKey{}); ok || got != nil {
		t.Fatal("Call contracts accepted an unavailable factor")
	}
	for _, contract := range first {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor || !contract.Conclusion.Available() {
			t.Fatal("Call contract has malformed factor ownership")
		}
	}
	canonical, canonicalOK := coverage.SealContracts(first)
	if !canonicalOK || !reflect.DeepEqual(first, canonical) {
		t.Fatal("Call contracts are not in generic canonical order")
	}
}

func TestContractsHaveExactRelevantSourceInventory(t *testing.T) {
	contracts, ok := Contracts(testFactor(2))
	if !ok {
		t.Fatal("Call contracts")
	}
	want := []sourceCount{
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet, 1},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet, 1},
		{semanticsource.OriginProgramFlowFunction, 0, 1},
		{semanticsource.OriginProgramFlowCall, 0, 1},
		{semanticsource.OriginProgramFlowOutcome, 0, 1},
		{semanticsource.OriginTargetOperation, 0, 1},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI, 1},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback, 1},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, 1},
		{semanticsource.OriginLinkProjectBaseApplication, 0, 1},
		{semanticsource.OriginLinkBoundary, 0, 1},
		{semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, 1},
	}
	assertSourceInventory(t, contracts, want)
}

func TestContractsShareOneDispatchConclusionAcrossSelectedCallOperands(t *testing.T) {
	factor := testFactor(3)
	contracts, ok := Contracts(factor)
	if !ok {
		t.Fatal("Call contracts")
	}
	dispatchConclusion, dispatchOK := coverage.DeriveConclusion(factor, uint16(dispatch), revision)
	if !dispatchOK {
		t.Fatal("Call dispatch conclusion")
	}
	for _, source := range []struct {
		origin semanticsource.Origin
		facet  semanticsource.Facet
	}{
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet},
		{semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet},
		{semanticsource.OriginProgramFlowCall, 0},
		{semanticsource.OriginTargetOperation, 0},
		{semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI},
		{semanticsource.OriginLinkProjectBaseApplication, 0},
		{semanticsource.OriginLinkBoundary, 0},
	} {
		definition, found := semanticsource.Definition(source.origin, source.facet)
		if !found {
			t.Fatal("missing generated source definition")
		}
		if !containsPair(contracts, definition.Token(), dispatchConclusion) {
			t.Fatal("Call selected-dispatch operands did not share one conclusion")
		}
	}
}

func TestContractsContainNoDuplicateRequirement(t *testing.T) {
	seen := make(map[coverage.CoverageContract]struct{})
	contracts, ok := Contracts(testFactor(4))
	if !ok {
		t.Fatal("Call contracts")
	}
	for _, contract := range contracts {
		if _, duplicate := seen[contract]; duplicate {
			t.Fatal("duplicate Call coverage contract")
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
		t.Fatalf("Call source count = %d, want %d", len(got), len(want))
	}
	for _, expected := range want {
		definition, found := semanticsource.Definition(expected.origin, expected.facet)
		if !found || got[definition.Token()] != expected.count {
			t.Fatalf("Call source %d/%d count = %d, want %d", expected.origin, expected.facet, got[definition.Token()], expected.count)
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
