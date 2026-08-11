package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// SourceSeed is one unconditional, result-bearing Link Value admitted by this
// exact Schema. It is a cold Rule operand over the existing Link Value; Value
// introduces neither a source row nor a second source identity.
type SourceSeed struct {
	schema *Schema
	value  linkboundary.Value
}

// SourceSeed admits exactly Program literals and binder-authorized runtime
// TypeValue roots. Contextual boot, endpoint, fresh-result, capability, and
// host-member relations remain available only through their owning relations.
func (schema *Schema) SourceSeed(value linkboundary.Value) (SourceSeed, bool) {
	if schema == nil || schema.source == nil || !schema.unconditionalValue(value) {
		return SourceSeed{}, false
	}
	return SourceSeed{schema: schema, value: value}, true
}

// SourceSeedAt projects Link's canonical Value position directly. An excluded
// position returns false; there is no compacted parallel ordinal.
func (schema *Schema) SourceSeedAt(index int) (SourceSeed, bool) {
	if schema == nil || schema.source == nil {
		return SourceSeed{}, false
	}
	value, ok := schema.source.Boundary().Values().At(index)
	if !ok {
		return SourceSeed{}, false
	}
	return schema.SourceSeed(value)
}

func (schema *Schema) unconditionalValue(value linkboundary.Value) bool {
	if schema == nil || schema.source == nil {
		return false
	}
	if _, ok := schema.SourceValue(value); !ok {
		return false
	}
	if schema.typeRefs[value] != 0 {
		return true
	}
	_, _, ok := schema.sourceLiteral(value)
	return ok
}

func (seed SourceSeed) valid() bool {
	return seed.schema != nil && seed.schema.source != nil && seed.schema.unconditionalValue(seed.value)
}

// ID returns the existing canonical Link Value identity of this source.
func (seed SourceSeed) ID() (keyspace.ContentID, bool) {
	if !seed.valid() {
		return keyspace.ContentID{}, false
	}
	return seed.schema.source.Boundary().Values().ID(seed.value)
}

// Origin returns the existing authored Program occurrence that issued this
// unconditional source.  Link remains the sole owner of the Value-to-source
// relation; Value retains neither a second occurrence row nor an ordinal.
func (seed SourceSeed) Origin() (linkproject.Shard, keyspace.Term, bool) {
	if !seed.valid() {
		return linkproject.Shard{}, 0, false
	}
	return seed.schema.source.Boundary().Values().Origin(seed.value)
}

// Result rederives both the existing Value Factor coordinate and its immutable
// source fact. The fact contains no integer or floating-point payload.
func (seed SourceSeed) Result() (Coordinate, Value, bool) {
	if !seed.valid() {
		return Coordinate{}, Value{}, false
	}
	coordinate, coordinateOK := seed.schema.CoordinateFor(seed.value)
	fact, factOK := seed.schema.SourceValue(seed.value)
	if !coordinateOK || !factOK {
		return Coordinate{}, Value{}, false
	}
	return coordinate, fact, true
}

// ReturnBoundary is Value's direct coordinate for one executable Program
// return. Link supplies the sealed shard topology only: no Link return row,
// identity, or projection mediates this domain operand.
type ReturnBoundary struct {
	schema  *Schema
	shard   linkproject.Shard
	term    keyspace.Term
	content keyspace.ContentID
	values  Coordinate
}

func (schema *Schema) ReturnBoundary(shard linkproject.Shard, term keyspace.Term) (ReturnBoundary, bool) {
	if schema == nil || schema.source == nil || shard == (linkproject.Shard{}) || term == 0 {
		return ReturnBoundary{}, false
	}
	p, ok := schema.source.Project().Mounts().Program(shard)
	if !ok || p == nil || !p.Flow().Executable().Contains(term) {
		return ReturnBoundary{}, false
	}
	_, values, ok := p.Flow().Authored().Control().Returns().Get(term)
	if !ok {
		return ReturnBoundary{}, false
	}
	value, ok := schema.source.Boundary().Values().Of(shard, values)
	if !ok {
		return ReturnBoundary{}, false
	}
	coordinate, ok := schema.CoordinateFor(value)
	if !ok {
		return ReturnBoundary{}, false
	}
	shardIndex, shardOK := schema.source.Project().Mounts().Index(shard)
	content := returnBoundaryContent(schema.source.ContentID(), uint64(shardIndex+1), term)
	if !shardOK || !content.Available() {
		return ReturnBoundary{}, false
	}
	return ReturnBoundary{schema: schema, shard: shard, term: term, content: content, values: coordinate}, true
}

