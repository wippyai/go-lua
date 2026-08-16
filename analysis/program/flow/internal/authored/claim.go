package authored

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ValueClaim is erased source claim intent after its operand is evaluated.
// Static separately owns the optional type target of TypeAs and TypeColonColon;
// NonNil has no target.
type ValueClaim struct {
	Owner   keyspace.Term
	Operand keyspace.Term
	Kind    kind.ValueClaimKind
}

func validValueClaimKind(value kind.ValueClaimKind) bool {
	return value >= kind.ValueClaimTypeAs && value <= kind.ValueClaimNonNil
}

// TypeValue is one runtime scalar type-load occurrence. Static separately owns
// the runtime-loadable type target.
type TypeValue struct{ Owner keyspace.Term }

type claimStore struct {
	claims     []ValueClaim
	typeValues []TypeValue
}

func (view Claims) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.claims.claims)
}

func (view Claims) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyValueClaim, index, len(view.component.claims.claims))
}

func (view Claims) Get(term keyspace.Term) (owner, operand keyspace.Term, claimKind kind.ValueClaimKind, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyValueClaim, len(view.component.claims.claims)) {
		return 0, 0, 0, false
	}
	row := view.component.claims.claims[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Operand, row.Kind, true
}

func (view TypeValues) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.claims.typeValues)
}

func (view TypeValues) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyTypeValue, index, len(view.component.claims.typeValues))
}

func (view TypeValues) Get(term keyspace.Term) (owner keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyTypeValue, len(view.component.claims.typeValues)) {
		return 0, false
	}
	return view.component.claims.typeValues[keyspace.TermOrdinal(term)-1].Owner, true
}
