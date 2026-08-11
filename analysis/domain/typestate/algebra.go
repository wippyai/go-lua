package typestate

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/keyspace"
	proglink "github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target"
)

// Coordinate is one exact protocol-state × cleanup-duty × holder tuple.  It
// is decoded from Schema's finite Link-backed range; clients cannot submit a
// partial support declaration.
type Coordinate struct {
	State  StateCoordinate
	Duty   Duty
	Holder HolderOrigin
}

// Schema is the sole immutable Typestate family for one sealed Link.  It owns
// the complete origin and coordinate range.  There is no caller-supplied
// support table and no precomputed origin×state×duty×holder product.
type Schema struct{ universe *universe }

type universe struct {
	source          *proglink.Link
	linkID          keyspace.ContentID
	id              keyspace.ContentID
	sources         []ResourceSource
	keys            []Key
	keyIndex        map[Key]uint32
	ranges          []keyRange
	coordinateCount uint32
}

// keyRange is Schema's private cached image of one Link ResourceOrigin and
// its closed materialization role.  It keeps only per-key offsets and exact
// Link-owned holder support; it is not a stored Cartesian coordinate product.
type keyRange struct {
	key             Key
	protocol        target.Protocol
	states          []StateCoordinate
	stateIndex      map[StateCoordinate]uint32
	holders         []HolderOrigin
	holderIndex     map[HolderOrigin]uint32
	coordinateStart uint32
	coordinateEnd   uint32
}

// NewSchema admits the complete canonical Link range.  The Link owns
// structural origins; this Factor owns only its closed recurrence roles and
// the finite coordinate decoding law.
func NewSchema(source *proglink.Link) (Schema, bool) {
	if source == nil || !source.ContentID().Available() {
		return Schema{}, false
	}
	owner := &universe{
		source:   source,
		linkID:   source.ContentID(),
		keyIndex: make(map[Key]uint32),
	}
	if !owner.cacheResourceRange() {
		return Schema{}, false
	}
	owner.id = typestateSchemaID(owner.linkID)
	schema := Schema{universe: owner}
	if !owner.id.Available() || owner.coordinateCount == 0 {
		return Schema{}, false
	}
	return schema, true
}

// cacheResourceRange derives Typestate's complete source range from Target
// declarations and Link's direct template-eligibility facts. All later key
// admission and coordinate lookups are local indexed operations.
func (u *universe) cacheResourceRange() bool {
	if u == nil || u.source == nil || u.keyIndex == nil {
		return false
	}
	sources := enumerateResourceSources(u.source)
	if len(sources) == 0 {
		return false
	}
	u.sources = append([]ResourceSource(nil), sources...)
	for _, raw := range u.sources {
		protocol := raw.protocol
		var support []HolderOrigin
		roles := []materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary}
		if raw.kind == resourceSourceUnknown {
			opaque, ok := OpaqueHolder(u.source)
			if !ok {
				return false
			}
			support = []HolderOrigin{opaque}
			roles = []materialization.Role{materialization.Summary}
		} else {
			var ok bool
			support, ok = holdersForResource(u.source, raw, protocol)
			if !ok {
				return false
			}
			if len(support) == 0 {
				return false
			}
		}
		for _, role := range roles {
			resource, ok := materializeResource(raw, role)
			if !ok || !u.appendKeyRange(Key{Resource: resource}, protocol, support) {
				return false
			}
		}
	}
	return len(u.keys) != 0 && len(u.keys) == len(u.ranges) && len(u.keys) == len(u.keyIndex)
}

// SourceCount is Typestate's complete canonical structural source range.
func (s Schema) SourceCount() int {
	if !s.Valid() {
		return 0
	}
	return len(s.universe.sources)
}

// SourceAt returns one Typestate-owned source coordinate. It is not a Link
// origin handle and carries no caller-constructible cross-product identity.
func (s Schema) SourceAt(index int) (ResourceSource, bool) {
	if !s.Valid() || index < 0 || index >= len(s.universe.sources) {
		return ResourceSource{}, false
	}
	source := s.universe.sources[index]
	return source, source.validFor(s.universe.source)
}

func holdersForResource(source *proglink.Link, raw ResourceSource, protocol target.Protocol) ([]HolderOrigin, bool) {
	local, ok := LocalHolder(source, raw.application)
	if !ok {
		return nil, false
	}
	result := []HolderOrigin{local}
	// Lifecycle activation products remain intentionally absent: this cut owns
	// static source structure only, so every call-backed source starts local.
	_ = protocol
	return result, true
}

