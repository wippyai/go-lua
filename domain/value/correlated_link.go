package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// SourceSeed is one unconditional, result-bearing Link Value admitted by this
// exact Schema. It is a cold Rule operand over the existing Link Value; Value
// introduces neither a source row nor a second source identity.
type SourceSeed struct {
	schema     *Schema
	valueID    identity.ContentID
	module     identity.ContentID
	occurrence identity.ContentID
}

// SourceSeed admits exactly Program literals and binder-authorized runtime
// TypeValue roots. Contextual boot, endpoint, fresh-result, capability, and
// host-member relations remain available only through their owning relations.
func (schema *Schema) SourceSeedForValueID(id identity.ContentID) (SourceSeed, bool) {
	if schema == nil || !id.Available() {
		return SourceSeed{}, false
	}
	row, ok := schema.coordinates[id]
	if !ok || row.coordinate == 0 || row.source.schema != schema || !row.source.valid() {
		return SourceSeed{}, false
	}
	return SourceSeed{schema: schema, valueID: id}, true
}

func (schema *Schema) unconditionalValueID(value identity.ContentID) bool {
	if schema == nil || !value.Available() {
		return false
	}
	if _, ok := schema.SourceValueID(value); !ok {
		return false
	}
	if schema.typeRefs[value] != 0 {
		return true
	}
	_, _, ok := schema.sourceLiteralID(value)
	return ok
}

func (seed SourceSeed) valid() bool {
	if seed.schema == nil || !seed.valueID.Available() {
		return false
	}
	row, ok := seed.schema.coordinates[seed.valueID]
	return ok && row.coordinate != 0 && row.source.schema == seed.schema && row.source.valid()
}

// ID returns the existing canonical Link Value identity of this source.
func (seed SourceSeed) ID() (identity.ContentID, bool) {
	if !seed.valid() {
		return identity.ContentID{}, false
	}
	return seed.valueID, true
}

// Occurrence returns the exact mount-qualified ProgramArtifact row that
// issued this source. Raw shard/term coordinates never cross this receipt
// boundary.
func (seed SourceSeed) Occurrence() (identity.ContentID, identity.ContentID, bool) {
	if !seed.valid() || !seed.module.Available() || !seed.occurrence.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	return seed.module, seed.occurrence, true
}

// Result rederives both the existing Value Factor coordinate and its immutable
// source fact. The fact contains no integer or floating-point payload.
func (seed SourceSeed) Result() (Coordinate, Value, bool) {
	if !seed.valid() {
		return Coordinate{}, Value{}, false
	}
	row := seed.schema.coordinates[seed.valueID]
	return Coordinate{schema: seed.schema, index: row.coordinate}, row.source, true
}

// ReturnBoundary is Value's direct heterogeneous topology for one executable
// Program return. Link supplies the sealed shard topology only: no Link
// return row, identity, or projection mediates this domain operand. The root
// and every fixed Values member are already-issued Value coordinates; the
// optional open tail is represented only by its closed Program ordinal and is
// intentionally not exposed as another coordinate.
type ReturnBoundary struct {
	schema       *Schema
	key          computationKey
	content      identity.ContentID
	root         Coordinate
	memberOffset uint32
	memberCount  uint32
	hasTail      bool
	tailKind     programschema.ValuesTailKind
}

func (schema *Schema) ReturnBoundary(module, occurrence identity.ContentID) (ReturnBoundary, bool) {
	if schema == nil || schema.returnBoundaries == nil || !module.Available() || !occurrence.Available() {
		return ReturnBoundary{}, false
	}
	boundary, ok := schema.returnBoundaries[computationKey{module: module, occurrence: occurrence}]
	return boundary, ok && boundary.valid()
}

func (boundary ReturnBoundary) valid() bool {
	if boundary.schema == nil || !boundary.content.Available() {
		return false
	}
	expected, ok := boundary.schema.returnBoundaries[boundary.key]
	if !ok || expected != boundary || !boundary.root.Valid() {
		return false
	}
	if uint64(boundary.memberOffset)+uint64(boundary.memberCount) > uint64(len(boundary.schema.returnBoundaryMembers)) {
		return false
	}
	if !boundary.hasTail && boundary.tailKind != programschema.ValuesTailInvalid {
		return false
	}
	if boundary.hasTail && !boundary.tailKind.Valid() {
		return false
	}
	return true
}

func (schema *Schema) OwnsReturnBoundary(boundary ReturnBoundary) bool {
	return schema != nil && boundary.schema == schema && boundary.valid()
}

func (boundary ReturnBoundary) ID() (identity.ContentID, bool) {
	if !boundary.valid() {
		return identity.ContentID{}, false
	}
	return boundary.content, true
}

// Root returns the already-issued Value coordinate for the canonical Values
// root. A caller never receives a Link projection row or raw Program term.
func (boundary ReturnBoundary) Root() (Coordinate, bool) {
	if !boundary.valid() {
		return Coordinate{}, false
	}
	return boundary.root, true
}

