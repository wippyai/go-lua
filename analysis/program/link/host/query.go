package host

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// Each view is a non-embedded capability.  The Component itself only issues
// these views and exposes lifecycle/content facts; no consumer can reach an
// unrelated Host relation through a god-object method set.
type Capabilities struct{ component *Component }
type CapabilitySeeds struct{ component *Component }
type Exposures struct{ component *Component }
type Members struct{ component *Component }
type BootRoots struct{ component *Component }
type Attachments struct{ component *Component }
type Globals struct{ component *Component }

func (c *Component) Capabilities() Capabilities       { return Capabilities{component: c} }
func (c *Component) CapabilitySeeds() CapabilitySeeds { return CapabilitySeeds{component: c} }
func (c *Component) Exposures() Exposures             { return Exposures{component: c} }
func (c *Component) Members() Members                 { return Members{component: c} }
func (c *Component) BootRoots() BootRoots             { return BootRoots{component: c} }
func (c *Component) Attachments() Attachments         { return Attachments{component: c} }
func (c *Component) Globals() Globals                 { return Globals{component: c} }

func live(c *Component) bool { return c != nil && c.authority != nil && c.authority.component == c }
func validCap(c *Component, x ProviderCapability) bool {
	return live(c) && x.component == c && x.ordinal != 0 && int(x.ordinal) <= len(c.authority.capabilities)
}
func validSeed(c *Component, x ProviderCapabilitySeed) bool {
	return live(c) && x.component == c && x.ordinal != 0 && int(x.ordinal) <= len(c.authority.seeds)
}
func validBoot(c *Component, x BootRoot) bool {
	return live(c) && x.component == c && x.ordinal != 0 && x.ordinal <= c.authority.bootRoots
}
func validAttachment(c *Component, x BootMetatableAttachment) bool {
	return live(c) && x.component == c && x.ordinal != 0 && int(x.ordinal) <= len(c.authority.attachments)
}
func validGlobalHandle(c *Component, x GlobalBinding) bool {
	return live(c) && x.component == c && x.ordinal != 0 && int(x.ordinal) <= len(c.authority.globals)
}