func (u *universe) appendKeyRange(key Key, protocol target.Protocol, holders []HolderOrigin) bool {
	if u == nil || !key.validFor(u.source) || len(holders) == 0 {
		return false
	}
	if _, duplicate := u.keyIndex[key]; duplicate {
		return false
	}
	contract, targetOK := u.source.Boundary().Target()
	if !targetOK || contract == nil {
		return false
	}
	states := make([]StateCoordinate, contract.StateCount(protocol))
	stateIndex := make(map[StateCoordinate]uint32, len(states))
	for index := range states {
		targetState, ok := contract.StateAt(protocol, index)
		if !ok {
			return false
		}
		state, ok := stateCoordinateForUniverse(u, protocol, targetState)
		if !ok {
			return false
		}
		if _, duplicate := stateIndex[state]; duplicate {
			return false
		}
		states[index] = state
		stateIndex[state] = uint32(index)
	}
	if len(states) == 0 {
		return false
	}
	ownedHolders := append([]HolderOrigin(nil), holders...)
	holderIndex := make(map[HolderOrigin]uint32, len(ownedHolders))
	for index, holder := range ownedHolders {
		if !holder.validFor(u.source) {
			return false
		}
		if _, duplicate := holderIndex[holder]; duplicate {
			return false
		}
		holderIndex[holder] = uint32(index)
	}
	width := uint64(len(states)) * uint64(DutyUnknown) * uint64(len(ownedHolders))
	if width == 0 || width > uint64(math.MaxUint32) || uint64(u.coordinateCount)+width > uint64(math.MaxUint32) {
		return false
	}
	index := uint32(len(u.keys))
	u.keys = append(u.keys, key)
	u.keyIndex[key] = index
	u.ranges = append(u.ranges, keyRange{
		key:             key,
		protocol:        protocol,
		states:          states,
		stateIndex:      stateIndex,
		holders:         ownedHolders,
		holderIndex:     holderIndex,
		coordinateStart: u.coordinateCount,
		coordinateEnd:   u.coordinateCount + uint32(width),
	})
	u.coordinateCount += uint32(width)
	return true
}

func (s Schema) Valid() bool {
	return s.universe != nil && s.universe.source != nil && s.universe.linkID.Available() &&
		s.universe.source.ContentID() == s.universe.linkID && s.universe.id.Available() &&
		len(s.universe.sources) != 0 && len(s.universe.keys) != 0 && len(s.universe.keys) == len(s.universe.ranges) &&
		len(s.universe.keys) == len(s.universe.keyIndex) && s.universe.coordinateCount != 0
}

func (s Schema) ContentID() keyspace.ContentID {
	if !s.Valid() {
		return keyspace.ContentID{}
	}
	return s.universe.id
}

func (s Schema) LinkContentID() keyspace.ContentID {
	if !s.Valid() {
		return keyspace.ContentID{}
	}
	return s.universe.linkID
}

// KeyCount is the complete Link-origin range after closed Factor role
// materialization.  Exact structural sources support Exact/Recent/Summary;
// explicit unknown sources support Summary only.
func (s Schema) KeyCount() int {
	if !s.Valid() {
		return 0
	}
	return len(s.universe.keys)
}

// KeyAt returns one canonical cached materialized source.  Link's origin
// range was consumed once by NewSchema; this does not reopen that range.
func (s Schema) KeyAt(index int) (Key, bool) {
	if !s.Valid() || index < 0 || index >= len(s.universe.keys) {
		return Key{}, false
	}
	return s.universe.keys[index], true
}

// Admit is the complete resource-key admission law.  A valid opaque key must
// reappear in the canonical finite range for this exact Schema.
func (s Schema) Admit(resource ResourceOrigin) (Key, bool) {
	if !s.Valid() {
		return Key{}, false
	}
	key := Key{Resource: resource}
	index, ok := s.universe.keyIndex[key]
	if !ok || uint64(index) >= uint64(len(s.universe.keys)) {
		return Key{}, false
	}
	canonical := s.universe.keys[index]
	if canonical != key {
		return Key{}, false
	}
	return canonical, true
}

// CoordinateCount is the whole finite product encoded by this Factor: each
// admitted key's protocol-local state range × the closed duty vocabulary ×
// its exact Link-owned holder support. Per-key offsets encode this range;
// Schema never allocates a second Cartesian coordinate table.
func (s Schema) CoordinateCount() int {
	if !s.Valid() {
		return 0
	}
	return int(s.universe.coordinateCount)
}

