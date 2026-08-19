package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// The append-only slots these families occupy. They are derived from the last
// slot declared before them so a family added later cannot reuse a slot a
// consumer already addresses.
const (
	slotRegion       = slotStaticExpression + 1
	slotRegionMember = slotRegion + 1
	slotWTOEvent     = slotRegionMember + 1
)

var (
	regionFamily       = Family[Region]{slot: slotRegion, name: "region"}
	regionMemberFamily = Family[RegionMember]{slot: slotRegionMember, name: "region-member"}
	wtoEventFamily     = Family[WTOEvent]{slot: slotWTOEvent, name: "wto-event"}
)

func RegionFamily() Family[Region] { return regionFamily }

func RegionMemberFamily() Family[RegionMember] { return regionMemberFamily }

func WTOEventFamily() Family[WTOEvent] { return wtoEventFamily }

// RegionMember is one point identity a region contains. Its position is its
// ordinal in RegionMemberFamily and the parent region names the half-open
// span it owns, so no region retains a slice header.
type RegionMember struct{ id identity.ContentID }

// NewRegionMember copies one canonical region member identity.
func NewRegionMember(id identity.ContentID) (RegionMember, bool) {
	row := RegionMember{id: id}
	return row, row.Available()
}

func (row RegionMember) Available() bool { return row.id.Available() }

func (row RegionMember) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// Region is one weak-topological-order region. Its members are a span in
// RegionMemberFamily, preserving the canonical member order while making this
// row flat and copy-safe. The first member is the region header, so a region
// that names no member proves nothing and is never sealed.
type Region struct {
	id           identity.ContentID
	parent       identity.ContentID
	memberOffset uint32
	memberCount  uint32
	cyclic       bool
}

// NewRegion copies one canonical Region row and replaces its nested member
// slice with a dense RegionMemberFamily span.
func NewRegion(id, parent identity.ContentID, memberOffset, memberCount uint32, cyclic bool) (Region, bool) {
	row := Region{id: id, parent: parent, memberOffset: memberOffset, memberCount: memberCount, cyclic: cyclic}
	return row, row.Available()
}

func (row Region) Available() bool {
	return row.id.Available() && row.memberCount != 0 &&
		uint64(row.memberOffset)+uint64(row.memberCount) <= uint64(^uint32(0))
}

func (row Region) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row Region) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}

func (row Region) Cyclic() bool { return row.Available() && row.cyclic }

func (row Region) MemberSpan() (offset, count uint32, ok bool) {
	return row.memberOffset, row.memberCount, row.Available()
}

func (row Region) MemberCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.memberCount)
}

// WTO event ordinals retain the artifact's enter/point/exit ordinals. Ordinal
// zero is invalid.
const (
	WTOEventEnter uint8 = 1
	WTOEventPoint uint8 = 2
	WTOEventExit  uint8 = 3
)

// WTOEvent is one bracket in the nested weak topological order. An event
// names a point or a region, never both: the enter and exit brackets name the
// region they open and close, and a point event names the point it visits.
type WTOEvent struct {
	region identity.ContentID
	point  identity.ContentID
	kind   uint8
}

// NewWTOEvent copies one canonical WTOEvent row.
func NewWTOEvent(kind uint8, region, point identity.ContentID) (WTOEvent, bool) {
	row := WTOEvent{kind: kind, region: region, point: point}
	return row, row.Available()
}

func (row WTOEvent) Available() bool {
	if row.kind < WTOEventEnter || row.kind > WTOEventExit {
		return false
	}
	if row.kind == WTOEventPoint {
		return !row.region.Available() && row.point.Available()
	}
	return row.region.Available() && !row.point.Available()
}

func (row WTOEvent) Kind() uint8 {
	if !row.Available() {
		return 0
	}
	return row.kind
}

func (row WTOEvent) RegionID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.region
}

func (row WTOEvent) PointID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.point
}
