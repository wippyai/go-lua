package coverage

import "sort"

// Requirement returns the detached semantic obligation carried by one
// owner-issued contract.  It is intentionally a field projection: callers
// still obtain the contract from the domain owner and cannot mint a source
// token or conclusion through this helper.
func (contract CoverageContract) Requirement() Requirement { return contract.requirement() }

// SealRequirements returns one detached, canonical owner-local treatment
// slice.  Domain planners use it after selecting the exact Rule/Query lane;
// it does not introduce a registry or a second source vocabulary.
func SealRequirements(requirements []Requirement) ([]Requirement, bool) {
	if len(requirements) == 0 {
		return nil, false
	}
	sealed := append([]Requirement(nil), requirements...)
	sort.Slice(sealed, func(left, right int) bool { return compareRequirement(sealed[left], sealed[right]) < 0 })
	for index, requirement := range sealed {
		if !validRequirement(requirement) || index > 0 && requirement == sealed[index-1] {
			return nil, false
		}
	}
	return sealed, true
}

// SealContracts returns one detached canonical contract fragment. It owns no
// source or domain vocabulary; it only applies the same validity, order, and
// uniqueness law that Freeze later applies to the complete denominator.
func SealContracts(contracts []CoverageContract) ([]CoverageContract, bool) {
	if len(contracts) == 0 {
		return nil, false
	}
	sealed := append([]CoverageContract(nil), contracts...)
	sort.Slice(sealed, func(left, right int) bool {
		return compareRequirement(sealed[left].requirement(), sealed[right].requirement()) < 0
	})
	for index, contract := range sealed {
		if !validRequirement(contract.requirement()) || index > 0 && contract == sealed[index-1] {
			return nil, false
		}
	}
	return sealed, true
}
