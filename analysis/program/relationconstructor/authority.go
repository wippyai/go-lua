package relationconstructor

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// authorityTokenDomain is the one namespace this producer issues in. Every
// token is derived from the owner's own fence and the declaration's own key,
// so an axis's attachment is a function of what that axis declared.
const authorityTokenDomain = "wippy.analysis/relation/authority/token/v1"

// TypeResolver is the deliberately narrow bridge from a declared column's
// carrier to the type its owner issued. Column types are owned by the carrier
// surface, not by this producer, so the resolution is supplied rather than
// derived here. It is consumed only during production and never retained.
type TypeResolver func(carrier.Key) (model.TypeID, bool)

// Authority projects one axis's sealed member catalog into the owner-local
// relational attachment that relcompile installs from.
//
// Every identity it needs is already determined by the declaration. A
// relation's scope is the decision context its rows are produced under, and
// rows exist exactly per candidate the provider admits, so relations sharing a
// candidate provider share a scope. Nothing is minted: tokens are derived from
// the owner's own fence and the declaration's own key, and the model issues
// every identity from those under the one-assigner law.
//
// The producer refuses rather than guesses. Two relations that would share a
// scope while producing rows under different nesting are a composition defect:
// one decides rows per candidate and the other per parent row, so a single
// decision context cannot hold both.
func Authority(axis schema.EntryReference, catalog member.Catalog, owner authority.Owner, types TypeResolver) (authority.Catalog, bool) {
	if !axis.Available() || !owner.Available() || types == nil || !catalog.Complete() {
		return authority.Catalog{}, false
	}
	scopes, ok := declaredScopes(axis, catalog, owner)
	if !ok {
		return authority.Catalog{}, false
	}
	relations := make([]authority.RelationSpec, 0, len(catalog.Relations))
	keys := make([]authority.KeySpec, 0, len(catalog.Relations))
	denominators := make([]authority.DenominatorSpec, 0, len(catalog.Relations))
	for _, relation := range catalog.Relations {
		token, ok := authorityToken(owner, "relation", relation.Key)
		if !ok {
			return authority.Catalog{}, false
		}
		relations = append(relations, authority.RelationSpec{
			Name:        relation.Key,
			Token:       token,
			Scope:       scopeKey(axis, relation),
			Addressing:  declaredAddresses(relation.Addressing),
			Publication: publicationKey(relation),
		})
		for _, vector := range relation.Keys {
			keyToken, ok := authorityToken(owner, "key", vector.Name)
			if !ok {
				return authority.Catalog{}, false
			}
			keys = append(keys, authority.KeySpec{
				Name:     vector.Name,
				Token:    keyToken,
				Relation: relation.Key,
				Columns:  append([]schema.Key(nil), vector.Columns...),
			})
			// A denominator is the universe one relation's rows are addressed
			// over under one key. It has no identity of its own, so it restates
			// the pair rather than naming a third thing.
			denominators = append(denominators, authority.DenominatorSpec{
				Name: vector.Name, Relation: relation.Key, Key: vector.Name,
			})
		}
	}
	columns := make([]authority.ColumnSpec, 0, len(catalog.Projections))
	for _, projection := range catalog.Projections {
		token, ok := authorityToken(owner, "column", projection.Key)
		if !ok {
			return authority.Catalog{}, false
		}
		typeID, ok := types(carrier.Key(projection.Result))
		if !ok || !typeID.Available() {
			return authority.Catalog{}, false
		}
		columns = append(columns, authority.ColumnSpec{
			Name: projection.Key, Token: token, Relation: projection.Relation, Type: typeID,
		})
	}
	declaration, ok := authority.NewDeclaration(relations, columns, keys, scopes, denominators)
	if !ok {
		return authority.Catalog{}, false
	}
	return declaration.Seal(owner)
}

// declaredScopes collects one scope per distinct candidate provider the axis's
// relations name, and refuses a provider whose relations disagree about the
// nesting their rows are produced under.
func declaredScopes(axis schema.EntryReference, catalog member.Catalog, owner authority.Owner) ([]authority.ScopeSpec, bool) {
	scopes := make([]authority.ScopeSpec, 0, len(catalog.Relations))
	nesting := make(map[schema.Key]bool, len(catalog.Relations))
	seen := make(map[schema.Key]struct{}, len(catalog.Relations))
	for _, relation := range catalog.Relations {
		key := scopeKey(axis, relation)
		nested := relation.Parent.Declared()
		if held, present := nesting[key]; present {
			if held != nested {
				return nil, false
			}
			continue
		}
		nesting[key] = nested
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		token, ok := authorityToken(owner, "scope", key)
		if !ok {
			return nil, false
		}
		// A decision scope declares no dimension of its own, so its region is
		// the empty conjunction. Physical extent is not stated here: the mount
		// lowers a region to the support its owner declared for the atoms.
		scopes = append(scopes, authority.ScopeSpec{Name: key, Token: token, Region: region.True()})
	}
	return scopes, true
}

// scopeKey names the decision context one relation's rows are produced under.
//
// The candidate provider is the whole answer: rows exist exactly per candidate
// it admits. A relation naming no provider produces a base population that no
// candidate gates, so it takes the axis's own structural scope.
func scopeKey(axis schema.EntryReference, relation member.Relation) schema.Key {
	provider := relation.CandidateProvider
	switch {
	case provider.AxisRelation.Available():
		return "scope/" + provider.AxisRelation.Axis.Key + "/" + provider.AxisRelation.Member
	case provider.IssuedRow.Available():
		return "scope/issued/" + provider.IssuedRow
	default:
		return "scope/" + axis.Key
	}
}

// publicationKey answers the key a relation publishes under. A relation that
// declares exactly one key vector publishes under it; one that declares several
// states no single publication key, and this producer names none rather than
// choosing among them.
func publicationKey(relation member.Relation) schema.Key {
	if len(relation.Keys) != 1 {
		return ""
	}
	return relation.Keys[0].Name
}

// declaredAddresses restates the coordinates a relation named as the addresses
// its attachment carries. The relation is the authority on which column fills
// which coordinate; this only changes the spelling.
func declaredAddresses(addressing member.Addressing) []authority.Address {
	pairs := [...]struct {
		coordinate authority.Coordinate
		column     schema.Key
	}{
		{authority.CoordinateAddress, addressing.Address},
		{authority.CoordinateParent, addressing.Parent},
		{authority.CoordinateOrdinal, addressing.Ordinal},
		{authority.CoordinateTag, addressing.Tag},
		{authority.CoordinateOccurrence, addressing.Occurrence},
	}
	addresses := make([]authority.Address, 0, len(pairs))
	for _, pair := range pairs {
		if !pair.column.Available() {
			continue
		}
		addresses = append(addresses, authority.Address{Coordinate: pair.coordinate, Column: pair.column})
	}
	return addresses
}

// authorityToken derives one declaration's token inside the owner's identity
// space. The owner fence and the declaration's own key are the whole input, so
// two productions of one declaration issue the same token and no second
// namespace exists.
func authorityToken(owner authority.Owner, kind string, key schema.Key) (identity.ContentID, bool) {
	if !owner.Available() || kind == "" || !key.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(authorityTokenDomain, owner.Token[:], []byte(kind), []byte(key))
}
