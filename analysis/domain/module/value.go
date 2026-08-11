package module

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// Schema is the one immutable Module Factor family for one sealed Link. Its
// finite key table owns every coordinate's admitted subject support. Values
// carry only this family identity and are therefore comparable across keys.
type Schema struct{ owner *schema }

type schema struct {
	source            *link.Link
	linkID            keyspace.ContentID
	keys              []keySupport
	keyByCoordinateID map[keyspace.ContentID]uint32
}

type keySupport struct {
	coordinate linkmodule.ModuleCoordinate
	id         keyspace.ContentID
	pending    []linkmodule.ModuleInitGeneration
	ready      []readySupport
}

// readySupport retains the one Link-proved init-site → ready-subject relation.
// A coordinate-wide subject pool would let one generation publish a result
// owned by another generation at the same coordinate.
type readySupport struct {
	site     linkmodule.ModuleInitGeneration
	subjects []linkmodule.ModuleReadySubject
}

// pendingSite retains an existing Link module-init site together with the
// one shared materialization role. The site is structural support, not a
// runtime-generated identity or a second Module key.
type pendingSite struct {
	site linkmodule.ModuleInitGeneration
	role materialization.Role
}

type readySite struct {
	site    linkmodule.ModuleInitGeneration
	subject linkmodule.ModuleReadySubject
}

// NewSchema derives the complete cache-state support from Link's native
// ModuleInit sites. No caller can introduce a Pending generation or Ready
// subject: Link alone correlates import/cache ingress, transported coordinate,
// destination entry, and successful completion result.
func NewSchema(source *link.Link) (Schema, bool) {
	if source == nil || !source.ContentID().Available() {
		return Schema{}, false
	}
	coordinates := source.Module().Coordinates()
	generations := source.Module().Generations()
	outcomes := source.Module().Outcomes()
	owner := &schema{
		source:            source,
		linkID:            source.ContentID(),
		keys:              make([]keySupport, coordinates.Count()),
		keyByCoordinateID: make(map[keyspace.ContentID]uint32, coordinates.Count()),
	}
	for index := range owner.keys {
		coordinate, ok := coordinates.At(index)
		if !ok {
			return Schema{}, false
		}
		id, ok := coordinates.ID(coordinate)
		if !ok || !id.Available() || uint64(index) > uint64(^uint32(0)) {
			return Schema{}, false
		}
		if _, duplicate := owner.keyByCoordinateID[id]; duplicate {
			return Schema{}, false
		}
		owner.keys[index] = keySupport{coordinate: coordinate, id: id}
		owner.keyByCoordinateID[id] = uint32(index)
	}
	for generationIndex := 0; generationIndex < generations.Count(); generationIndex++ {
		generation, generationOK := generations.At(generationIndex)
		_, target, _, _, entryOK := generations.Entry(generation)
		targetID, targetIDOK := coordinates.ID(target)
		keyIndex, targetOK := owner.keyByCoordinateID[targetID]
		if !generationOK || !entryOK || !targetIDOK || !targetOK || uint64(keyIndex) >= uint64(len(owner.keys)) {
			return Schema{}, false
		}
		support := &owner.keys[keyIndex]
		support.pending = append(support.pending, generation)
		outcomeCount := outcomes.Count(generation)
		readySubjects := make([]linkmodule.ModuleReadySubject, 0, outcomeCount)
		for outcomeIndex := 0; outcomeIndex < outcomeCount; outcomeIndex++ {
			outcome, outcomeOK := outcomes.At(generation, outcomeIndex)
			subject, ready := outcomes.ReadySubject(outcome)
			if !outcomeOK {
				return Schema{}, false
			}
			if ready {
				readySubjects = append(readySubjects, subject)
			}
		}
		if readySubjects, readyOK := normalizedReadySubjects(source, readySubjects); !readyOK {
			return Schema{}, false
		} else if len(readySubjects) != 0 {
			support.ready = append(support.ready, readySupport{site: generation, subjects: readySubjects})
		}
	}
	for index := range owner.keys {
		support := &owner.keys[index]
		var normalized bool
		if support.pending, normalized = normalizedGenerations(source, support.pending); !normalized {
			return Schema{}, false
		}
		if support.ready, normalized = normalizedReadySupport(source, support.ready); !normalized {
			return Schema{}, false
		}
	}
	return Schema{owner: owner}, true
}

