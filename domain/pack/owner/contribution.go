package owner

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/pack"
)

// This file is the outward half of the mount hook beside it. Mount seals this
// Link's pack universe into a solve; Contribute publishes one completed solve's
// pack lane out of it, onto the one column this axis declares. Both halves are
// stated by this package, so what the pack factor's column holds is pack's own
// statement rather than a shape a composition reconstructs.
//
// The projection speaks no storage vocabulary. It hands rows to a writer the
// materializer supplies, which keeps the published column's storage out of this
// domain and this domain's roots out of that storage.

// columnDenominator is the derivation domain of the key universe this axis's
// column is total over. It is domain separated from every other contributor's,
// so two columns keyed by two domains' coordinates can never name one
// membership authority.
const columnDenominator = "analysis/domain/pack/owner/column-denominator/v1"

// Lane is one completed solve's pack factor lane, read one root at a time: it
// answers the fact the lane wrote at a root and whether it wrote one at all.
// The materializer holds the lane; this package reads it only at roots the
// sealed schema issued, so a fact can never be published against a root of
// another schema.
type Lane func(root pack.Root) (fact pack.Value, held bool)

// Denominator is the key universe the published column is total over: the
// identity the membership is sealed under, and the members in the schema's own
// declaration order. Both are functions of the seal alone, so what an absent
// row means is settled by the declaration and never by the solve.
//
// The identity is derived from the members themselves, in that order. A pack
// schema publishes the portable identity of every root it sealed, so the
// membership authority is the content of the set it covers rather than a name
// that stands in for it. The Link alone would not name it: one Link sealed
// against two static authorities carries two root sets.
func Denominator(schema *pack.Schema) (identity.ContentID, []pack.Root, bool) {
	if !validPackSchema(schema) {
		return identity.ContentID{}, nil, false
	}
	count := schema.RootCount()
	if count == 0 {
		return identity.ContentID{}, nil, false
	}
	members := make([]pack.Root, 0, count)
	parts := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		root, issued := schema.RootAt(index)
		if !issued {
			return identity.ContentID{}, nil, false
		}
		id, named := schema.RootID(root)
		if !named || !id.Available() {
			return identity.ContentID{}, nil, false
		}
		members = append(members, root)
		parts = append(parts, id[:])
	}
	denominator, derived := identity.DeriveContentID(columnDenominator, parts...)
	if !derived {
		return identity.ContentID{}, nil, false
	}
	return denominator, members, true
}

// Contribute publishes one solved lane onto the column this axis declares. It
// walks the schema's roots in declaration order and hands the writer the fact
// the lane holds at each; a root the lane holds no fact for is written no row,
// and against the denominator above it reads back as a proven absence rather
// than as ignorance.
//
// A fact the schema does not admit at its root is refused rather than
// published: the column states what this schema's own algebra owns, so a value
// of another schema cannot reach a consumer through it.
func Contribute(schema *pack.Schema, lane Lane, publish func(root pack.Root, fact pack.Value) bool) bool {
	if !validPackSchema(schema) || lane == nil || publish == nil {
		return false
	}
	count := schema.RootCount()
	if count == 0 {
		return false
	}
	for index := 0; index < count; index++ {
		root, issued := schema.RootAt(index)
		if !issued {
			return false
		}
		fact, held := lane(root)
		if !held {
			continue
		}
		if !schema.Admit(root, fact) || !publish(root, fact) {
			return false
		}
	}
	return true
}