func (boundary ReturnBoundary) valid() bool {
	if boundary.schema == nil || !boundary.content.Available() {
		return false
	}
	expected, ok := boundary.schema.ReturnBoundary(boundary.shard, boundary.term)
	return ok && expected == boundary
}

func (schema *Schema) OwnsReturnBoundary(boundary ReturnBoundary) bool {
	return schema != nil && boundary.schema == schema && boundary.valid()
}

func (boundary ReturnBoundary) ID() (keyspace.ContentID, bool) {
	if !boundary.valid() {
		return keyspace.ContentID{}, false
	}
	return boundary.content, true
}

// Values returns the already-issued Value coordinate for the exact returned
// Pack.  A caller never receives a Link projection row or raw Program term.
func (boundary ReturnBoundary) Values() (Coordinate, bool) {
	if !boundary.valid() {
		return Coordinate{}, false
	}
	return boundary.values, true
}

func returnBoundaryContent(linkID keyspace.ContentID, shard uint64, term keyspace.Term) keyspace.ContentID {
	var payload [56]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("val-ret!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shard))
	binary.BigEndian.PutUint64(payload[48:56], uint64(term))
	return sha256.Sum256(payload[:])
}

// CapabilitySeed is one existing Link capability source.  The associated
// provider capability remains a Link handle; Value only admits it beneath an
// exact atom through WithCapability.
type CapabilitySeed struct {
	schema *Schema
	seed   linkhost.ProviderCapabilitySeed
	index  uint32
}

// CapabilityCount is the sealed provider-instance range admitted by this
// Schema. The handles themselves remain Link-owned.
func (schema *Schema) CapabilityCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.capabilities)
}

func (schema *Schema) CapabilityAt(index int) (linkhost.ProviderCapability, bool) {
	if schema == nil || index < 0 || index >= len(schema.capabilities) {
		return linkhost.ProviderCapability{}, false
	}
	return schema.capabilities[index], true
}

func (schema *Schema) CapabilitySeedCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.capabilitySeeds)
}

func (schema *Schema) CapabilitySeedAt(index int) (CapabilitySeed, bool) {
	if schema == nil || index < 0 || index >= len(schema.capabilitySeeds) {
		return CapabilitySeed{}, false
	}
	return CapabilitySeed{schema: schema, seed: schema.capabilitySeeds[index], index: uint32(index)}, true
}

func (schema *Schema) CapabilitySeed(seed linkhost.ProviderCapabilitySeed) (CapabilitySeed, bool) {
	if schema == nil {
		return CapabilitySeed{}, false
	}
	capability, ok := schema.source.Host().CapabilitySeeds().Capability(seed)
	if !ok || schema.capabilityID[capability] == 0 {
		return CapabilitySeed{}, false
	}
	for index, candidate := range schema.capabilitySeeds {
		if candidate == seed {
			return CapabilitySeed{schema: schema, seed: seed, index: uint32(index)}, true
		}
	}
	return CapabilitySeed{}, false
}

func (seed CapabilitySeed) valid() bool {
	if seed.schema == nil || seed.schema.source == nil || int(seed.index) >= len(seed.schema.capabilitySeeds) || seed.schema.capabilitySeeds[seed.index] != seed.seed {
		return false
	}
	capability, ok := seed.schema.source.Host().CapabilitySeeds().Capability(seed.seed)
	return ok && seed.schema.capabilityID[capability] != 0
}