// Valid confirms that this schema still denotes the sealed Link authority from
// which its finite cache coordinate universe was derived.  In particular, an
// empty coordinate universe is valid: callers must not recover authority by
// looking at a first Key.
func (schema Schema) Valid() bool {
	return schema.owner != nil && schema.owner.source != nil && schema.owner.linkID.Available() &&
		schema.owner.source.ContentID() == schema.owner.linkID
}

// LinkContentID returns the sealed Link authority for this whole Schema.  It
// remains available when the Link has no Module cache coordinates, so cold
// Rule declarations never need to infer ownership from KeyAt(0).
func (schema Schema) LinkContentID() (keyspace.ContentID, bool) {
	if !schema.Valid() {
		return keyspace.ContentID{}, false
	}
	return schema.owner.linkID, true
}

// Link returns Module's exact immutable structural authority.  ContentID is
// the portable replay identity; this live pointer is the construction fence
// that prevents a same-content independently sealed Link from supplying
// coordinates to this schema.
func (schema Schema) Link() *link.Link {
	if !schema.Valid() {
		return nil
	}
	return schema.owner.source
}

func (schema Schema) KeyCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.owner.keys)
}

func (schema Schema) KeyAt(index int) (Key, bool) {
	if !schema.Valid() || index < 0 || index >= len(schema.owner.keys) {
		return Key{}, false
	}
	return Key{owner: schema.owner, index: uint32(index)}, true
}

func (schema Schema) KeyForCoordinate(coordinate linkmodule.ModuleCoordinate) (Key, bool) {
	if !schema.Valid() {
		return Key{}, false
	}
	id, ok := schema.owner.source.Module().Coordinates().ID(coordinate)
	if !ok {
		return Key{}, false
	}
	index, ok := schema.owner.keyByCoordinateID[id]
	if !ok || uint64(index) >= uint64(len(schema.owner.keys)) {
		return Key{}, false
	}
	return Key{owner: schema.owner, index: index}, true
}

// Value is the normalized may-relation Cold | Pending(site, role) |
// Ready(site, subject). It contains the family owner only—never a Key.
// Top is interpreted against finite support of the coordinate where it is read.
type Value struct {
	owner   *schema
	top     bool
	cold    bool
	pending []pendingSite
	ready   []readySite
}

// Default is the one constant sparse Factor default: {Cold}.
func (schema Schema) Default() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner, cold: true}, true
}

func (schema Schema) Bottom() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner}, true
}

func (schema Schema) Top() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner, top: true}, true
}

// Pending is the sole fresh source admission constructor. A ModuleInit cache
// Rule may create only the exact fresh Recent reference supplied by its common
// predecessor transaction. Ready cannot be constructed directly.
func (schema Schema) Pending(key Key, site linkmodule.ModuleInitGeneration, role materialization.Role) (Value, bool) {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner || role != materialization.Recent || !containsGeneration(schema.owner.source, support.pending, site) {
		return Value{}, false
	}
	return Value{owner: schema.owner, pending: []pendingSite{{site: site, role: role}}}, true
}

// ReplacePending replaces one exact external Suspension reference with another
// already-validated reference. It makes no materialization, liveness, or
// consumption decision: the production Rule must read the matching committed
// Suspension observation before invoking it.
func (schema Schema) ReplacePending(current Value, key Key, oldSite linkmodule.ModuleInitGeneration, oldRole materialization.Role, newSite linkmodule.ModuleInitGeneration, newRole materialization.Role) (Value, bool) {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner || !schema.owns(current) || !schema.Admits(key, current) || current.top || !pendingRoleValid(oldRole) || !pendingRoleValid(newRole) || !containsGeneration(schema.owner.source, support.pending, newSite) {
		return Value{}, false
	}
	pending := append([]pendingSite(nil), current.pending...)
	replaced := false
	for index := range pending {
		if pending[index].site == oldSite && pending[index].role == oldRole {
			pending[index] = pendingSite{site: newSite, role: newRole}
			replaced = true
			break
		}
	}
	if !replaced {
		return Value{}, false
	}
	pending, pendingOK := normalizedPending(schema.owner.source, pending)
	if !pendingOK {
		return Value{}, false
	}
	return Value{owner: schema.owner, cold: current.cold, pending: pending, ready: append([]readySite(nil), current.ready...)}, true
}