// MemberCount returns the fixed member width of this exact owner-fenced
// return boundary. Foreign or malformed boundaries expose no rows.
func (boundary ReturnBoundary) MemberCount() int {
	if !boundary.valid() {
		return 0
	}
	return int(boundary.memberCount)
}

// MemberAt returns one ordered fixed Values member from the Schema-owned
// dense member arena. The arena is immutable after sealing and remains fenced
// by the exact Schema that issued this boundary.
func (boundary ReturnBoundary) MemberAt(index int) (Coordinate, bool) {
	if !boundary.valid() || index < 0 || index >= int(boundary.memberCount) {
		return Coordinate{}, false
	}
	position := uint64(boundary.memberOffset) + uint64(index)
	if position >= uint64(len(boundary.schema.returnBoundaryMembers)) {
		return Coordinate{}, false
	}
	member := boundary.schema.returnBoundaryMembers[position]
	if !member.coordinate.Valid() {
		return Coordinate{}, false
	}
	return member.coordinate, true
}

// HasTail reports whether the canonical Values row has an open tail. The
// tail's producer topology is retained as metadata so consumers can widen
// without reconstructing Program rows or inventing a finite coordinate.
func (boundary ReturnBoundary) HasTail() bool {
	return boundary.valid() && boundary.hasTail
}

// TailKind returns the canonical open-tail producer ordinal. Closed or
// foreign boundaries return ValuesTailInvalid rather than leaking metadata.
func (boundary ReturnBoundary) TailKind() programschema.ValuesTailKind {
	if !boundary.valid() || !boundary.hasTail {
		return programschema.ValuesTailInvalid
	}
	return boundary.tailKind
}

// CapabilitySeed is one existing Link capability source.  The associated
// provider capability remains a Link handle; Value only admits it beneath an
// exact atom through WithCapability.
type CapabilitySeed struct {
	schema *Schema
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

func (schema *Schema) CapabilityAt(index int) (identity.ContentID, bool) {
	if schema == nil || index < 0 || index >= len(schema.capabilities) {
		return identity.ContentID{}, false
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
	return CapabilitySeed{schema: schema, index: uint32(index)}, true
}

func (schema *Schema) CapabilitySeedForID(id identity.ContentID) (CapabilitySeed, bool) {
	if schema == nil || !id.Available() {
		return CapabilitySeed{}, false
	}
	for index, candidate := range schema.capabilitySeeds {
		if candidate.id == id {
			return CapabilitySeed{schema: schema, index: uint32(index)}, true
		}
	}
	return CapabilitySeed{}, false
}

func (seed CapabilitySeed) valid() bool {
	return seed.schema != nil && int(seed.index) < len(seed.schema.capabilitySeeds) && seed.schema.capabilitySeeds[seed.index].id.Available() && seed.schema.capabilityID[seed.schema.capabilitySeeds[seed.index].capability] != 0
}

// ID identifies this exact Link capability-source row. Link retains the
// canonical range and all source geometry; the Schema-fenced ordinal merely
// gives a typed Rule operand its stable semantic content without a name key.
func (seed CapabilitySeed) ID() (identity.ContentID, bool) {
	if !seed.valid() || !seed.schema.linkID.Available() {
		return identity.ContentID{}, false
	}
	var payload [32 + 8 + 8]byte
	linkID := seed.schema.linkID
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:40], uint64(seed.index))
	binary.BigEndian.PutUint64(payload[40:48], 3)
	return sha256.Sum256(payload[:]), true
}

func (seed CapabilitySeed) CapabilityID() (identity.ContentID, bool) {
	if !seed.valid() {
		return identity.ContentID{}, false
	}
	return seed.schema.capabilitySeeds[seed.index].capability, true
}

func (seed CapabilitySeed) Source() (CapabilitySource, bool) {
	if !seed.valid() {
		return CapabilitySourceInvalid, false
	}
	return seed.schema.capabilitySeeds[seed.index].source, true
}

// Exposure returns the existing Value coordinate only for Link's exposure
// capability-source variant. Initial-root, ABI-input, and result sources have
// no Program Value by design and remain their owning boundary relations.
func (seed CapabilitySeed) Exposure() (Coordinate, bool) {
	if !seed.valid() {
		return Coordinate{}, false
	}
	source, ok := seed.Source()
	if !ok || source != CapabilitySourceExposure {
		return Coordinate{}, false
	}
	value := seed.schema.capabilitySeeds[seed.index].exposure
	if !value.Available() {
		return Coordinate{}, false
	}
	return seed.schema.CoordinateForID(value)
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
	capability, ok := seed.CapabilityID()
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

func (member HostMember) CapabilityID() (identity.ContentID, bool) {
	if !member.valid() {
		return identity.ContentID{}, false
	}
	return member.schema.hostMembers[member.index].capability, true
}

func (member HostMember) OutputID() (identity.ContentID, bool) {
	if !member.valid() {
		return identity.ContentID{}, false
	}
	return member.schema.hostMembers[member.index].output, true
}

func (member HostMember) EndpointID() (identity.ContentID, bool) {
	if !member.valid() {
		return identity.ContentID{}, false
	}
	return member.schema.hostMembers[member.index].endpoint, true
}
