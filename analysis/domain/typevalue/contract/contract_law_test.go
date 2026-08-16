package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsLaws(t *testing.T) {
	factor := contractFactor(t, 3)
	contracts, ok := Contracts(factor)
	if !ok {
		t.Fatal("Contracts rejected available Factor")
	}
	again, ok := Contracts(factor)
	if !ok || !sameContracts(contracts, again) {
		t.Fatal("Contracts is not deterministic")
	}
	if _, ok := Contracts(engine.SemanticKey{}); ok {
		t.Fatal("Contracts accepted unavailable Factor")
	}
	if got, want := sourcesOf(contracts), typeValueSources(t); !sameTokens(got, want) {
		t.Fatalf("source inventory = %#v, want %#v", got, want)
	}
	requirements := make(map[coverage.CoverageContract]struct{}, len(contracts))
	conclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor {
			t.Fatalf("non-factor contract: %#v", contract)
		}
		if _, duplicate := requirements[contract]; duplicate {
			t.Fatalf("duplicate requirement: %#v", contract)
		}
		requirements[contract] = struct{}{}
		conclusions[contract.Conclusion] = struct{}{}
	}
	if len(conclusions) != 3 {
		t.Fatalf("distinct conclusion vocabulary = %d, want 3", len(conclusions))
	}
	original := sourceInventory
	for left, right := 0, len(sourceInventory)-1; left < right; left, right = left+1, right-1 {
		sourceInventory[left], sourceInventory[right] = sourceInventory[right], sourceInventory[left]
	}
	permuted, permutedOK := Contracts(factor)
	sourceInventory = original
	if !permutedOK || !sameContracts(contracts, permuted) {
		t.Fatal("Contracts changed under source-inventory permutation")
	}
}
func contractFactor(t testing.TB, marker byte) engine.SemanticKey {
	t.Helper()
	var digest [32]byte
	digest[0] = marker
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		t.Fatal("NewSemanticKey")
	}
	return key
}
func token(t testing.TB, origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.Token {
	t.Helper()
	definition, ok := semanticsource.Definition(origin, facet)
	if !ok {
		t.Fatalf("Definition(%d,%d)", origin, facet)
	}
	return definition.Token()
}
func typeValueSources(t testing.TB) []semanticsource.Token {
	return []semanticsource.Token{token(t, semanticsource.OriginProgramFlowTypeValue, 0), token(t, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget), token(t, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef), token(t, semanticsource.OriginLinkProjectShardMount, 0), token(t, semanticsource.OriginLinkStatic, 0)}
}
func sourcesOf(contracts []coverage.CoverageContract) []semanticsource.Token {
	result := make([]semanticsource.Token, len(contracts))
	for index, contract := range contracts {
		result[index] = contract.Source
	}
	return result
}
func sameTokens(left, right []semanticsource.Token) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
func sameContracts(left, right []coverage.CoverageContract) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
