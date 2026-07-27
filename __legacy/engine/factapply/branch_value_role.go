package factapply

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// BranchRelationValueRole is one representation-neutral Values operand in a
// prepared branch transaction. Its identity is the lexical source cell, never
// a concrete State slot or a formal forest-local index. Concrete and formal
// adapters resolve this role exactly once into their respective carriers.
type BranchRelationValueRole struct {
	seal   *branchProgramSeal
	symbol symbol.ID
}

func newBranchLexicalValueRole(seal *branchProgramSeal, id symbol.ID) (BranchRelationValueRole, bool) {
	if seal == nil || id == 0 {
		return BranchRelationValueRole{}, false
	}
	return BranchRelationValueRole{seal: seal, symbol: id}, true
}

// LexicalSymbol returns the canonical semantic source of this role. A caller
// must still prove transaction ownership through a FactorLayout containing it;
// the symbol alone does not grant access to a concrete or formal carrier.
func (r BranchRelationValueRole) LexicalSymbol() (symbol.ID, bool) {
	return r.symbol, r.seal != nil && r.symbol != 0
}

func (r BranchRelationValueRole) validFor(seal *branchProgramSeal) bool {
	return seal != nil && r.seal == seal && r.symbol != 0
}

func (r BranchRelationValueRole) concreteSlot(seal *branchProgramSeal) (statekey.Value, bool) {
	if !r.validFor(seal) {
		return 0, false
	}
	slot := statekey.SymbolValue(r.symbol)
	return slot, slot != 0
}

type branchAtomValueRoles struct {
	currentReads, currentWrites []BranchRelationValueRole
	originalReads               []BranchRelationValueRole
}

type branchValueRoleSource struct{ symbol symbol.ID }

type branchAtomValueRoleSources struct {
	currentReads, currentWrites []branchValueRoleSource
	originalReads               []branchValueRoleSource
}

func branchLexicalValueRoleSource(id symbol.ID) (branchValueRoleSource, bool) {
	if id == 0 {
		return branchValueRoleSource{}, false
	}
	return branchValueRoleSource{symbol: id}, true
}

func sealBranchValueRoleSources(in []branchValueRoleSource, seal *branchProgramSeal) ([]BranchRelationValueRole, error) {
	out := make([]BranchRelationValueRole, 0, len(in))
	for _, source := range in {
		role, ok := newBranchLexicalValueRole(seal, source.symbol)
		if !ok {
			return nil, fmt.Errorf("factapply: malformed lexical branch Values role")
		}
		duplicate := false
		for _, prior := range out {
			duplicate = duplicate || prior.symbol == role.symbol
		}
		if !duplicate {
			out = append(out, role)
		}
	}
	return out, nil
}

func cloneBranchValueRoles(in []BranchRelationValueRole) []BranchRelationValueRole {
	return append([]BranchRelationValueRole(nil), in...)
}

func canonicalBranchValueRoles(in []BranchRelationValueRole, seal *branchProgramSeal) ([]BranchRelationValueRole, bool) {
	out := make([]BranchRelationValueRole, 0, len(in))
	for _, role := range in {
		if !role.validFor(seal) {
			return nil, false
		}
		duplicate := false
		for _, prior := range out {
			duplicate = duplicate || prior.symbol == role.symbol
		}
		if !duplicate {
			out = append(out, role)
		}
	}
	return out, true
}
