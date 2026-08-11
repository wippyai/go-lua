package coverage

import "testing"

func TestSealContractsCanonicalizesAndDetaches(t *testing.T) {
	catalog := lawCatalog(t)
	first, _ := catalog.TokenAt(0)
	second, _ := catalog.TokenAt(1)
	owner := lawKey(1)
	left, _ := DeriveConclusion(owner, 1, 1)
	right, _ := DeriveConclusion(owner, 2, 1)
	input := []CoverageContract{
		{Source: second, Class: OwnerFactor, Owner: owner, Conclusion: right},
		{Source: first, Class: OwnerFactor, Owner: owner, Conclusion: left},
	}
	sealed, ok := SealContracts(input)
	if !ok || len(sealed) != 2 || compareRequirement(sealed[0].requirement(), sealed[1].requirement()) >= 0 {
		t.Fatal("contract fragment was not canonically sealed")
	}
	input[0] = CoverageContract{}
	if sealed[0] == (CoverageContract{}) || sealed[1] == (CoverageContract{}) {
		t.Fatal("sealed contract fragment aliases caller storage")
	}
}

func TestSealContractsRejectsEmptyInvalidAndDuplicateFragments(t *testing.T) {
	if _, ok := SealContracts(nil); ok {
		t.Fatal("empty contract fragment admitted")
	}
	catalog := lawCatalog(t)
	source, _ := catalog.TokenAt(0)
	owner := lawKey(1)
	conclusion, _ := DeriveConclusion(owner, 1, 1)
	contract := CoverageContract{Source: source, Class: OwnerFactor, Owner: owner, Conclusion: conclusion}
	if _, ok := SealContracts([]CoverageContract{{}}); ok {
		t.Fatal("invalid contract admitted")
	}
	if _, ok := SealContracts([]CoverageContract{contract, contract}); ok {
		t.Fatal("duplicate contract admitted")
	}
}