// CoordinateAt returns the exact resource key and tuple at a finite canonical
// index. Callers never manufacture coordinates. Every holder was admitted by
// the exact Link resource-holder row cached for this key; a shared Application
// alone never supplies lifecycle support.
func (s Schema) CoordinateAt(index int) (Key, Coordinate, bool) {
	if !s.Valid() || index < 0 || uint64(index) >= uint64(s.universe.coordinateCount) {
		return Key{}, Coordinate{}, false
	}
	coordinate := uint32(index)
	keyIndex := sort.Search(len(s.universe.ranges), func(index int) bool {
		return s.universe.ranges[index].coordinateEnd > coordinate
	})
	if keyIndex >= len(s.universe.ranges) {
		return Key{}, Coordinate{}, false
	}
	range_ := s.universe.ranges[keyIndex]
	if coordinate < range_.coordinateStart || len(range_.states) == 0 || len(range_.holders) == 0 {
		return Key{}, Coordinate{}, false
	}
	remaining := int(coordinate - range_.coordinateStart)
	holderCount := len(range_.holders)
	stateIndex := remaining / (int(DutyUnknown) * holderCount)
	holderIndex := remaining % holderCount
	if stateIndex < 0 || stateIndex >= len(range_.states) || holderIndex < 0 || holderIndex >= len(range_.holders) {
		return Key{}, Coordinate{}, false
	}
	duty := Duty((remaining/holderCount)%int(DutyUnknown) + int(DutyLocal))
	if !duty.Valid() {
		return Key{}, Coordinate{}, false
	}
	return range_.key, Coordinate{State: range_.states[stateIndex], Duty: duty, Holder: range_.holders[holderIndex]}, true
}

func (s Schema) coordinate(key Key, coordinate Coordinate) (uint32, bool) {
	range_, ok := s.keyRange(key)
	if !ok || !coordinate.Duty.Valid() {
		return 0, false
	}
	stateIndex, ok := range_.stateIndex[coordinate.State]
	if !ok {
		return 0, false
	}
	holderIndex, ok := range_.holderIndex[coordinate.Holder]
	if !ok {
		return 0, false
	}
	value := uint64(range_.coordinateStart) + uint64(stateIndex)*uint64(DutyUnknown)*uint64(len(range_.holders)) +
		uint64(coordinate.Duty-DutyLocal)*uint64(len(range_.holders)) + uint64(holderIndex) + 1
	if value == 0 || value > uint64(math.MaxUint32) {
		return 0, false
	}
	return uint32(value), true
}

// holderCount and holderAt decode the exact Link-owned holder range cached for
// one key. A callback occurrence is never inferred from a shared Application:
// Link has no resource-to-callback row, so it cannot enter this support.
func (s Schema) holderCount(key Key) int {
	range_, ok := s.keyRange(key)
	if !ok {
		return 0
	}
	return len(range_.holders)
}

func (s Schema) holderAt(key Key, index int) (HolderOrigin, bool) {
	range_, ok := s.keyRange(key)
	if !ok || index < 0 || index >= len(range_.holders) {
		return HolderOrigin{}, false
	}
	return range_.holders[index], true
}

func (s Schema) holderIndex(key Key, holder HolderOrigin) (int, bool) {
	range_, ok := s.keyRange(key)
	if !ok {
		return 0, false
	}
	index, ok := range_.holderIndex[holder]
	return int(index), ok
}

func (s Schema) protocol(key Key) (target.Protocol, bool) {
	range_, ok := s.keyRange(key)
	if !ok {
		return 0, false
	}
	return range_.protocol, true
}

func (s Schema) keyRange(key Key) (keyRange, bool) {
	if !s.Valid() {
		return keyRange{}, false
	}
	index, ok := s.universe.keyIndex[key]
	if !ok || uint64(index) >= uint64(len(s.universe.ranges)) {
		return keyRange{}, false
	}
	range_ := s.universe.ranges[index]
	if range_.key != key {
		return keyRange{}, false
	}
	return range_, true
}

func (s Schema) Rebind(source *proglink.Link) (Schema, bool) {
	if !s.Valid() || source == nil || source.ContentID() != s.universe.linkID {
		return Schema{}, false
	}
	rebound, ok := NewSchema(source)
	if !ok || rebound.ContentID() != s.ContentID() {
		return Schema{}, false
	}
	return rebound, true
}

// Algebra is the one homogeneous lattice instance for all Typestate keys.
type Algebra struct{ schema Schema }

func NewAlgebra(schema Schema) (Algebra, bool) {
	if !schema.Valid() {
		return Algebra{}, false
	}
	return Algebra{schema: schema}, true
}

func (a Algebra) Default() Relation {
	if !a.valid() {
		return Relation{}
	}
	return bottomRelation(a.schema.universe)
}

func (a Algebra) Top() Relation {
	if !a.valid() {
		return Relation{}
	}
	return topRelation(a.schema.universe)
}