func (v Capabilities) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.capabilities)
}
func (v Capabilities) At(i int) (ProviderCapability, bool) {
	c := v.component
	if !live(c) || i < 0 || i >= len(c.authority.capabilities) {
		return ProviderCapability{}, false
	}
	return ProviderCapability{c, uint32(i + 1)}, true
}
func (v Capabilities) ID(x ProviderCapability) (identity.ContentID, bool) {
	c := v.component
	if !validCap(c, x) {
		return identity.ContentID{}, false
	}
	var p [48]byte
	id := c.ContentID()
	copy(p[:32], id[:])
	binary.BigEndian.PutUint64(p[32:40], 0x686f73742d636170)
	binary.BigEndian.PutUint64(p[40:48], uint64(x.ordinal))
	return sha256.Sum256(p[:]), true
}
func (v CapabilitySeeds) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.activeSeeds)
}
func (v CapabilitySeeds) At(index int) (ProviderCapabilitySeed, bool) {
	c := v.component
	if !live(c) || index < 0 || index >= len(c.authority.activeSeeds) {
		return ProviderCapabilitySeed{}, false
	}
	return ProviderCapabilitySeed{component: c, ordinal: c.authority.activeSeeds[index]}, true
}
func (v CapabilitySeeds) Ref(x ProviderCapabilitySeed) (ProviderCapabilitySeedRef, bool) {
	c := v.component
	if !validSeed(c, x) || !seedActive(c, c.authority.seeds[x.ordinal-1]) {
		return ProviderCapabilitySeedRef{}, false
	}
	return ProviderCapabilitySeedRef{hostID: c.ContentID(), ordinal: x.ordinal}, true
}
func (v CapabilitySeeds) Find(ref ProviderCapabilitySeedRef) (ProviderCapabilitySeed, bool) {
	c := v.component
	if !live(c) || !c.ContentID().Available() || ref.hostID != c.ContentID() || ref.ordinal == 0 || int(ref.ordinal) > len(c.authority.seeds) {
		return ProviderCapabilitySeed{}, false
	}
	x := ProviderCapabilitySeed{component: c, ordinal: ref.ordinal}
	if !seedActive(c, c.authority.seeds[x.ordinal-1]) {
		return ProviderCapabilitySeed{}, false
	}
	return x, true
}
func (v CapabilitySeeds) ID(x ProviderCapabilitySeed) (identity.ContentID, bool) {
	ref, ok := v.Ref(x)
	if !ok {
		return identity.ContentID{}, false
	}
	var p [56]byte
	copy(p[:32], ref.hostID[:])
	binary.BigEndian.PutUint64(p[32:40], 0x686f73742d637073) // host-cps
	binary.BigEndian.PutUint64(p[40:48], 1)
	binary.BigEndian.PutUint64(p[48:56], uint64(ref.ordinal))
	return sha256.Sum256(p[:]), true
}
func (v CapabilitySeeds) Capability(x ProviderCapabilitySeed) (ProviderCapability, bool) {
	c := v.component
	if !validSeed(c, x) {
		return ProviderCapability{}, false
	}
	r := c.authority.seeds[x.ordinal-1]
	if !seedActive(c, r) {
		return ProviderCapability{}, false
	}
	return ProviderCapability{c, r.capability}, true
}
func (v CapabilitySeeds) Source(x ProviderCapabilitySeed) (ProviderCapabilitySource, bool) {
	c := v.component
	if !validSeed(c, x) {
		return ProviderCapabilitySourceInvalid, false
	}
	r := c.authority.seeds[x.ordinal-1]
	return r.source, seedActive(c, r)
}
func (v CapabilitySeeds) InitialRoot(x ProviderCapabilitySeed) (vocabulary.InitialRoot, bool) {
	c := v.component
	if !validSeed(c, x) {
		return 0, false
	}
	r := c.authority.seeds[x.ordinal-1]
	return r.root, r.source == ProviderCapabilitySourceInitialRoot && seedActive(c, r)
}
func (v CapabilitySeeds) ABIInput(x ProviderCapabilitySeed) (vocabulary.Operation, vocabulary.ValueFormal, bool) {
	c := v.component
	if !validSeed(c, x) {
		return 0, 0, false
	}
	r := c.authority.seeds[x.ordinal-1]
	return r.operation, r.formal, r.source == ProviderCapabilitySourceABIInput && seedActive(c, r)
}
func (v CapabilitySeeds) Result(x ProviderCapabilitySeed) (vocabulary.Operation, uint32, uint32, bool) {
	c := v.component
	if !validSeed(c, x) {
		return 0, 0, 0, false
	}
	r := c.authority.seeds[x.ordinal-1]
	return r.operation, r.outcome, r.result, r.source == ProviderCapabilitySourceResult && seedActive(c, r)
}
func (v CapabilitySeeds) Exposure(x ProviderCapabilitySeed) (linkboundary.Value, bool) {
	c := v.component
	if !validSeed(c, x) {
		return linkboundary.Value{}, false
	}
	r := c.authority.seeds[x.ordinal-1]
	return r.value, r.source == ProviderCapabilitySourceExposure && seedActive(c, r)
}
func seedActive(c *Component, r capabilitySeedRow) bool {
	if !live(c) || r.capability == 0 || int(r.capability) > len(c.authority.capabilities) {
		return false
	}
	if r.source != ProviderCapabilitySourceExposure {
		return r.source == ProviderCapabilitySourceInitialRoot || r.source == ProviderCapabilitySourceABIInput || r.source == ProviderCapabilitySourceResult
	}
	shard, access, ok := c.authority.boundary.Values().Origin(r.value)
	if !ok {
		return false
	}
	p, ok := c.authority.project.Mounts().Program(shard)
	return ok && p.Flow().Executable().Contains(access) && hostAccess(p, access, false)
}