// ID identifies this exact Link capability-source row. Link retains the
// canonical range and all source geometry; the Schema-fenced ordinal merely
// gives a typed Rule operand its stable semantic content without a name key.
func (seed CapabilitySeed) ID() (keyspace.ContentID, bool) {
	if !seed.valid() || !seed.schema.source.ContentID().Available() {
		return keyspace.ContentID{}, false
	}
	var payload [32 + 8 + 8]byte
	linkID := seed.schema.source.ContentID()
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], uint64(seed.index))
	binary.BigEndian.PutUint64(payload[40:48], 3)
	return sha256.Sum256(payload[:]), true
}

func (seed CapabilitySeed) Capability() (linkhost.ProviderCapability, bool) {
	if !seed.valid() {
		return linkhost.ProviderCapability{}, false
	}
	return seed.schema.source.Host().CapabilitySeeds().Capability(seed.seed)
}

func (seed CapabilitySeed) Source() (linkhost.ProviderCapabilitySource, bool) {
	if !seed.valid() {
		return linkhost.ProviderCapabilitySourceInvalid, false
	}
	return seed.schema.source.Host().CapabilitySeeds().Source(seed.seed)
}

// Exposure returns the existing Value coordinate only for Link's exposure
// capability-source variant. Initial-root, ABI-input, and result sources have
// no Program Value by design and remain their owning boundary relations.
func (seed CapabilitySeed) Exposure() (Coordinate, bool) {
	if !seed.valid() {
		return Coordinate{}, false
	}
	source, ok := seed.Source()
	if !ok || source != linkhost.ProviderCapabilitySourceExposure {
		return Coordinate{}, false
	}
	value, ok := seed.schema.source.Host().CapabilitySeeds().Exposure(seed.seed)
	if !ok {
		return Coordinate{}, false
	}
	return seed.schema.CoordinateFor(value)
}

// ApplyExposure decorates every exact alternative in an existing exposure
// fact with the capability selected by this seed. Its input must be the same
// sealed Value coordinate as the exposure: capabilities never flow to an
// arbitrary equal-shaped fact.
func (seed CapabilitySeed) ApplyExposure(coordinate Coordinate, input Value) (Value, bool) {
	exposure, ok := seed.Exposure()
	if !ok || exposure != coordinate || !seed.schema.owns(input) {
		return Value{}, false
	}
	if input.top || seed.schema.Equal(input, seed.schema.Bottom()) {
		return input, true
	}
	capability, ok := seed.Capability()
	if !ok {
		return Value{}, false
	}
	atoms, ok := seed.schema.Atoms(input)
	if !ok {
		return Value{}, false
	}
	result := input
	for _, atom := range atoms {
		result, ok = seed.schema.WithCapability(result, atom, capability)
		if !ok {
			return Value{}, false
		}
	}
	return result, true
}

// HostMember is one exact host member row in existing Link order.  Its index
// is local only to this Schema and never enters Value state or a serialized
// fact image.
type HostMember struct {
	schema *Schema
	index  uint32
}

func (schema *Schema) HostMemberCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.hostMembers)
}

func (schema *Schema) HostMemberAt(index int) (HostMember, bool) {
	if schema == nil || index < 0 || index >= len(schema.hostMembers) {
		return HostMember{}, false
	}
	return HostMember{schema: schema, index: uint32(index)}, true
}

func (member HostMember) valid() bool {
	return member.schema != nil && int(member.index) < len(member.schema.hostMembers)
}

func (member HostMember) Capability() (linkhost.ProviderCapability, bool) {
	if !member.valid() {
		return linkhost.ProviderCapability{}, false
	}
	return member.schema.hostMembers[member.index].capability, true
}

func (member HostMember) Output() (linkboundary.Value, bool) {
	if !member.valid() {
		return linkboundary.Value{}, false
	}
	return member.schema.hostMembers[member.index].output, true
}

func (member HostMember) Endpoint() (linkboundary.Endpoint, bool) {
	if !member.valid() {
		return linkboundary.Endpoint{}, false
	}
	return member.schema.hostMembers[member.index].endpoint, true
}
