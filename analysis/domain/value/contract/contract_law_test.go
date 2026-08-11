package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsLaws(t *testing.T) {
	factor := contractFactor(t, 1)
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
	if got, want := sourcesOf(contracts), sourceTokens(t); !sameTokens(got, want) {
		t.Fatalf("source inventory = %#v, want %#v", got, want)
	}
	seenRequirements := make(map[coverage.CoverageContract]struct{}, len(contracts))
	seenConclusions := make(map[engine.SemanticKey]struct{})
	for _, contract := range contracts {
		if contract.Class != coverage.OwnerFactor || contract.Owner != factor {
			t.Fatalf("non-factor contract: %#v", contract)
		}
		if _, duplicate := seenRequirements[contract]; duplicate {
			t.Fatalf("duplicate requirement: %#v", contract)
		}
		seenRequirements[contract] = struct{}{}
		seenConclusions[contract.Conclusion] = struct{}{}
	}
	if len(seenConclusions) != 7 {
		t.Fatalf("distinct conclusion vocabulary = %d, want 7", len(seenConclusions))
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

// TestValuePlanLeavesBranchLoopBreakControlStructural proves that the
// aggregate FlowControl claim is topology, not a Value Rule conclusion. The
// same boundary applies to operator and claim aggregates: until their
// dedicated Rules are declared, they remain outside this plan.
func TestValuePlanLeavesBranchLoopBreakControlStructural(t *testing.T) {
	factor := contractFactor(t, 11)
	plan, ok := BuildPlan(factor, PlanBindings{
		Source: contractFactor(t, 12), RawGet: contractFactor(t, 13),
		Allocation: contractFactor(t, 14), Bootstrap: contractFactor(t, 15),
		Transfer: contractFactor(t, 16), Query: contractFactor(t, 17),
	})
	if !ok {
		t.Fatal("Value plan rejected complete owner bindings")
	}
	for _, rule := range plan.Rules {
		for _, requirement := range rule.Covers {
			switch requirement.Source.Origin() {
			case semanticsource.OriginProgramFlowControl, semanticsource.OriginProgramFlowOperators, semanticsource.OriginProgramFlowClaim:
				t.Fatalf("Value Rule claimed structural/unsupported Flow source: %#v", requirement.Source)
			}
		}
	}
	if len(plan.Queries) != 1 || len(plan.Queries[0].Covers) != 1 ||
		plan.Queries[0].Covers[0].Source.Origin() != semanticsource.OriginProgramFlowValues {
		t.Fatal("Value literal/value query was not retained")
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
func sourceTokens(t testing.TB) []semanticsource.Token {
	return []semanticsource.Token{
		token(t, semanticsource.OriginProgramSourceExactKey, 0), token(t, semanticsource.OriginProgramFlowLiterals, 0), token(t, semanticsource.OriginProgramFlowValues, 0), token(t, semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence),
		token(t, semanticsource.OriginProgramFlowLens, 0), token(t, semanticsource.OriginProgramFlowStorage, 0), token(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal), token(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead), token(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign), token(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite),
		token(t, semanticsource.OriginProgramFlowConstructors, 0), token(t, semanticsource.OriginProgramFlowTypeValue, 0),
		token(t, semanticsource.OriginTargetContract, 0), token(t, semanticsource.OriginTargetOperation, 0), token(t, semanticsource.OriginTargetBoot, 0), token(t, semanticsource.OriginLinkProjectShardMount, 0),
		token(t, semanticsource.OriginLinkHost, 0), token(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure), token(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot), token(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember), token(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget),
	}
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