// PublishReady replaces only the exact pending reference selected by the
// caller with its Link-sealed ready subject. It does not prove that the
// referenced Suspension generation is live; the production Rule must already
// have read that committed matching observation.
func (schema Schema) PublishReady(current Value, key Key, site linkmodule.ModuleInitGeneration, role materialization.Role, subject linkmodule.ModuleReadySubject) (Value, bool) {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner || !schema.owns(current) || !schema.Admits(key, current) || current.top || !pendingRoleValid(role) || !containsPending(current.pending, site, role) || !containsReadyForSite(schema.owner.source, support.ready, site, subject) {
		return Value{}, false
	}
	pending := make([]pendingSite, 0, len(current.pending)-1)
	removed := false
	for _, item := range current.pending {
		if !removed && item.site == site && item.role == role {
			removed = true
			continue
		}
		pending = append(pending, item)
	}
	ready, readyOK := unionReadySites(schema.owner.source, current.ready, []readySite{{site: site, subject: subject}})
	if !removed || !readyOK {
		return Value{}, false
	}
	return Value{owner: schema.owner, cold: current.cold, pending: pending, ready: ready}, true
}

func (value Value) Valid() bool   { return value.owner != nil }
func (value Value) HasCold() bool { return value.Valid() && (value.top || value.cold) }
func (value Value) IsBottom() bool {
	return value.Valid() && !value.top && !value.cold && len(value.pending) == 0 && len(value.ready) == 0
}
func (value Value) IsTop() bool { return value.Valid() && value.top }

func (value Value) PendingCount() int {
	if !value.Valid() || value.top {
		return 0
	}
	return len(value.pending)
}
func (value Value) PendingAt(index int) (linkmodule.ModuleInitGeneration, materialization.Role, bool) {
	if !value.Valid() || value.top || index < 0 || index >= len(value.pending) {
		return linkmodule.ModuleInitGeneration{}, materialization.Invalid, false
	}
	item := value.pending[index]
	return item.site, item.role, true
}
func (value Value) ReadyCount() int {
	if !value.Valid() || value.top {
		return 0
	}
	return len(value.ready)
}
func (value Value) ReadyAt(index int) (linkmodule.ModuleInitGeneration, linkmodule.ModuleReadySubject, bool) {
	if !value.Valid() || value.top || index < 0 || index >= len(value.ready) {
		return linkmodule.ModuleInitGeneration{}, linkmodule.ModuleReadySubject{}, false
	}
	item := value.ready[index]
	return item.site, item.subject, true
}

func (schema Schema) owns(value Value) bool { return schema.Valid() && value.owner == schema.owner }

func normalizedGenerations(source *link.Link, values []linkmodule.ModuleInitGeneration) ([]linkmodule.ModuleInitGeneration, bool) {
	if source == nil || len(values) == 0 {
		return nil, source != nil
	}
	out := append([]linkmodule.ModuleInitGeneration(nil), values...)
	sort.Slice(out, func(left, right int) bool {
		order, ok := source.Module().Generations().Compare(out[left], out[right])
		return ok && order < 0
	})
	for index := range out {
		if _, ok := source.Module().Generations().ID(out[index]); !ok {
			return nil, false
		}
	}
	return uniqueGenerations(out), true
}

func pendingRoleValid(role materialization.Role) bool {
	return role == materialization.Recent || role == materialization.Summary
}

func normalizedPending(source *link.Link, values []pendingSite) ([]pendingSite, bool) {
	if source == nil || len(values) == 0 {
		return nil, source != nil
	}
	out := append([]pendingSite(nil), values...)
	sort.Slice(out, func(left, right int) bool {
		return lessPending(source, out[left], out[right])
	})
	for _, item := range out {
		if !pendingRoleValid(item.role) {
			return nil, false
		}
		if _, ok := source.Module().Generations().ID(item.site); !ok {
			return nil, false
		}
	}
	end := 0
	for _, item := range out {
		if end == 0 || out[end-1] != item {
			out[end] = item
			end++
		}
	}
	return out[:end], true
}

func lessPending(source *link.Link, left, right pendingSite) bool {
	order, ok := source.Module().Generations().Compare(left.site, right.site)
	if !ok {
		return false
	}
	return order < 0 || order == 0 && left.role < right.role
}

