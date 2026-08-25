package suspension

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/state"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Source is one Value coordinate selected by a mounted liveness subject.
// Coordinate is issued by the exact Value schema handed to the derivation;
// Tag is the one-based position in that schema's canonical coordinate order.
// The row intentionally carries no module or semantic ID: those are cold
// construction inputs, not a second runtime identity authority.
type Source struct {
	coordinate valuedomain.Coordinate
	tag        uint64
}

// Coordinate is the owner-issued Value key projected by this row.
func (source Source) Coordinate() (valuedomain.Coordinate, bool) {
	return source.coordinate, source.coordinate.Valid() && source.tag != 0
}

// Tag is the canonical selected-read correlation tag.
func (source Source) Tag() (uint64, bool) {
	return source.tag, source.coordinate.Valid() && source.tag != 0
}

// SourcePlan is the immutable relation state emitted for one candidate. The
// small inline prefix keeps ordinary Cell/Values subjects allocation-free;
// the explicit suffix is the only representation for a wider aggregate.
type SourcePlan struct {
	inline [8]Source
	extra  []Source
	size   int
}

func (plan SourcePlan) Count() int {
	if plan.size < 0 {
		return 0
	}
	return plan.size
}

func (plan SourcePlan) At(index int) (Source, bool) {
	if index < 0 || index >= plan.size {
		return Source{}, false
	}
	if index < len(plan.inline) {
		return plan.inline[index], true
	}
	index -= len(plan.inline)
	if index < 0 || index >= len(plan.extra) {
		return Source{}, false
	}
	return plan.extra[index], true
}

// SuspensionSourceCount is the direct relation accessor for the sealed
// source plan.  The generated family calls free accessors so it can thread the
// immutable plan without retaining an owner or a callback.
func SuspensionSourceCount(plan SourcePlan) int { return plan.Count() }

// SuspensionSourceAt is the direct relation accessor for one canonical Value
// source row.
func SuspensionSourceAt(plan SourcePlan, index int) (Source, bool) {
	return plan.At(index)
}

func (plan *SourcePlan) add(source Source) bool {
	if plan == nil || !source.coordinate.Valid() || source.tag == 0 || plan.size < 0 {
		return false
	}
	if plan.size < len(plan.inline) {
		plan.inline[plan.size] = source
		plan.size++
		return true
	}
	plan.extra = append(plan.extra, source)
	plan.size++
	return true
}

// DeriveSuspensionSources is the sole Value relation derivation for
// suspension. It follows the subject-liveness kind to its canonical semantic
// values, resolves those values through the owner-issued mounted coordinate
// directory, and orders them by dense Value coordinate. Root and scalar Value
// subjects are routed directly through Heap and therefore publish an empty
// source set; an empty set is a valid closed relation, not an unknown fallback.
func DeriveSuspensionSources(values *valuedomain.Schema, candidate lifecycle.MountedSubjectLiveness) (SourcePlan, bool) {
	if values == nil || !values.Valid() || !candidate.Available() {
		return SourcePlan{}, false
	}
	span := candidate.Span()
	if !span.Available() {
		return SourcePlan{}, false
	}
	ids, idsOK := subjectValueIDs(candidate.State(), span)
	if !idsOK {
		return SourcePlan{}, false
	}
	if len(ids) == 0 {
		return SourcePlan{}, true
	}
	type indexed struct {
		coordinate valuedomain.Coordinate
		index      uint32
	}
	ordered := make([]indexed, len(ids))
	for index, id := range ids {
		if !id.Available() {
			return SourcePlan{}, false
		}
		coordinate, coordinateOK := values.CoordinateForMountedSemantic(candidate.MountID(), id)
		if !coordinateOK || !coordinate.Valid() {
			return SourcePlan{}, false
		}
		dense, denseOK := values.CoordinateIndex(coordinate)
		if !denseOK {
			return SourcePlan{}, false
		}
		ordered[index] = indexed{coordinate: coordinate, index: dense}
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].index < ordered[right].index })
	var plan SourcePlan
	for index, row := range ordered {
		if index > 0 && ordered[index-1].index == row.index {
			return SourcePlan{}, false
		}
		if !plan.add(Source{coordinate: row.coordinate, tag: uint64(index) + 1}) {
			return SourcePlan{}, false
		}
	}
	return plan, true
}

// subjectValueIDs projects the Program-owned semantic subject directory. It
// reads only the immutable State carried by the candidate; no mounted Program
// or downstream catalog is retained in the relation.
func subjectValueIDs(publication state.State, span lifecycle.SubjectLivenessSpan) ([]identity.ContentID, bool) {
	if !publication.Available() || !span.Available() {
		return nil, false
	}
	view, viewOK := lifecycle.NewView(publication)
	if !viewOK {
		return nil, false
	}
	switch span.SubjectKind() {
	case lifecycle.SubjectLivenessRoot:
		return nil, true
	case lifecycle.SubjectLivenessCell:
		if _, lifetimeOK := view.StorageCellLifetimeForID(span.SubjectID()); !lifetimeOK {
			return nil, false
		}
		return []identity.ContentID{span.SubjectID()}, true
	case lifecycle.SubjectLivenessValue:
		return []identity.ContentID{span.SubjectID()}, true
	case lifecycle.SubjectLivenessValues:
		return valuesAggregateIDs(publication, span.SubjectID())
	default:
		return nil, false
	}
}

func valuesAggregateIDs(publication state.State, aggregateID identity.ContentID) ([]identity.ContentID, bool) {
	if !publication.Available() || !aggregateID.Available() {
		return nil, false
	}
	frozen := publication.Frozen()
	catalog, catalogOK := programcatalog.CatalogID(publication.CatalogID())
	if !catalogOK {
		return nil, false
	}
	family := programschema.ValuesFamily()
	count, countOK := family.Count(&frozen, catalog)
	if !countOK || count < 0 {
		return nil, false
	}
	var aggregate programschema.Values
	found := false
	for index := 0; index < count; index++ {
		row, rowOK := family.At(&frozen, catalog, index)
		if !rowOK || !row.Available() || row.ID() != aggregateID {
			continue
		}
		if found {
			return nil, false
		}
		aggregate, found = row, true
	}
	if !found {
		return nil, false
	}
	if _, open := aggregate.Tail(); open {
		return nil, false
	}
	ids := make([]identity.ContentID, 0, aggregate.MemberCount()+1)
	ids = append(ids, aggregate.ID())
	offset, memberCount, spanOK := aggregate.MemberSpan()
	if !spanOK {
		return nil, false
	}
	members := programschema.ValuesMemberFamily()
	for index := 0; index < int(memberCount); index++ {
		row, rowOK := members.At(&frozen, catalog, int(offset)+index)
		if !rowOK || !row.Available() {
			return nil, false
		}
		ids = append(ids, row.ID())
	}
	seen := make(map[identity.ContentID]struct{}, len(ids))
	for _, id := range ids {
		if !id.Available() {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, false
		}
		seen[id] = struct{}{}
	}
	return ids, true
}
