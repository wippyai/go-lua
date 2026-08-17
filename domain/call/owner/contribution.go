package owner

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/call"
)

// This file is the outward half of the mount hook beside it. Mount seals this
// Link's call universe into a solve; Contribute publishes one completed solve's
// call lane out of it, onto the one column this axis declares. Both halves are
// stated by this package, so what the call factor's column holds is call's own
// statement rather than a shape a composition reconstructs.
//
// The projection speaks no storage vocabulary. It hands rows to a writer the
// materializer supplies, which keeps the published column's storage out of this
// domain and this domain's keys out of that storage.

// columnDenominator is the derivation domain of the key universe this axis's
// column is total over. It is domain separated from every other contributor's,
// so two columns keyed by two domains' coordinates can never name one
// membership authority.
const columnDenominator = "analysis/domain/call/owner/column-denominator/v1"

// Lane is one completed solve's call factor lane, read one key at a time: it
// answers the fact the lane wrote at a key and whether it wrote one at all. The
// materializer holds the lane; this package reads it only at keys the sealed
// algebra issued, so a fact can never be published against a key of another
// algebra.
type Lane func(key call.Key) (fact call.Value, held bool)

// Denominator is the key universe the published column is total over: the
// identity the membership is sealed under, and the members in the algebra's own
// declaration order. Both are functions of the seal alone, so what an absent
// row means is settled by the declaration and never by the solve.
//
// The identity is derived from the members themselves, in that order. A call
// algebra publishes the portable identity of every site it sealed, so the
// membership authority is the content of the set it covers rather than a name
// that stands in for it.
func Denominator(algebra *call.Algebra) (identity.ContentID, []call.Key, bool) {
	if algebra == nil || !algebra.Valid() {
		return identity.ContentID{}, nil, false
	}
	count := algebra.KeyCount()
	if count == 0 {
		return identity.ContentID{}, nil, false
	}
	members := make([]call.Key, 0, count)
	parts := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		key, issued := algebra.KeyAt(index)
		if !issued {
			return identity.ContentID{}, nil, false
		}
		id, named := key.ContentID()
		if !named || !id.Available() {
			return identity.ContentID{}, nil, false
		}
		members = append(members, key)
		parts = append(parts, id[:])
	}
	denominator, derived := identity.DeriveContentID(columnDenominator, parts...)
	if !derived {
		return identity.ContentID{}, nil, false
	}
	return denominator, members, true
}

// Contribute publishes one solved lane onto the column this axis declares. It
// walks the algebra's keys in declaration order and hands the writer the fact
// the lane holds at each; a key the lane holds no fact for is written no row,
// and against the denominator above it reads back as a proven absence rather
// than as ignorance.
//
// A fact the algebra does not admit at its key is refused rather than
// published: the column states what this algebra owns, so a value of another
// algebra cannot reach a consumer through it.
func Contribute(algebra *call.Algebra, lane Lane, publish func(key call.Key, fact call.Value) bool) bool {
	if algebra == nil || !algebra.Valid() || lane == nil || publish == nil {
		return false
	}
	count := algebra.KeyCount()
	if count == 0 {
		return false
	}
	for index := 0; index < count; index++ {
		key, issued := algebra.KeyAt(index)
		if !issued {
			return false
		}
		fact, held := lane(key)
		if !held {
			continue
		}
		if !algebra.OwnsKey(key) || !algebra.Admits(key, fact) || !publish(key, fact) {
			return false
		}
	}
	return true
}
