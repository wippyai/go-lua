package owner

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/effect/factor"
)

// This file is the outward half of the mount hook beside it. Mount seals this
// Link's effect algebra into a solve; Contribute publishes one completed
// solve's effect lane out of it, onto the one column this axis declares. Both
// halves are stated by this package, so what the effect factor's column holds
// is effect's own statement rather than a shape a composition reconstructs.
//
// The projection speaks no storage vocabulary. It hands rows to a writer the
// materializer supplies, which keeps the published column's storage out of this
// domain and this domain's roots out of that storage.

// columnDenominator is the derivation domain of the key universe this axis's
// column is total over. It is domain separated from every other contributor's,
// so two columns keyed by two domains' coordinates can never name one
// membership authority.
const columnDenominator = "analysis/domain/effect/owner/column-denominator/v1"

// Lane is one completed solve's effect factor lane, read one root at a time: it
// answers the fact the lane wrote at a root and whether it wrote one at all.
// The materializer holds the lane; this package reads it only at roots the
// sealed algebra issued, so a fact can never be published against a root of
// another algebra.
type Lane func(root factor.Root) (fact factor.Value, held bool)

// Denominator is the key universe the published column is total over: the
// identity the membership is sealed under, and the members in the algebra's own
// declaration order. Both are functions of the seal alone, so what an absent
// row means is settled by the declaration and never by the solve.
//
// The identity is derived from the members themselves, in that order. An effect
// algebra publishes the portable identity of every body root it sealed, so the
// membership authority is the content of the set it covers rather than a name
// that stands in for it.
func Denominator(algebra *factor.Algebra) (identity.ContentID, []factor.Root, bool) {
	if algebra == nil || !algebra.Valid() {
		return identity.ContentID{}, nil, false
	}
	count := algebra.RootCount()
	if count == 0 {
		return identity.ContentID{}, nil, false
	}
	members := make([]factor.Root, 0, count)
	parts := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		root, issued := algebra.RootAt(index)
		if !issued {
			return identity.ContentID{}, nil, false
		}
		id, named := algebra.RootID(root)
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
// walks the algebra's roots in declaration order and hands the writer the fact
// the lane holds at each; a root the lane holds no fact for is written no row,
// and against the denominator above it reads back as a proven absence rather
// than as ignorance.
//
// A fact the algebra does not admit at its root is refused rather than
// published: the column states what this algebra owns, so a value of another
// algebra cannot reach a consumer through it.
func Contribute(algebra *factor.Algebra, lane Lane, publish func(root factor.Root, fact factor.Value) bool) bool {
	if algebra == nil || !algebra.Valid() || lane == nil || publish == nil {
		return false
	}
	count := algebra.RootCount()
	if count == 0 {
		return false
	}
	for index := 0; index < count; index++ {
		root, issued := algebra.RootAt(index)
		if !issued {
			return false
		}
		fact, held := lane(root)
		if !held {
			continue
		}
		if !algebra.Admit(root, fact) || !publish(root, fact) {
			return false
		}
	}
	return true
}

// FoldExact folds the one row a subject's column holds into the answer the
// effect-exact family publishes for it. The fold is the domain's own: it opens
// on the algebra and joins the single observed row, which is the very fold the
// solve's exact query runs. The family's result column is therefore a fold over
// the published row and not a second reading of the solve.
//
// The row is exact: an absent row folds to a present-free answer of one row,
// and a root the column proves nothing about folds to no answer at all.
func FoldExact(algebra *factor.Algebra, root factor.Root, lane Lane) (factor.EffectObservation, bool) {
	if algebra == nil || !algebra.Valid() || lane == nil {
		return factor.EffectObservation{}, false
	}
	if _, indexed := algebra.RootIndex(root); !indexed {
		return factor.EffectObservation{}, false
	}
	fact, held := lane(root)
	if held && !algebra.Admit(root, fact) {
		return factor.EffectObservation{}, false
	}
	folded, ok := factor.AccumulateEffect(algebra, factor.BeginEffect(algebra), fact, held, true)
	if !ok {
		return factor.EffectObservation{}, false
	}
	return folded, true
}

// ContributeExact publishes one subject's answer into the result column the
// effect-exact family is answered through. The subject key belongs to the
// materializer, which is what the query family is asked at; the answer belongs
// to this domain, which is what the query family folds.
func ContributeExact[S comparable](algebra *factor.Algebra, subject S, root factor.Root, lane Lane, publish func(subject S, answer factor.EffectObservation) bool) bool {
	if publish == nil {
		return false
	}
	answer, folded := FoldExact(algebra, root, lane)
	if !folded {
		return false
	}
	return publish(subject, answer)
}
