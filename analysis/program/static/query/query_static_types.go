package query

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// StaticTypes is the post-seal hot view over Static's complete authored type
// forest. It retains only the immutable canonical census/identity values,
// never the enclosing Component or a copied term list.
type StaticTypes struct {
	authority staticTypeAuthority
}

// StaticTypeRef is one checked local Static type term. The owning query
// validates the term before issuing this value; the reference carries no
// copied Snapshot or census. Term remains the local (family, ordinal) encoding
// used by the canonical keyspace.
type StaticTypeRef struct {
	term keyspace.Term
}

// staticTypeAuthority is the canonical identity/census pair needed by this
// query. It deliberately does not retain the composed Snapshot or any second
// ordinal/index table.
type staticTypeAuthority struct {
	contentID identity.ContentID
	census    [keyspace.FamilyCount]uint32
}

// StaticTypes returns the composed Static type capability.
func (view View) StaticTypes() StaticTypes {
	if !view.available() {
		return StaticTypes{}
	}
	return StaticTypes{authority: staticTypeAuthority{contentID: view.snapshot.contentID, census: view.snapshot.census}}
}

func (types StaticTypes) available() bool {
	return types.authority.contentID.Available()
}

// Count returns the complete canonical Static type forest cardinality.
func (types StaticTypes) Count() int {
	if !types.available() {
		return 0
	}
	total := 0
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if StaticTypeFamily(family) {
			total += int(types.authority.census[family])
		}
	}
	return total
}

// At returns one owner-bound capability in the canonical family order. No
// second ordinal/offset table is materialized.
func (types StaticTypes) At(index int) (StaticTypeRef, bool) {
	if !types.available() || index < 0 {
		return StaticTypeRef{}, false
	}
	offset := uint64(index)
	for family := keyspace.FamilyTypeAlias; family <= keyspace.FamilyTypeConditional; family++ {
		if !StaticTypeFamily(family) {
			continue
		}
		count := uint64(types.authority.census[family])
		if offset < count {
			term := keyspace.MakeTerm(family, uint32(offset+1))
			return StaticTypeRef{term: term}, true
		}
		offset -= count
	}
	return StaticTypeRef{}, false
}

// Ref validates and binds one raw Component-local static type Term. Terms are
// local (family, ordinal) encodings and can be rebound to another snapshot;
// no owner provenance is encoded in the term itself.
func (types StaticTypes) Ref(term keyspace.Term) (StaticTypeRef, bool) {
	if !types.available() || !types.staticTypeTerm(term) {
		return StaticTypeRef{}, false
	}
	return StaticTypeRef{term: term}, true
}

func (types StaticTypes) staticTypeTerm(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	if !StaticTypeFamily(family) {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(types.authority.census[family])
}

// Term recovers the checked local Term. A zero ref returns zero.
func (ref StaticTypeRef) Term() keyspace.Term {
	return ref.term
}

func staticTypeTerm(census [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	if !StaticTypeFamily(family) {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	return ordinal != 0 && uint64(ordinal) <= uint64(census[family])
}

// StaticTypeFamily reports whether family belongs to the closed authored
// Static type-forest denominator.
func StaticTypeFamily(family keyspace.Family) bool {
	return staticrole.TypeReferenceTargetFamily(family) || staticrole.NodeFamily(family)
}
