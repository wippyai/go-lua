// Package publication exposes the effect domain's typed view of a detached
// analysis result. The underlying result remains the owner of query geometry;
// this package only selects the exact-effect family and decodes its cells.
package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/effect/factor"
)

// Open opens the unique exact-effect family in a detached result.
//
// A result with no matching family, or with more than one matching family, is
// not an effect publication and is rejected. Family keys are the only
// authority used for this selection; the returned wrapper retains no mutable
// state.
func Open(input *result.Result) (Family, bool) {
	if input == nil {
		return Family{}, false
	}
	var (
		selected result.Family
		matches  int
	)
	for index := 0; index < input.FamilyCount(); index++ {
		candidate, ok := input.FamilyAt(index)
		if !ok || candidate.Key() != factor.ExactResultFamily {
			continue
		}
		matches++
		selected = candidate
	}
	if matches != 1 {
		return Family{}, false
	}
	return Family{family: selected}, true
}

// Family is the immutable exact-effect query family in a detached result.
type Family struct {
	family result.Family
}

// QueryCount reports the number of published exact-effect query rows.
func (family Family) QueryCount() int {
	return family.family.QueryCount()
}

// QueryAt returns one immutable exact-effect query row.
func (family Family) QueryAt(index int) (Query, bool) {
	query, ok := family.family.QueryAt(index)
	if !ok {
		return Query{}, false
	}
	return Query{query: query}, true
}

// Query is one immutable exact-effect query row.
type Query struct {
	query result.Query
}

// Status reports the detached query publication status.
func (query Query) Status() result.QueryStatus {
	return query.query.Status()
}

// SiteID returns the semantic site identity for this query row.
func (query Query) SiteID() (identity.ContentID, bool) {
	return query.query.SiteID()
}

// MountID returns the mount identity for this query row.
func (query Query) MountID() (identity.ContentID, bool) {
	return query.query.MountID()
}

// PointID returns the point identity for this query row.
func (query Query) PointID() (identity.ContentID, bool) {
	return query.query.PointID()
}

// BodyCount reports the number of bodies reached by this query's point.
func (query Query) BodyCount() int {
	return query.query.BodyCount()
}

// BodyAt returns one immutable body geometry row.
func (query Query) BodyAt(index int) (result.Body, bool) {
	return query.query.BodyAt(index)
}

// Effect decodes the typed exact-effect cell for a hit query. Misses, proven
// absences, invalid rows, and unavailable cells are rejected here so a caller
// cannot mistake query status for a domain effect.
func (query Query) Effect() (factor.Result, bool) {
	if query.query.Status() != result.QueryHit {
		return factor.Result{}, false
	}
	cell, ok := query.query.Cell()
	if !ok {
		return factor.Result{}, false
	}
	return factor.DecodeResult(cell.Present(), cell.RowCount(), cell.Payload())
}