func (a Algebra) Of(key Key, entries ...Entry) (Relation, bool) {
	if !a.valid() {
		return Relation{}, false
	}
	if _, ok := a.schema.Admit(key.Resource); !ok {
		return Relation{}, false
	}
	value, ok := normalizeRelation(a.schema, key, entries)
	if !ok || !a.accepts(value) {
		return Relation{}, false
	}
	return value, true
}

// Admits is the closed Relation-family fence used by the declaring owner.
// Link-derived key selection remains separate from this carrier check.
func (a Algebra) Admits(value Relation) bool { return a.accepts(value) }

// AdmitsAt is the per-key carrier fence for owners. A homogeneous Relation
// still records its resource identity in every non-bottom cell, so acceptance
// at one key must not admit a value constructed for another key.
func (a Algebra) AdmitsAt(key Key, value Relation) bool {
	return a.validFact(Fact{Key: key, Value: value})
}

func (a Algebra) Equal(left, right Relation) bool {
	return a.accepts(left) && a.accepts(right) && equalRelation(left, right)
}
func (a Algebra) Same(left, right Relation) bool { return a.Equal(left, right) }
func (a Algebra) LessOrEq(left, right Relation) bool {
	return a.accepts(left) && a.accepts(right) && lessRelation(left, right)
}
func (a Algebra) Join(left, right Relation) Relation {
	if !a.accepts(left) || !a.accepts(right) {
		return Relation{}
	}
	return joinRelation(left, right)
}
func (a Algebra) Meet(left, right Relation) Relation {
	if !a.accepts(left) || !a.accepts(right) {
		return Relation{}
	}
	return meetRelation(left, right)
}
func (a Algebra) Widen(previous, next Relation) Relation { return a.Join(previous, next) }

func (a Algebra) WidenRank(key Key, value Relation, component int) uint64 {
	if component != 0 || !a.accepts(value) || value.IsTop() {
		return 0
	}
	canonical, ok := a.schema.Admit(key.Resource)
	if !ok || canonical != key {
		return 0
	}
	range_, ok := a.schema.keyRange(key)
	if !ok || len(range_.holders) == 0 || len(range_.states) == 0 {
		return 0
	}
	rank := uint64(len(range_.states)) * uint64(DutyUnknown) * uint64(len(range_.holders)) * 3
	if rank == 0 {
		return 0
	}
	// A relation supplied at this key may only contain coordinates for this
	// resource. Guard subtraction so malformed foreign/overfull payloads cannot
	// underflow a measure that is used to prove widening termination.
	keyID := key.Resource.id
	for _, current := range value.cells {
		if current.resource != keyID {
			return 0
		}
		coordinateKey, _, coordinateOK := a.schema.CoordinateAt(int(current.coordinate - 1))
		if !coordinateOK || coordinateKey != key {
			return 0
		}
		used := uint64(population(current.count))
		if used > rank {
			return 0
		}
		rank -= used
	}
	return rank
}

func population(count Multiplicity) int {
	n := 0
	for count != 0 {
		n += int(count & 1)
		count >>= 1
	}
	return n
}

func (a Algebra) Lattice() lattice.Lattice[Relation] {
	return lattice.Lattice[Relation]{Bottom: a.Default, Top: a.Top, Equal: a.Equal, Same: a.Same, LessOrEq: a.LessOrEq, Join: a.Join, Meet: a.Meet, Widen: a.Widen}
}

func (a Algebra) Fingerprint(value Relation) uint64 {
	if !a.accepts(value) {
		return 0
	}
	hash := uint64(0x54_59_50_45)
	for offset := 0; offset < len(a.schema.universe.id); offset += 8 {
		hash = internal.MixHash(hash, binary.BigEndian.Uint64(a.schema.universe.id[offset:offset+8]))
	}
	if value.top {
		return internal.MixHash(hash, 1)
	}
	for _, current := range value.cells {
		hash = internal.MixHash(hash, uint64(current.coordinate))
		hash = internal.MixHash(hash, uint64(current.count))
	}
	return hash
}

func (a Algebra) valid() bool { return a.schema.Valid() }

func (a Algebra) accepts(value Relation) bool {
	if !a.valid() || value.universe != a.schema.universe {
		return false
	}
	if value.top {
		return true
	}
	for index, current := range value.cells {
		if current.coordinate == 0 || !current.resource.Available() || !current.count.Valid() ||
			index > 0 && value.cells[index-1].coordinate >= current.coordinate {
			return false
		}
	}
	return true
}

func typestateSchemaID(linkID keyspace.ContentID) keyspace.ContentID {
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.typestate.schema"))
	_, _ = hash.Write(linkID[:])
	writeTypestateWord(hash, 5)
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func writeTypestateWord(dst interface{ Write([]byte) (int, error) }, value uint64) {
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], value)
	_, _ = dst.Write(word[:])
}
