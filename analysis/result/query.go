package result

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// QueryStatus is the closed publication state retained by a detached Result.
// Miss and invalid never enter Result: detachment refuses them. Proven absence
// remains distinct from a hit whose domain cell reports Present=false.
type QueryStatus uint8

const (
	QueryInvalid QueryStatus = iota
	QueryHit
	QueryProvenAbsent
)

// resultFamily owns one sealed publication contract and all of its site rows.
// The family ordinal is the publication registry ordinal and is retained once
// here rather than copied into every query row.
type resultFamily struct {
	ordinal  uint32
	key      schema.Key
	contract engine.CanonicalResultContract
	queries  []resultQuery
}

// resultQuery is deliberately generic. Mount and point identity are resolved
// through point; family identity and codec are resolved through family.
type resultQuery struct {
	site   identity.ContentID
	key    identity.ContentID
	point  uint32
	status QueryStatus
	cell   engine.CanonicalResultCell
}

func (row resultQuery) valid(points []resultPoint, contract engine.CanonicalResultContract) bool {
	if !row.site.Available() || !row.key.Available() || row.point == 0 || uint64(row.point) > uint64(len(points)) || !contract.Available() {
		return false
	}
	switch row.status {
	case QueryHit:
		return row.cell.Available() && row.cell.ContractID() == contract.ContentID()
	case QueryProvenAbsent:
		return !row.cell.Available()
	default:
		return false
	}
}

// Family is one immutable generic query family in a Result.
type Family struct {
	owner   *Result
	ordinal uint32
}

func (result *Result) FamilyCount() int {
	if !result.valid() {
		return 0
	}
	return len(result.families)
}

func (result *Result) FamilyAt(index int) (Family, bool) {
	if !result.valid() || index < 0 || index >= len(result.families) {
		return Family{}, false
	}
	return Family{owner: result, ordinal: uint32(index + 1)}, true
}

// FamilyByKey returns the unique query family whose authored publication key
// matches key. A missing key and an ambiguous key are both rejected so callers
// cannot silently select one family from a malformed multi-family result.
func (result *Result) FamilyByKey(key schema.Key) (Family, bool) {
	if !result.valid() || !key.Available() {
		return Family{}, false
	}
	var selected Family
	matches := 0
	for index, candidate := range result.families {
		if candidate.key != key {
			continue
		}
		matches++
		selected = Family{owner: result, ordinal: uint32(index + 1)}
	}
	return selected, matches == 1
}

func (family Family) row() (resultFamily, bool) {
	if family.owner == nil || !family.owner.valid() || family.ordinal == 0 || uint64(family.ordinal) > uint64(len(family.owner.families)) {
		return resultFamily{}, false
	}
	row := family.owner.families[family.ordinal-1]
	if row.ordinal != family.ordinal {
		return resultFamily{}, false
	}
	return row, true
}

func (family Family) Key() schema.Key {
	row, ok := family.row()
	if !ok {
		return ""
	}
	return row.key
}

// ID is the registered semantic family identity carried by the contract.
func (family Family) ID() identity.ContentID {
	row, ok := family.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.contract.FamilyID()
}

func (family Family) Codec() identity.SemanticKey {
	row, ok := family.row()
	if !ok {
		return identity.SemanticKey{}
	}
	return row.contract.Codec()
}

func (family Family) ContractID() identity.ContentID {
	row, ok := family.row()
	if !ok {
		return identity.ContentID{}
	}
	return row.contract.ContentID()
}

func (family Family) QueryCount() int {
	if _, ok := family.row(); !ok {
		return 0
	}
	return len(family.owner.families[family.ordinal-1].queries)
}

func (family Family) QueryAt(index int) (Query, bool) {
	row, ok := family.row()
	if !ok || index < 0 || index >= len(row.queries) {
		return Query{}, false
	}
	return Query{owner: family.owner, family: family.ordinal, ordinal: uint32(index + 1)}, true
}

// Query is one immutable site publication row. The row's point and family
// ordinals resolve all shared geometry and contract information from Result.
type Query struct {
	owner   *Result
	family  uint32
	ordinal uint32
}

func (query Query) row() (resultQuery, resultFamily, bool) {
	if query.owner == nil || !query.owner.valid() || query.family == 0 || uint64(query.family) > uint64(len(query.owner.families)) {
		return resultQuery{}, resultFamily{}, false
	}
	family := query.owner.families[query.family-1]
	if family.ordinal != query.family || query.ordinal == 0 || uint64(query.ordinal) > uint64(len(family.queries)) {
		return resultQuery{}, resultFamily{}, false
	}
	return family.queries[query.ordinal-1], family, true
}

func (query Query) SiteID() (identity.ContentID, bool) {
	row, _, ok := query.row()
	return row.site, ok
}

func (query Query) PublicationKey() (identity.ContentID, bool) {
	row, _, ok := query.row()
	return row.key, ok
}

func (query Query) Status() QueryStatus {
	row, _, ok := query.row()
	if !ok {
		return QueryInvalid
	}
	return row.status
}

func (query Query) Cell() (engine.CanonicalResultCell, bool) {
	row, _, ok := query.row()
	return row.cell, ok && row.status == QueryHit && row.cell.Available()
}

func (query Query) pointRow() (resultPoint, bool) {
	row, _, ok := query.row()
	if !ok || row.point == 0 || uint64(row.point) > uint64(len(query.owner.points)) {
		return resultPoint{}, false
	}
	return query.owner.points[row.point-1], true
}

func (query Query) PointID() (identity.ContentID, bool) {
	point, ok := query.pointRow()
	return point.point, ok
}

func (query Query) MountID() (identity.ContentID, bool) {
	point, ok := query.pointRow()
	return point.mount, ok
}

// ContextID returns the exact canonical execution context carried by the
// detached publication site. A generic or malformed context-free point does
// not acquire a fallback context here.
func (query Query) ContextID() (identity.ContentID, bool) {
	point, ok := query.pointRow()
	return point.context, ok && point.context.Available()
}

func (query Query) BodyCount() int {
	point, ok := query.pointRow()
	if !ok {
		return 0
	}
	return len(point.bodies)
}

func (query Query) BodyAt(index int) (Body, bool) {
	point, ok := query.pointRow()
	if !ok || index < 0 || index >= len(point.bodies) {
		return Body{}, false
	}
	bodyOrdinal := point.bodies[index]
	if bodyOrdinal == 0 || uint64(bodyOrdinal) > uint64(len(query.owner.bodies)) {
		return Body{}, false
	}
	return Body{owner: query.owner, ordinal: bodyOrdinal}, true
}
