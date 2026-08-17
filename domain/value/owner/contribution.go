package owner

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/value"
)

// This file is the outward half of the mount hook beside it. Mount seals this
// Link's value universe into a solve; Contribute publishes one completed
// solve's value lane out of it, onto the one column this axis declares. Both
// halves are stated by this package, so what the value factor's column holds is
// value's own statement rather than a shape a composition reconstructs.
//
// The projection speaks no storage vocabulary. It hands rows to a writer the
// materializer supplies, which keeps the published column's storage out of this
// domain and this domain's coordinates out of that storage.

// columnDenominator is the derivation domain of the key universe this axis's
// column is total over. It is domain separated from every other contributor's,
// so two columns keyed by two domains' coordinates can never name one
// membership authority.
const columnDenominator = "analysis/domain/value/owner/column-denominator/v1"

// Lane is one completed solve's value factor lane, read one coordinate at a
// time: it answers the fact the lane wrote at a coordinate and whether it wrote
// one at all. The materializer holds the lane; this package reads it only at
// coordinates the sealed schema issued, so a fact can never be published
// against a coordinate of another schema.
type Lane func(coordinate value.Coordinate) (fact value.Value, held bool)

// Denominator is the key universe the published column is total over: the
// identity the membership is sealed under, and the members in the schema's own
// declaration order. Both are functions of the seal alone, so what an absent
// row means is settled by the declaration and never by the solve.
//
// The identity is derived from the Link the schema sealed its coordinates from,
// because the value coordinate range is exactly that Link's boundary value
// range. Two schemas of one Link are total over one key universe; schemas of
// two Links never are.
func Denominator(schema *value.Schema) (identity.ContentID, []value.Coordinate, bool) {
	if schema == nil || !schema.Valid() {
		return identity.ContentID{}, nil, false
	}
	source := schema.LinkID()
	count := schema.CoordinateCount()
	if !source.Available() || count == 0 {
		return identity.ContentID{}, nil, false
	}
	members := make([]value.Coordinate, 0, count)
	for index := 0; index < count; index++ {
		coordinate, issued := schema.CoordinateAt(index)
		if !issued {
			return identity.ContentID{}, nil, false
		}
		members = append(members, coordinate)
	}
	denominator, derived := identity.DeriveContentID(columnDenominator, source[:])
	if !derived {
		return identity.ContentID{}, nil, false
	}
	return denominator, members, true
}

// Contribute publishes one solved lane onto the column this axis declares. It
// walks the schema's coordinates in declaration order and hands the writer the
// fact the lane holds at each; a coordinate the lane holds no fact for is
// written no row, and against the denominator above it reads back as a proven
// absence rather than as ignorance.
//
// A fact the schema does not admit at its coordinate is refused rather than
// published: the column states what this schema's own algebra owns, so a value
// of another schema cannot reach a consumer through it.
func Contribute(schema *value.Schema, lane Lane, publish func(coordinate value.Coordinate, fact value.Value) bool) bool {
	if schema == nil || !schema.Valid() || lane == nil || publish == nil {
		return false
	}
	count := schema.CoordinateCount()
	if count == 0 {
		return false
	}
	for index := 0; index < count; index++ {
		coordinate, issued := schema.CoordinateAt(index)
		if !issued {
			return false
		}
		fact, held := lane(coordinate)
		if !held {
			continue
		}
		if !schema.AdmitsCoordinate(coordinate, fact) || !publish(coordinate, fact) {
			return false
		}
	}
	return true
}

// FoldSummary folds the rows one subject's column holds into the answer the
// value-summary family publishes for it. The fold is the domain's own: it opens
// at the schema's coordinate width and joins coordinatewise under the schema's
// order, which is the very fold the family's declaration beside this file binds
// the solve to. The family's result column is therefore a fold over the
// published rows and not a second reading of the solve.
func FoldSummary(schema *value.Schema, lane Lane) (value.ValueSummaryObservation, bool) {
	if schema == nil || !schema.Valid() || lane == nil {
		return value.ValueSummaryObservation{}, false
	}
	count := schema.CoordinateCount()
	folded, ok := value.AccumulateValueSummaryRows(schema, value.BeginValueSummary(schema), count, func(index int) (value.Value, bool, bool) {
		coordinate, issued := schema.CoordinateAt(index)
		if !issued {
			return value.Value{}, false, false
		}
		fact, held := lane(coordinate)
		if held && !schema.AdmitsCoordinate(coordinate, fact) {
			return value.Value{}, false, false
		}
		return fact, held, true
	})
	if !ok {
		return value.ValueSummaryObservation{}, false
	}
	return folded, true
}

// ContributeSummary publishes one subject's answer into the result column the
// value-summary family is answered through. The subject key belongs to the
// materializer, which is what the query family is asked at; the answer belongs
// to this domain, which is what the query family folds.
func ContributeSummary[S comparable](schema *value.Schema, subject S, lane Lane, publish func(subject S, answer value.ValueSummaryObservation) bool) bool {
	if publish == nil {
		return false
	}
	answer, folded := FoldSummary(schema, lane)
	if !folded {
		return false
	}
	return publish(subject, answer)
}
