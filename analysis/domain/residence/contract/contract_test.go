package contract

import (
	"crypto/sha256"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/coverage"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestContractsExactInventoryAndCanonicalDeterminism(t *testing.T) {
	factor := lawFactor(1)
	first, firstOK := Contracts(factor)
	second, secondOK := Contracts(factor)
	if !firstOK || !secondOK || len(first) != len(inventory) || !reflect.DeepEqual(first, second) {
		t.Fatal("non-canonical Residence contract inventory")
	}
	expected := make(map[coverage.CoverageContract]struct{}, len(inventory))
	for _, item := range inventory {
		definition, ok := semanticsource.Definition(item.origin, item.facet)
		conclusion, derived := coverage.DeriveConclusion(factor, item.conclusion, revision)
		if !ok || !derived {
			t.Fatal("invalid Residence source inventory")
		}
		expected[coverage.CoverageContract{Source: definition.Token(), Class: coverage.OwnerFactor, Owner: factor, Conclusion: conclusion}] = struct{}{}
	}
	for _, contract := range first {
		if _, found := expected[contract]; !found {
			t.Fatal("Residence contract escaped exact source inventory")
		}
	}
	if !canonical(first) {
		t.Fatal("Residence contracts are not in canonical source order")
	}
	first[0] = coverage.CoverageContract{}
	replayed, replayedOK := Contracts(factor)
	if !replayedOK || len(replayed) == 0 || replayed[0].Owner != factor {
		t.Fatal("caller mutation reached canonical contract inventory")
	}
}

func TestContractsAllowMultipleConclusionsButNoDuplicateRequirement(t *testing.T) {
	contracts, ok := Contracts(lawFactor(2))
	if !ok {
		t.Fatal("Residence contract")
	}
	bySource := make(map[semanticsource.Token]map[engine.SemanticKey]struct{})
	unique := make(map[coverage.CoverageContract]struct{}, len(contracts))
	for _, contract := range contracts {
		if _, duplicate := unique[contract]; duplicate {
			t.Fatal("duplicate Residence coverage requirement")
		}
		unique[contract] = struct{}{}
		if bySource[contract.Source] == nil {
			bySource[contract.Source] = make(map[engine.SemanticKey]struct{})
		}
		bySource[contract.Source][contract.Conclusion] = struct{}{}
	}
	multiple := false
	for _, conclusions := range bySource {
		multiple = multiple || len(conclusions) > 1
	}
	if !multiple {
		t.Fatal("Residence collapsed distinct retention conclusions")
	}
}

func TestContractsSeparateExactTransferAndContinuationSources(t *testing.T) {
	factor := lawFactor(3)
	contracts, ok := Contracts(factor)
	if !ok {
		t.Fatal("Residence contract")
	}
	for _, expected := range []struct {
		facet      semanticsource.Facet
		conclusion uint16
	}{
		{semanticsource.FacetTargetTransfer, transferConclusion},
		{semanticsource.FacetTargetCallback, callbackConclusion},
		{semanticsource.FacetTargetSuspension, suspensionConclusion},
		{semanticsource.FacetTargetResume, resumeConclusion},
		{semanticsource.FacetTargetSpawn, suspensionConclusion},
	} {
		if !hasTargetConclusion(contracts, expected.facet, expected.conclusion, factor) {
			t.Fatalf("missing exact Residence target source facet %d", expected.facet)
		}
	}
	if hasTargetFacet(contracts, semanticsource.FacetTargetTransferOutcome) || hasTargetFacet(contracts, semanticsource.FacetTargetCallbackRelease) {
		t.Fatal("Residence claimed an unobserved transfer outcome or callback release")
	}
}

func TestContractsRejectInvalidOwner(t *testing.T) {
	if contracts, ok := Contracts(engine.SemanticKey{}); ok || contracts != nil {
		t.Fatal("invalid Residence Factor owner admitted")
	}
}

func canonical(contracts []coverage.CoverageContract) bool {
	for index := 1; index < len(contracts); index++ {
		if compare(contracts[index-1], contracts[index]) >= 0 {
			return false
		}
	}
	return true
}

func compare(left, right coverage.CoverageContract) int {
	if result := compareSource(left.Source, right.Source); result != 0 {
		return result
	}
	if left.Class != right.Class {
		return int(left.Class) - int(right.Class)
	}
	leftDigest, rightDigest := left.Owner.Digest(), right.Owner.Digest()
	for index := range leftDigest {
		if leftDigest[index] != rightDigest[index] {
			return int(leftDigest[index]) - int(rightDigest[index])
		}
	}
	if left.Owner.Version() != right.Owner.Version() {
		if left.Owner.Version() < right.Owner.Version() {
			return -1
		}
		return 1
	}
	leftDigest, rightDigest = left.Conclusion.Digest(), right.Conclusion.Digest()
	for index := range leftDigest {
		if leftDigest[index] != rightDigest[index] {
			return int(leftDigest[index]) - int(rightDigest[index])
		}
	}
	if left.Conclusion.Version() < right.Conclusion.Version() {
		return -1
	}
	if left.Conclusion.Version() > right.Conclusion.Version() {
		return 1
	}
	return 0
}

func compareSource(left, right semanticsource.Token) int {
	if left.Origin() != right.Origin() {
		return int(left.Origin()) - int(right.Origin())
	}
	if left.Facet() != right.Facet() {
		return int(left.Facet()) - int(right.Facet())
	}
	if left.Revision() != right.Revision() {
		return int(left.Revision()) - int(right.Revision())
	}
	if left.Digest() < right.Digest() {
		return -1
	}
	if left.Digest() > right.Digest() {
		return 1
	}
	return 0
}

func lawFactor(value byte) engine.SemanticKey {
	digest := sha256.Sum256([]byte{0x52, value})
	key, ok := engine.NewSemanticKey(digest, uint64(value)+1)
	if !ok {
		panic("law factor")
	}
	return key
}

func hasTargetConclusion(contracts []coverage.CoverageContract, facet semanticsource.Facet, conclusion uint16, factor engine.SemanticKey) bool {
	want, ok := coverage.DeriveConclusion(factor, conclusion, revision)
	if !ok {
		return false
	}
	for _, contract := range contracts {
		if contract.Source.Origin() == semanticsource.OriginTargetOperation && contract.Source.Facet() == facet && contract.Conclusion == want {
			return true
		}
	}
	return false
}

func hasTargetFacet(contracts []coverage.CoverageContract, facet semanticsource.Facet) bool {
	for _, contract := range contracts {
		if contract.Source.Origin() == semanticsource.OriginTargetOperation && contract.Source.Facet() == facet {
			return true
		}
	}
	return false
}
