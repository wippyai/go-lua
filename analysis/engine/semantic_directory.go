package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// semanticDirectoryEntry is the one address a published ContentID owns: the
// role that issued it and the dense slot holding that role's row locator.
// One ContentID carries exactly one entry, so a published identity can never
// resolve to two locators.
type semanticDirectoryEntry struct {
	kind bindingSemanticRowKind
	slot uint32
}

// semanticDirectoryMetrics are the construction counters the I3 cost laws are
// measured against. They record what one sealed construction installed and
// reserved; they are never updated after seal, so a retained generation adds
// nothing to them.
type semanticDirectoryMetrics struct {
	entries    int
	operations int
	capacity   int
}

// semanticDirectory is the sealed ContentID directory of one BindingTopology.
// It is produced by exactly one constructor, is immutable after seal, and is
// shared unchanged by every graph revision of its topology: a revision
// replaces the root bindings a locator resolves against, never the directory
// entries themselves.
type semanticDirectory struct {
	topology    *equation.Topology
	state       *schemaBindingState
	authority   *schemaBindingAuthority
	entries     map[identity.ContentID]semanticDirectoryEntry
	points      []equation.PointRowLocator
	members     []equation.RuleMemberRowLocator
	queries     []equation.QueryRowLocator
	activations []equation.ActivationMemberRowLocator
	queryOrder  []identity.ContentID
	metrics     semanticDirectoryMetrics
}

// sealSemanticDirectory is the sole constructor. Row spans come from the
// sealed topology, so entry count is bounded by the admitted root bindings
// rather than by the construction input, and every duplicate ContentID or
// duplicate row slot rejects the seal.
func sealSemanticDirectory(topology *equation.Topology, state *schemaBindingState, authority *schemaBindingAuthority, rows *bindingSemanticRows) (*semanticDirectory, bool) {
	if topology == nil || state == nil || authority == nil || rows == nil || state.phase != schemaBindingSealed || state.authority != authority {
		return nil, false
	}
	pointRows, memberRows, queryRows := topology.PointRowCount(), topology.RuleMemberRowCount(), topology.QueryRowCount()
	total := len(rows.points) + len(rows.members) + len(rows.queries) + len(rows.activations)
	if len(rows.points) > pointRows || len(rows.members) > memberRows || len(rows.queries) != queryRows || len(rows.activations) > memberRows {
		return nil, false
	}
	activationRows := 0
	if len(rows.activations) != 0 {
		activationRows = memberRows
	}
	result := &semanticDirectory{
		topology: topology, state: state, authority: authority,
		entries:     make(map[identity.ContentID]semanticDirectoryEntry, total),
		points:      make([]equation.PointRowLocator, pointRows),
		members:     make([]equation.RuleMemberRowLocator, memberRows),
		queries:     make([]equation.QueryRowLocator, queryRows),
		activations: make([]equation.ActivationMemberRowLocator, activationRows),
		queryOrder:  make([]identity.ContentID, queryRows),
	}
	result.metrics.capacity = total + pointRows + memberRows + activationRows + 2*queryRows
	for id, ref := range rows.points {
		locator, ok := topology.PointRow(ref)
		slot := int(uint64(ref)) - 1
		if !ok || slot < 0 || slot >= pointRows || result.points[slot] != (equation.PointRowLocator{}) || !result.claim(id, bindingSemanticPoint, slot) {
			return nil, false
		}
		result.points[slot] = locator
	}
	for id, ref := range rows.members {
		locator, ok := topology.RuleMemberRow(ref)
		slot := int(uint64(ref)) - 1
		if !ok || slot < 0 || slot >= memberRows || result.members[slot] != (equation.RuleMemberRowLocator{}) || !result.claim(id, bindingSemanticMember, slot) {
			return nil, false
		}
		result.members[slot] = locator
	}
	for id, ordinal := range rows.queries {
		locator, ok := topology.QueryRow(ordinal)
		if !ok || ordinal >= uint64(queryRows) || result.queryOrder[ordinal].Available() || !result.claim(id, bindingSemanticQuery, int(ordinal)) {
			return nil, false
		}
		result.queries[ordinal] = locator
		result.queryOrder[ordinal] = id
	}
	for id, ref := range rows.activations {
		locator, ok := topology.ActivationMemberRow(ref)
		slot := int(uint64(ref)) - 1
		if !ok || slot < 0 || slot >= activationRows || result.activations[slot] != (equation.ActivationMemberRowLocator{}) || !result.claim(id, bindingSemanticActivation, slot) {
			return nil, false
		}
		result.activations[slot] = locator
	}
	for _, id := range result.queryOrder {
		result.metrics.operations++
		if !id.Available() {
			return nil, false
		}
	}
	return result, true
}

// claim installs the single entry a ContentID owns. A ContentID already
// carrying an entry is rejected, so no identity reaches two role locators.
func (directory *semanticDirectory) claim(id identity.ContentID, kind bindingSemanticRowKind, slot int) bool {
	directory.metrics.operations++
	if !id.Available() || slot < 0 || slot > int(^uint32(0)) {
		return false
	}
	if _, duplicate := directory.entries[id]; duplicate {
		return false
	}
	directory.entries[id] = semanticDirectoryEntry{kind: kind, slot: uint32(slot)}
	directory.metrics.entries++
	return true
}

// ownedBy reports that this directory addresses exactly the sealed topology
// and Binding authority that constructed it.
func (directory *semanticDirectory) ownedBy(topology *equation.Topology, state *schemaBindingState, authority *schemaBindingAuthority) bool {
	return directory != nil && directory.topology == topology && directory.state == state && directory.authority == authority && directory.entries != nil
}

// resolve returns the one entry published under id.
func (directory *semanticDirectory) resolve(id identity.ContentID) (semanticDirectoryEntry, bool) {
	if directory == nil || !id.Available() {
		return semanticDirectoryEntry{}, false
	}
	entry, found := directory.entries[id]
	return entry, found
}

func (directory *semanticDirectory) point(id identity.ContentID) (equation.PointRowLocator, bool) {
	entry, found := directory.resolve(id)
	if !found || entry.kind != bindingSemanticPoint || int(entry.slot) >= len(directory.points) {
		return equation.PointRowLocator{}, false
	}
	return directory.points[entry.slot], true
}

func (directory *semanticDirectory) member(id identity.ContentID) (equation.RuleMemberRowLocator, bool) {
	entry, found := directory.resolve(id)
	if !found || entry.kind != bindingSemanticMember || int(entry.slot) >= len(directory.members) {
		return equation.RuleMemberRowLocator{}, false
	}
	return directory.members[entry.slot], true
}

func (directory *semanticDirectory) query(id identity.ContentID) (equation.QueryRowLocator, bool) {
	entry, found := directory.resolve(id)
	if !found || entry.kind != bindingSemanticQuery || int(entry.slot) >= len(directory.queries) {
		return equation.QueryRowLocator{}, false
	}
	return directory.queries[entry.slot], true
}

func (directory *semanticDirectory) activation(id identity.ContentID) (equation.ActivationMemberRowLocator, bool) {
	entry, found := directory.resolve(id)
	if !found || entry.kind != bindingSemanticActivation || int(entry.slot) >= len(directory.activations) {
		return equation.ActivationMemberRowLocator{}, false
	}
	return directory.activations[entry.slot], true
}