func normalizedReadySubjects(source *link.Link, values []linkmodule.ModuleReadySubject) ([]linkmodule.ModuleReadySubject, bool) {
	if source == nil || len(values) == 0 {
		return nil, source != nil
	}
	out := append([]linkmodule.ModuleReadySubject(nil), values...)
	sort.Slice(out, func(left, right int) bool {
		order, ok := source.Module().ReadySubjects().Compare(out[left], out[right])
		return ok && order < 0
	})
	for index := range out {
		if _, ok := source.Module().ReadySubjects().Compare(out[index], out[index]); !ok {
			return nil, false
		}
	}
	return uniqueReadySubjects(out), true
}

func normalizedReadySupport(source *link.Link, values []readySupport) ([]readySupport, bool) {
	if source == nil || len(values) == 0 {
		return nil, source != nil
	}
	out := append([]readySupport(nil), values...)
	for index := range out {
		subjects, ok := normalizedReadySubjects(source, out[index].subjects)
		if !ok || len(subjects) == 0 {
			return nil, false
		}
		out[index].subjects = subjects
	}
	sort.Slice(out, func(left, right int) bool {
		order, ok := source.Module().Generations().Compare(out[left].site, out[right].site)
		return ok && order < 0
	})
	for index := range out {
		if _, ok := source.Module().Generations().ID(out[index].site); !ok || index != 0 && out[index-1].site == out[index].site {
			return nil, false
		}
	}
	return out, true
}

func uniqueGenerations(values []linkmodule.ModuleInitGeneration) []linkmodule.ModuleInitGeneration {
	end := 0
	for _, value := range values {
		if end == 0 || values[end-1] != value {
			values[end] = value
			end++
		}
	}
	return values[:end]
}

func uniqueReadySubjects(values []linkmodule.ModuleReadySubject) []linkmodule.ModuleReadySubject {
	end := 0
	for _, value := range values {
		if end == 0 || values[end-1] != value {
			values[end] = value
			end++
		}
	}
	return values[:end]
}

func containsGeneration(source *link.Link, values []linkmodule.ModuleInitGeneration, target linkmodule.ModuleInitGeneration) bool {
	index := sort.Search(len(values), func(index int) bool {
		order, ok := source.Module().Generations().Compare(values[index], target)
		return ok && order >= 0
	})
	return index < len(values) && values[index] == target
}

func containsReadySubject(source *link.Link, values []linkmodule.ModuleReadySubject, target linkmodule.ModuleReadySubject) bool {
	index := sort.Search(len(values), func(index int) bool {
		order, ok := source.Module().ReadySubjects().Compare(values[index], target)
		return ok && order >= 0
	})
	return index < len(values) && values[index] == target
}

func containsReadyForSite(source *link.Link, support []readySupport, site linkmodule.ModuleInitGeneration, subject linkmodule.ModuleReadySubject) bool {
	index := sort.Search(len(support), func(index int) bool {
		order, ok := source.Module().Generations().Compare(support[index].site, site)
		return ok && order >= 0
	})
	return index < len(support) && support[index].site == site && containsReadySubject(source, support[index].subjects, subject)
}

func containsPending(values []pendingSite, site linkmodule.ModuleInitGeneration, role materialization.Role) bool {
	for _, value := range values {
		if value.site == site && value.role == role {
			return true
		}
	}
	return false
}

func lessReadySite(source *link.Link, left, right readySite) bool {
	order, ok := source.Module().Generations().Compare(left.site, right.site)
	if !ok {
		return false
	}
	if order != 0 {
		return order < 0
	}
	order, ok = source.Module().ReadySubjects().Compare(left.subject, right.subject)
	return ok && order < 0
}

func unionReadySites(source *link.Link, left, right []readySite) ([]readySite, bool) {
	out := make([]readySite, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		if rightIndex == len(right) || leftIndex < len(left) && lessReadySite(source, left[leftIndex], right[rightIndex]) {
			out = append(out, left[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left) || lessReadySite(source, right[rightIndex], left[leftIndex]) {
			out = append(out, right[rightIndex])
			rightIndex++
			continue
		}
		out = append(out, left[leftIndex])
		leftIndex++
		rightIndex++
	}
	return out, true
}