func (v Exposures) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.exposures)
}
func (v Exposures) At(i int) (linkproject.Shard, keyspace.Term, linkboundary.Value, linkboundary.Endpoint, HostDispatch, bool) {
	c := v.component
	if !live(c) || i < 0 || i >= len(c.authority.exposures) {
		return linkproject.Shard{}, 0, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	r := c.authority.exposures[i]
	if !selectorValid(c, r, false) {
		return linkproject.Shard{}, 0, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	return r.shard, r.access, r.output, r.endpoint, r.dispatch, true
}
func (v Members) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.members)
}
func (v Members) At(i int) (linkproject.Shard, keyspace.Term, ProviderCapability, linkproject.Key, linkboundary.Value, linkboundary.Endpoint, HostDispatch, bool) {
	c := v.component
	if !live(c) || i < 0 || i >= len(c.authority.members) {
		return linkproject.Shard{}, 0, ProviderCapability{}, linkproject.Key{}, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	r := c.authority.members[i]
	if !selectorValid(c, r, true) {
		return linkproject.Shard{}, 0, ProviderCapability{}, linkproject.Key{}, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	return r.shard, r.access, ProviderCapability{c, r.capability}, r.key, r.output, r.endpoint, r.dispatch, true
}
func selectorValid(c *Component, r selectorRow, member bool) bool {
	if !live(c) || r.access == 0 || r.dispatch != HostDispatchLookup {
		return false
	}
	p, ok := c.authority.project.Mounts().Program(r.shard)
	if !ok || !p.Flow().Executable().Contains(r.access) || !hostAccess(p, r.access, member) {
		return false
	}
	if _, _, ok := c.authority.boundary.Values().Origin(r.output); !ok {
		return false
	}
	if _, ok := c.authority.boundary.Endpoints().Operation(r.endpoint); !ok {
		return false
	}
	return (!member && r.capability == 0) || (member && r.capability != 0 && int(r.capability) <= len(c.authority.capabilities))
}
func (v Exposures) EndpointCount(s linkproject.Shard, access keyspace.Term) int {
	c := v.component
	if !live(c) {
		return 0
	}
	return selectorCount(c, c.authority.exposures, s, access, false)
}
func (v Exposures) EndpointAt(s linkproject.Shard, access keyspace.Term, index int) (linkboundary.Endpoint, bool) {
	c := v.component
	if !live(c) {
		return linkboundary.Endpoint{}, false
	}
	r, ok := selectorAt(c, c.authority.exposures, s, access, index, false)
	return r.endpoint, ok
}
func (v Exposures) SelectorAt(s linkproject.Shard, access keyspace.Term, index int) (linkboundary.Value, linkboundary.Endpoint, HostDispatch, bool) {
	c := v.component
	if !live(c) {
		return linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	r, ok := selectorAt(c, c.authority.exposures, s, access, index, false)
	if !ok {
		return linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	return r.output, r.endpoint, r.dispatch, true
}
func (v Members) EndpointCount(s linkproject.Shard, access keyspace.Term) int {
	c := v.component
	if !live(c) {
		return 0
	}
	return selectorCount(c, c.authority.members, s, access, true)
}
func (v Members) EndpointAt(s linkproject.Shard, access keyspace.Term, index int) (linkboundary.Endpoint, bool) {
	c := v.component
	if !live(c) {
		return linkboundary.Endpoint{}, false
	}
	r, ok := selectorAt(c, c.authority.members, s, access, index, true)
	return r.endpoint, ok
}
func (v Members) SelectorAt(s linkproject.Shard, access keyspace.Term, index int) (ProviderCapability, linkproject.Key, linkboundary.Value, linkboundary.Endpoint, HostDispatch, bool) {
	c := v.component
	if !live(c) {
		return ProviderCapability{}, linkproject.Key{}, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	r, ok := selectorAt(c, c.authority.members, s, access, index, true)
	if !ok {
		return ProviderCapability{}, linkproject.Key{}, linkboundary.Value{}, linkboundary.Endpoint{}, HostDispatchInvalid, false
	}
	return ProviderCapability{c, r.capability}, r.key, r.output, r.endpoint, r.dispatch, true
}
func selectorCount(c *Component, rows []selectorRow, s linkproject.Shard, access keyspace.Term, member bool) int {
	if !live(c) || access == 0 {
		return 0
	}
	start := sort.Search(len(rows), func(i int) bool { return selectorKeyAtLeast(c, rows[i], s, access) })
	n := 0
	for i := start; i < len(rows) && rows[i].shard == s && rows[i].access == access; i++ {
		if selectorValid(c, rows[i], member) {
			n++
		}
	}
	return n
}
func selectorAt(c *Component, rows []selectorRow, s linkproject.Shard, access keyspace.Term, index int, member bool) (selectorRow, bool) {
	if index < 0 {
		return selectorRow{}, false
	}
	start := sort.Search(len(rows), func(i int) bool { return selectorKeyAtLeast(c, rows[i], s, access) })
	for i := start; i < len(rows) && rows[i].shard == s && rows[i].access == access; i++ {
		if selectorValid(c, rows[i], member) {
			if index == 0 {
				return rows[i], true
			}
			index--
		}
	}
	return selectorRow{}, false
}
func selectorKeyAtLeast(c *Component, r selectorRow, s linkproject.Shard, access keyspace.Term) bool {
	ri, rok := c.authority.project.Mounts().Index(r.shard)
	si, sok := c.authority.project.Mounts().Index(s)
	if !rok || !sok {
		return false
	}
	return ri > si || (ri == si && r.access >= access)
}

func (v BootRoots) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return int(c.authority.bootRoots)
}
func (v BootRoots) At(i int) (BootRoot, bool) {
	c := v.component
	if !live(c) || i < 0 || uint32(i) >= c.authority.bootRoots {
		return BootRoot{}, false
	}
	return BootRoot{c, uint32(i + 1)}, true
}

// ID returns a detached, Host-content-fenced identity for one actor-local
// bootstrap root.  The ordinal remains private to Host and cannot be reused
// across an equivalent reseal.
func (v BootRoots) ID(x BootRoot) (identity.ContentID, bool) {
	c := v.component
	if !validBoot(c, x) || !c.ContentID().Available() {
		return identity.ContentID{}, false
	}
	var payload [48]byte
	id := c.ContentID()
	copy(payload[:32], id[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x686f73742d626f6f) // host-boo
	binary.BigEndian.PutUint64(payload[40:48], uint64(x.ordinal))
	return sha256.Sum256(payload[:]), true
}
func (v BootRoots) Mapping(x BootRoot) (linkmodule.Actor, vocabulary.InitialRoot, bool) {
	c := v.component
	if !validBoot(c, x) {
		return linkmodule.Actor{}, 0, false
	}
	n := c.authority.target.InitialRootCount()
	if n == 0 {
		return linkmodule.Actor{}, 0, false
	}
	actor, ok := c.authority.module.Actors().At(int(x.ordinal-1) / n)
	return actor, vocabulary.InitialRoot((int(x.ordinal-1) % n) + 1), ok
}
func (v BootRoots) For(actor linkmodule.Actor, root vocabulary.InitialRoot) (BootRoot, bool) {
	c := v.component
	if !live(c) {
		return BootRoot{}, false
	}
	return c.authority.bootFor(actor, root)
}
func (v Attachments) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.attachments)
}
func (v Attachments) At(i int) (BootMetatableAttachment, bool) {
	c := v.component
	if !live(c) || i < 0 || i >= len(c.authority.attachments) {
		return BootMetatableAttachment{}, false
	}
	return BootMetatableAttachment{c, uint32(i + 1)}, true
}
func (v Attachments) Mapping(x BootMetatableAttachment) (vocabulary.InitialValueKind, BootRoot, bool) {
	c := v.component
	if !validAttachment(c, x) {
		return vocabulary.InitialValueInvalid, BootRoot{}, false
	}
	r := c.authority.attachments[x.ordinal-1]
	boot := BootRoot{c, r.boot}
	_, root, ok := v.component.BootRoots().Mapping(boot)
	if !ok {
		return vocabulary.InitialValueInvalid, BootRoot{}, false
	}
	shape, ok := c.authority.target.InitialRootBootShape(root)
	aggregate, aok := c.authority.target.BootShapeAggregate(shape)
	if !ok || !aok || aggregate != vocabulary.BootAggregateMetatable {
		return vocabulary.InitialValueInvalid, BootRoot{}, false
	}
	return r.base, boot, true
}
func (v Globals) Count() int {
	c := v.component
	if !live(c) {
		return 0
	}
	return len(c.authority.globals)
}
func (v Globals) At(i int) (GlobalBinding, bool) {
	c := v.component
	if !live(c) || i < 0 || i >= len(c.authority.globals) {
		return GlobalBinding{}, false
	}
	return GlobalBinding{c, uint32(i + 1)}, true
}

// GlobalBindingID is the detached identity of one exact Host-issued global
// row.  Consumers must retain this rather than smuggling the private dense
// ordinal into their own content identities.
func (v Globals) ID(x GlobalBinding) (identity.ContentID, bool) {
	c := v.component
	if !validGlobalHandle(c, x) {
		return identity.ContentID{}, false
	}
	var p [48]byte
	id := c.ContentID()
	copy(p[:32], id[:])
	binary.BigEndian.PutUint64(p[32:40], 0x686f73742d676c6f) // host-glo
	binary.BigEndian.PutUint64(p[40:48], uint64(x.ordinal))
	return sha256.Sum256(p[:]), true
}
func (v Globals) Mapping(x GlobalBinding) (linkmodule.AnalysisRoot, BootRoot, keyspace.Term, keyspace.Key, vocabulary.InitialBindingClass, vocabulary.InitialValue, bool) {
	c := v.component
	if !validGlobalHandle(c, x) {
		return linkmodule.AnalysisRoot{}, BootRoot{}, 0, 0, vocabulary.InitialBindingInvalid, 0, false
	}
	r := c.authority.globals[x.ordinal-1]
	return r.analysis, BootRoot{c, r.boot}, r.cell, r.key, r.class, r.value, true
}
func (v Globals) For(root linkmodule.AnalysisRoot, cell keyspace.Term) (GlobalBinding, bool) {
	c := v.component
	if !live(c) {
		return GlobalBinding{}, false
	}
	i, ok := c.authority.module.Roots().Index(root)
	if !ok || i >= len(c.authority.globalRanges) {
		return GlobalBinding{}, false
	}
	shard, _, _, ok := c.authority.module.Roots().Mapping(root)
	p, pok := c.authority.project.Mounts().Program(shard)
	if !ok || !pok {
		return GlobalBinding{}, false
	}
	kind, body, key, ok := p.Flow().Authored().Storage().Cells().Get(cell)
	if !ok || kind != flow.CellGlobal || body != 0 || key == 0 {
		return GlobalBinding{}, false
	}
	r := c.authority.globalRanges[i]
	at := sort.Search(int(r.end-r.start), func(n int) bool { return c.authority.globals[r.start+uint32(n)].key >= key })
	if uint32(at) >= r.end-r.start {
		return GlobalBinding{}, false
	}
	row := c.authority.globals[r.start+uint32(at)]
	if row.key != key {
		return GlobalBinding{}, false
	}
	return GlobalBinding{c, r.start + uint32(at) + 1}, true
}

// ForProgramCell is the exact inverse for one canonical Program global
// occurrence. It is intentionally owned by Host: callers provide the exact
// Project Shard, the exact mounted Program owner, and authored Cell term,
// while Host retains the only derived lookup from that triple to its sealed
// GlobalBinding. A Shard from an equivalent foreign Project, or an equivalent
// but separately sealed Program, is rejected before the map is consulted.
//
// The inverse is unique by construction.  Host admission rejects a duplicate
// (Shard, Cell) collision, so a false result means that this exact occurrence
// is not part of this Host rather than that a caller should scan the rows.
func (v Globals) ForProgramCell(shard linkproject.Shard, owner *program.Program, cell keyspace.Term) (GlobalBinding, bool) {
	c := v.component
	if !live(c) || owner == nil || cell == 0 {
		return GlobalBinding{}, false
	}
	shardIndex, ok := c.authority.project.Mounts().Index(shard)
	if !ok {
		return GlobalBinding{}, false
	}
	mounted, ok := c.authority.project.Mounts().Program(shard)
	if !ok || mounted != owner {
		return GlobalBinding{}, false
	}
	ordinal, ok := c.authority.globalByShardCell[globalLookupKey{shard: uint32(shardIndex), cell: cell}]
	if !ok || ordinal == 0 || uint64(ordinal) > uint64(len(c.authority.globals)) {
		return GlobalBinding{}, false
	}
	return GlobalBinding{c, ordinal}, true
}
