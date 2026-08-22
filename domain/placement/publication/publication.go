// Package publication exposes Placement's typed view of a detached analysis
// result. The generic Result remains the owner of query geometry; this package
// only selects the unique placement-summary family and decodes its cells.
package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/result"
	"github.com/wippyai/go-lua/domain/placement"
)

// Open opens the unique placement-summary family in a detached result. A
// missing or duplicated family is rejected rather than selecting by ordinal.
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
		if !ok || candidate.Key() != placement.SummaryResultFamily {
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

// Family is the immutable placement-summary query family in a detached
// Result.
type Family struct{ family result.Family }

// QueryCount reports the number of placement-summary query rows.
func (family Family) QueryCount() int { return family.family.QueryCount() }

// QueryAt returns one immutable placement-summary query row.
func (family Family) QueryAt(index int) (Query, bool) {
	query, ok := family.family.QueryAt(index)
	if !ok {
		return Query{}, false
	}
	return Query{query: query}, true
}

// Query is one immutable placement-summary query row.
type Query struct{ query result.Query }

// Status reports the detached query publication status.
func (query Query) Status() result.QueryStatus { return query.query.Status() }

// SiteID returns the semantic site identity for this query row.
func (query Query) SiteID() (identity.ContentID, bool) { return query.query.SiteID() }

// MountID returns the mount identity for this query row.
func (query Query) MountID() (identity.ContentID, bool) { return query.query.MountID() }

// ContextID returns the exact canonical execution context carried by the
// detached placement publication site. It never derives or defaults a
// context from the opaque SiteID or publication key.
func (query Query) ContextID() (identity.ContentID, bool) { return query.query.ContextID() }

// PointID returns the point identity for this query row.
func (query Query) PointID() (identity.ContentID, bool) { return query.query.PointID() }

// BodyCount reports the number of bodies reached by this query's point.
func (query Query) BodyCount() int { return query.query.BodyCount() }

// BodyAt returns one immutable body geometry row.
func (query Query) BodyAt(index int) (result.Body, bool) { return query.query.BodyAt(index) }

// Placement decodes the typed placement-summary cell for a hit query under
// the caller's exact Placement schema. Misses, proven absences, invalid rows,
// unavailable cells, and schema-mismatched payloads are rejected here.
func (query Query) Placement(expected placement.Schema) (placement.SummaryResult, bool) {
	if query.query.Status() != result.QueryHit {
		return placement.SummaryResult{}, false
	}
	cell, ok := query.query.Cell()
	if !ok {
		return placement.SummaryResult{}, false
	}
	return placement.DecodeSummaryResult(expected, cell.Present(), cell.RowCount(), cell.Payload())
}
