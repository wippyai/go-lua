package value

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Every computation operand this Schema seals names a fixed, small set of
// Value coordinates: the coordinate its rule writes and the coordinates its
// rule reads. Before this table each consuming family recovered that set the
// same way - authenticate the row, read its Endpoints, then ask
// CoordinateIndex for each coordinate - and it re-ran the whole walk once per
// read projector, once per write projector and once again inside the fold.
// The Schema is the seal that owns the coordinate directory, so it resolves
// every endpoint once, content-addresses the result, and hands consumers a
// dense vector they address by role.

// EndpointRole is the closed set of positions one sealed endpoint vector can
// carry. A family declares only the roles its operand actually has; asking a
// vector for a role it does not declare refuses rather than defaulting.
type EndpointRole uint8

const (
	EndpointRoleInvalid EndpointRole = iota
	// EndpointWrite is the coordinate the consuming rule writes.
	EndpointWrite
	// EndpointResult is the computation's own result coordinate. It is
	// declared only where it differs from the write target, which is the
	// guarded runtime-kind refinement: that rule narrows its input rather
	// than publishing the call result.
	EndpointResult
	// EndpointLeft and EndpointRight are the ordered inputs of a binary
	// computation. A unary computation declares only EndpointLeft.
	EndpointLeft
	EndpointRight
	// EndpointCompared is the coordinate a runtime-kind refinement compares
	// its input against.
	EndpointCompared
	endpointRoleCount
)

func (role EndpointRole) Available() bool {
	return role > EndpointRoleInvalid && role < endpointRoleCount
}

// endpointRow holds one operand's resolved endpoints. Each cell is a dense
// coordinate index biased by one, so zero is the sealed absence of a role.
type endpointRow struct {
	// key and family are the owner-issued identity of the row in the one
	// endpoint table.  Keeping them beside the role cells lets a consumer
	// redeem a family row by its endpoint ordinal without constructing a
	// second per-family order.  The table remains the sole denominator.
	key    computationKey
	family endpointFamily
	roles  [endpointRoleCount]uint32
}

// Endpoints is the owner-fenced handle for one row of the projection.
type Endpoints struct {
	schema *Schema
	slot   uint32
}

const endpointTableVersion uint64 = 1

const endpointTableDomain = "wippy.analysis.value.endpoints"

// endpointDraft is the seal-time image of one operand's endpoint vector. The
// operand's already owner-issued content identity orders the table, so the
// ordinals are a function of the sealed program and not of Go map iteration.
type endpointDraft struct {
	content identity.ContentID
	key     computationKey
	family  endpointFamily
	branch  uint8
	roles   [endpointRoleCount]uint32
}

// endpointFamily discriminates which operand map owns a draft so the slot can
// be written back to its row after the ordinals are assigned.
type endpointFamily uint8

const (
	endpointFamilyInvalid endpointFamily = iota
	endpointFamilyArithmetic
	endpointFamilyEquality
	endpointFamilyOrder
	endpointFamilyPresenceRefinement
	endpointFamilyModuleLoad
	endpointFamilyRuntimeKind
)

// sealEndpointVectors issues the projection once, after every operand family
// and the canonical coordinate order are sealed. It is a function of the
// sealed program alone: the drafts are ordered by owner-issued content
// identity, so two Links over one program produce identical ordinals and one
// identical table identity.
func (builder *valueBuilder) sealEndpointVectors() bool {
	if builder == nil || builder.Schema == nil || builder.endpoints != nil {
		return false
	}
	drafts := make([]endpointDraft, 0,
		len(builder.binaryArithmetics)+len(builder.binaryEqualities)+len(builder.binaryOrders)+
			len(builder.presenceRefinements)+len(builder.moduleLoadCalls)+len(builder.runtimeKindCalls))

	for key, row := range builder.binaryArithmetics {
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyArithmetic, map[EndpointRole]Coordinate{
			EndpointWrite: row.result, EndpointLeft: row.left, EndpointRight: row.right,
		})
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}
	for key, row := range builder.binaryEqualities {
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyEquality, map[EndpointRole]Coordinate{
			EndpointWrite: row.result, EndpointLeft: row.left, EndpointRight: row.right,
		})
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}
	for key, row := range builder.binaryOrders {
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyOrder, map[EndpointRole]Coordinate{
			EndpointWrite: row.result, EndpointLeft: row.left, EndpointRight: row.right,
		})
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}
	for key, row := range builder.presenceRefinements {
		// A presence refinement narrows the coordinate it reads, so its write
		// target and its sole input are one coordinate.
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyPresenceRefinement, map[EndpointRole]Coordinate{
			EndpointWrite: row.target, EndpointLeft: row.target,
		})
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}
	for key, row := range builder.moduleLoadCalls {
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyModuleLoad, map[EndpointRole]Coordinate{
			EndpointWrite: row.result, EndpointLeft: row.argument,
		})
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}
	for key, row := range builder.runtimeKindCalls {
		roles := map[EndpointRole]Coordinate{
			EndpointWrite: row.write, EndpointResult: row.result, EndpointLeft: row.input,
		}
		if row.refinement {
			roles[EndpointCompared] = row.comparison
		}
		draft, ok := builder.draftEndpoints(row.content, key, endpointFamilyRuntimeKind, roles)
		if !ok {
			return false
		}
		drafts = append(drafts, draft)
	}

	sort.Slice(drafts, func(left, right int) bool {
		return endpointDraftLess(drafts[left], drafts[right])
	})

	rows := make([]endpointRow, len(drafts))
	table := sha256.New()
	var header [16]byte
	binary.BigEndian.PutUint64(header[0:8], uint64(len(drafts)))
	binary.BigEndian.PutUint64(header[8:16], endpointTableVersion)
	_, _ = table.Write(builder.linkID[:])
	_, _ = table.Write([]byte(endpointTableDomain))
	_, _ = table.Write(header[:])
	for index, draft := range drafts {
		if index > 0 && drafts[index-1].content == draft.content {
			// Two operands cannot share one owner-issued identity; the table
			// would then have no stable ordinal for either.
			return false
		}
		rows[index] = endpointRow{key: draft.key, family: draft.family, roles: draft.roles}
		slot := uint32(index + 1)
		if !builder.installEndpointSlot(draft, slot) {
			return false
		}
		_, _ = table.Write(draft.content[:])
		var cells [endpointRoleCount * 4]byte
		for role := range draft.roles {
			binary.BigEndian.PutUint32(cells[role*4:role*4+4], draft.roles[role])
		}
		_, _ = table.Write(cells[:])
	}
	var tableID identity.ContentID
	copy(tableID[:], table.Sum(nil))
	if !tableID.Available() {
		return false
	}
	if rows == nil {
		rows = []endpointRow{}
	}
	builder.endpoints, builder.endpointTable = rows, tableID
	return true
}

// draftEndpoints resolves one operand's coordinates against the sealed
// coordinate directory. A coordinate this Schema does not own is a malformed
// operand, not a missing role.
func (builder *valueBuilder) draftEndpoints(content identity.ContentID, key computationKey, family endpointFamily, roles map[EndpointRole]Coordinate) (endpointDraft, bool) {
	if builder == nil || builder.Schema == nil || !content.Available() ||
		!key.module.Available() || !key.occurrence.Available() || family == endpointFamilyInvalid || len(roles) == 0 {
		return endpointDraft{}, false
	}
	draft := endpointDraft{content: content, key: key, family: family}
	for role, coordinate := range roles {
		index, indexOK := builder.Schema.CoordinateIndex(coordinate)
		if !role.Available() || !indexOK || index == ^uint32(0) {
			return endpointDraft{}, false
		}
		draft.roles[role] = index + 1
	}
	return draft, true
}

func endpointDraftLess(left, right endpointDraft) bool {
	for index := range left.content {
		if left.content[index] != right.content[index] {
			return left.content[index] < right.content[index]
		}
	}
	return false
}

// installEndpointSlot writes the assigned ordinal back onto the operand row
// that owns it. The row carries the slot so a consumer reaches its endpoints
// without a second directory lookup.
func (builder *valueBuilder) installEndpointSlot(draft endpointDraft, slot uint32) bool {
	switch draft.family {
	case endpointFamilyArithmetic:
		row, ok := builder.binaryArithmetics[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.binaryArithmetics[draft.key] = row
	case endpointFamilyEquality:
		row, ok := builder.binaryEqualities[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.binaryEqualities[draft.key] = row
	case endpointFamilyOrder:
		row, ok := builder.binaryOrders[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.binaryOrders[draft.key] = row
	case endpointFamilyPresenceRefinement:
		row, ok := builder.presenceRefinements[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.presenceRefinements[draft.key] = row
	case endpointFamilyModuleLoad:
		row, ok := builder.moduleLoadCalls[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.moduleLoadCalls[draft.key] = row
	case endpointFamilyRuntimeKind:
		row, ok := builder.runtimeKindCalls[draft.key]
		if !ok || row.endpoints != 0 {
			return false
		}
		row.endpoints = slot
		builder.runtimeKindCalls[draft.key] = row
	default:
		return false
	}
	return true
}

// endpointsSealed is the completeness fence for the projection. It is not
// folded into Valid: the Schema is already valid while the operand families
// it projects are still being sealed.
func (schema *Schema) endpointsSealed() bool {
	return schema != nil && schema.Valid() && schema.endpoints != nil && schema.endpointTable.Available()
}

// EndpointTableID is the content identity of the whole projection. It is the
// identity a Rule Program join pins when it references this table.
func (schema *Schema) EndpointTableID() (identity.ContentID, bool) {
	if !schema.endpointsSealed() {
		return identity.ContentID{}, false
	}
	return schema.endpointTable, true
}

// EndpointCount is the projection's dense extent.
func (schema *Schema) EndpointCount() int {
	if !schema.endpointsSealed() {
		return 0
	}
	return len(schema.endpoints)
}

// EndpointsAt addresses the projection by the dense ordinal a Rule Program
// join carries.
func (schema *Schema) EndpointsAt(ordinal int) (Endpoints, bool) {
	if !schema.endpointsSealed() || ordinal < 0 || ordinal >= len(schema.endpoints) {
		return Endpoints{}, false
	}
	vector := Endpoints{schema: schema, slot: uint32(ordinal + 1)}
	return vector, vector.Available()
}

// OwnsEndpoints authenticates a vector against this exact Schema.
func (schema *Schema) OwnsEndpoints(vector Endpoints) bool {
	return schema != nil && vector.schema == schema && vector.Available()
}

func (vector Endpoints) Available() bool {
	if vector.schema == nil || !vector.schema.endpointsSealed() ||
		vector.slot == 0 || uint64(vector.slot) > uint64(len(vector.schema.endpoints)) {
		return false
	}
	// A vector with no declared role names no coordinate and could never have
	// been sealed from an operand.
	row := vector.schema.endpoints[vector.slot-1]
	for _, cell := range row.roles {
		if cell != 0 {
			return true
		}
	}
	return false
}

// Ordinal is this vector's dense address in the projection.
func (vector Endpoints) Ordinal() (int, bool) {
	if !vector.Available() {
		return 0, false
	}
	return int(vector.slot) - 1, true
}

// Index is the dense Value coordinate index for one declared role: the number
// every consuming family previously recovered by re-reading the operand's
// coordinates and asking CoordinateIndex for each one.
func (vector Endpoints) Index(role EndpointRole) (uint64, bool) {
	if !vector.Available() || !role.Available() {
		return 0, false
	}
	cell := vector.schema.endpoints[vector.slot-1].roles[role]
	if cell == 0 {
		return 0, false
	}
	return uint64(cell - 1), true
}

// Coordinate is the sealed coordinate behind one declared role.
func (vector Endpoints) Coordinate(role EndpointRole) (Coordinate, bool) {
	index, ok := vector.Index(role)
	if !ok || index > uint64(^uint32(0)) {
		return Coordinate{}, false
	}
	return vector.schema.CoordinateAt(int(index))
}

// Declares reports whether this operand carries the given role.
func (vector Endpoints) Declares(role EndpointRole) bool {
	_, ok := vector.Index(role)
	return ok
}

func (schema *Schema) endpointsForSlot(slot uint32) (Endpoints, bool) {
	if !schema.endpointsSealed() || slot == 0 {
		return Endpoints{}, false
	}
	vector := Endpoints{schema: schema, slot: slot}
	return vector, vector.Available()
}

// EndpointVector returns the sealed endpoint vector for one admitted operand.
// The row is authenticated by the same owner fence its other projections use.
func (row BinaryArithmetic) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsBinaryArithmetic(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

func (row BinaryEquality) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsBinaryEquality(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

func (row BinaryOrder) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsBinaryOrder(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

func (row PresenceRefinement) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsPresenceRefinement(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

func (row ModuleLoadCall) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsModuleLoadCall(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

func (row RuntimeKindCall) EndpointVector() (Endpoints, bool) {
	if !row.valid() || !row.schema.OwnsRuntimeKindCall(row) {
		return Endpoints{}, false
	}
	return row.schema.endpointsForSlot(row.endpoints)
}

// Endpoint is the one-call read a consuming rule makes: the dense Value
// coordinate index of one role on one admitted operand. It is the whole of
// what the per-family endpoint walk used to compute.
func (row BinaryArithmetic) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}

// EndpointOrdinal is the one dense candidate address BinaryArithmetic uses.
// It is the ordinal in the Schema's shared endpoint table, not an arithmetic
// family-local index.  Keeping the address on the existing table means a
// Program candidate can round-trip through the same owner directory every
// consumer family already uses.
func (row BinaryArithmetic) EndpointOrdinal() (uint32, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	ordinal, ordinalOK := vector.Ordinal()
	if !ordinalOK || uint64(ordinal) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(ordinal), true
}

// Ordinal is the owner-issued dense candidate address.  It is intentionally
// an alias of EndpointOrdinal: there is no second arithmetic candidate index.
func (row BinaryArithmetic) Ordinal() (uint32, bool) {
	return row.EndpointOrdinal()
}

// Write projects the owner-issued write coordinate without reopening the
// original computation row.  The endpoint vector is the authority for this
// declaration, so a pre-seal or foreign row refuses.
func (row BinaryArithmetic) Write() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointWrite)
}

// Left projects the first owner-issued input coordinate.
func (row BinaryArithmetic) Left() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointLeft)
}

// Right projects the second owner-issued input coordinate.
func (row BinaryArithmetic) Right() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointRight)
}

// BinaryArithmeticForArtifactOccurrence resolves the owner-issued arithmetic
// row for one mounted Program occurrence.  The row's candidate ordinal is
// subsequently redeemed through BinaryArithmeticAt, which addresses the
// shared endpoint table directly.
func (schema *Schema) BinaryArithmeticForArtifactOccurrence(module, occurrence identity.ContentID) (BinaryArithmetic, bool) {
	if schema == nil || !module.Available() || !occurrence.Available() {
		return BinaryArithmetic{}, false
	}
	row, ok := schema.BinaryArithmetic(module, occurrence)
	if !ok || !schema.OwnsBinaryArithmetic(row) {
		return BinaryArithmetic{}, false
	}
	_, vectorOK := row.EndpointVector()
	return row, vectorOK
}

// BinaryArithmeticOrdinal returns the shared endpoint-table ordinal of one
// owner-issued arithmetic row.  It deliberately does not consult or create
// a family-local map.
func (schema *Schema) BinaryArithmeticOrdinal(row BinaryArithmetic) (uint32, bool) {
	if schema == nil || !schema.OwnsBinaryArithmetic(row) {
		return 0, false
	}
	return row.EndpointOrdinal()
}

// BinaryArithmeticAt redeems a dense arithmetic candidate by the ordinal of
// the existing endpoint table.  Endpoint rows belonging to another family
// refuse; they are still part of the denominator and are never renumbered for
// arithmetic.
func (schema *Schema) BinaryArithmeticAt(index int) (BinaryArithmetic, bool) {
	if schema == nil || !schema.endpointsSealed() || index < 0 || index >= len(schema.endpoints) {
		return BinaryArithmetic{}, false
	}
	endpoint := schema.endpoints[index]
	if endpoint.family != endpointFamilyArithmetic {
		return BinaryArithmetic{}, false
	}
	row, ok := schema.binaryArithmetics[endpoint.key]
	if !ok || row.endpoints != uint32(index+1) || !schema.OwnsBinaryArithmetic(row) {
		return BinaryArithmetic{}, false
	}
	return row, true
}

// EndpointOrdinal is the one dense candidate address BinaryEquality uses.
// It is the ordinal in the Schema's shared endpoint table, not an equality
// family-local index.
func (row BinaryEquality) EndpointOrdinal() (uint32, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	ordinal, ordinalOK := vector.Ordinal()
	if !ordinalOK || uint64(ordinal) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(ordinal), true
}

// Ordinal is the owner-issued dense candidate address. It is an alias of
// EndpointOrdinal: there is no second equality candidate index.
func (row BinaryEquality) Ordinal() (uint32, bool) {
	return row.EndpointOrdinal()
}

// Write projects the owner-issued equality write coordinate from the sealed
// endpoint vector.
func (row BinaryEquality) Write() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointWrite)
}

// Left projects the first owner-issued equality input coordinate.
func (row BinaryEquality) Left() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointLeft)
}

// Right projects the second owner-issued equality input coordinate.
func (row BinaryEquality) Right() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointRight)
}

// BinaryEqualityForArtifactOccurrence resolves the owner-issued equality row
// for one mounted Program occurrence. Its endpoint ordinal is subsequently
// redeemed through BinaryEqualityAt, which addresses the shared endpoint
// table directly.
func (schema *Schema) BinaryEqualityForArtifactOccurrence(module, occurrence identity.ContentID) (BinaryEquality, bool) {
	if schema == nil || !module.Available() || !occurrence.Available() {
		return BinaryEquality{}, false
	}
	row, ok := schema.BinaryEquality(module, occurrence)
	if !ok || !schema.OwnsBinaryEquality(row) {
		return BinaryEquality{}, false
	}
	_, vectorOK := row.EndpointVector()
	return row, vectorOK
}

// BinaryEqualityOrdinal returns the shared endpoint-table ordinal of one
// owner-issued equality row.
func (schema *Schema) BinaryEqualityOrdinal(row BinaryEquality) (uint32, bool) {
	if schema == nil || !schema.OwnsBinaryEquality(row) {
		return 0, false
	}
	return row.EndpointOrdinal()
}

// BinaryEqualityAt redeems a dense equality candidate by the ordinal of the
// existing endpoint table. Endpoint rows belonging to another family refuse;
// they remain part of the shared denominator and are never renumbered.
func (schema *Schema) BinaryEqualityAt(index int) (BinaryEquality, bool) {
	if schema == nil || !schema.endpointsSealed() || index < 0 || index >= len(schema.endpoints) {
		return BinaryEquality{}, false
	}
	endpoint := schema.endpoints[index]
	if endpoint.family != endpointFamilyEquality {
		return BinaryEquality{}, false
	}
	row, ok := schema.binaryEqualities[endpoint.key]
	if !ok || row.endpoints != uint32(index+1) || !schema.OwnsBinaryEquality(row) {
		return BinaryEquality{}, false
	}
	return row, true
}

// EndpointOrdinal is the one dense candidate address BinaryOrder uses. It is
// the ordinal in the Schema's shared endpoint table, not an order
// family-local index.
func (row BinaryOrder) EndpointOrdinal() (uint32, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	ordinal, ordinalOK := vector.Ordinal()
	if !ordinalOK || uint64(ordinal) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(ordinal), true
}

// Ordinal is the owner-issued dense candidate address. It is an alias of
// EndpointOrdinal: there is no second order candidate index.
func (row BinaryOrder) Ordinal() (uint32, bool) {
	return row.EndpointOrdinal()
}

// Write projects the owner-issued order write coordinate from the sealed
// endpoint vector.
func (row BinaryOrder) Write() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointWrite)
}

// Left projects the first owner-issued order input coordinate.
func (row BinaryOrder) Left() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointLeft)
}

// Right projects the second owner-issued order input coordinate.
func (row BinaryOrder) Right() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointRight)
}

// BinaryOrderForArtifactOccurrence resolves the owner-issued order row for
// one mounted Program occurrence. Its endpoint ordinal is subsequently
// redeemed through BinaryOrderAt, which addresses the shared endpoint table
// directly.
func (schema *Schema) BinaryOrderForArtifactOccurrence(module, occurrence identity.ContentID) (BinaryOrder, bool) {
	if schema == nil || !module.Available() || !occurrence.Available() {
		return BinaryOrder{}, false
	}
	row, ok := schema.BinaryOrder(module, occurrence)
	if !ok || !schema.OwnsBinaryOrder(row) {
		return BinaryOrder{}, false
	}
	_, vectorOK := row.EndpointVector()
	return row, vectorOK
}

// BinaryOrderOrdinal returns the shared endpoint-table ordinal of one
// owner-issued order row.
func (schema *Schema) BinaryOrderOrdinal(row BinaryOrder) (uint32, bool) {
	if schema == nil || !schema.OwnsBinaryOrder(row) {
		return 0, false
	}
	return row.EndpointOrdinal()
}

// BinaryOrderAt redeems a dense order candidate by the ordinal of the
// existing endpoint table. Endpoint rows belonging to another family refuse;
// they remain part of the shared denominator and are never renumbered.
func (schema *Schema) BinaryOrderAt(index int) (BinaryOrder, bool) {
	if schema == nil || !schema.endpointsSealed() || index < 0 || index >= len(schema.endpoints) {
		return BinaryOrder{}, false
	}
	endpoint := schema.endpoints[index]
	if endpoint.family != endpointFamilyOrder {
		return BinaryOrder{}, false
	}
	row, ok := schema.binaryOrders[endpoint.key]
	if !ok || row.endpoints != uint32(index+1) || !schema.OwnsBinaryOrder(row) {
		return BinaryOrder{}, false
	}
	return row, true
}

// Write projects the owner-issued narrowed-write coordinate from the sealed
// endpoint vector.
func (row PresenceRefinement) Write() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointWrite)
}

// Left projects the owner-issued guarded-read coordinate from the sealed
// endpoint vector.
func (row PresenceRefinement) Left() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointLeft)
}

// PresenceRefinementForArtifactOccurrence resolves the owner-issued
// refinement row for one mounted Program occurrence. Its endpoint ordinal is
// subsequently redeemed through PresenceRefinementAt, which addresses the
// shared endpoint table directly.
func (schema *Schema) PresenceRefinementForArtifactOccurrence(module, occurrence identity.ContentID) (PresenceRefinement, bool) {
	if schema == nil || !module.Available() || !occurrence.Available() {
		return PresenceRefinement{}, false
	}
	row, ok := schema.PresenceRefinement(module, occurrence)
	if !ok || !schema.OwnsPresenceRefinement(row) {
		return PresenceRefinement{}, false
	}
	_, vectorOK := row.EndpointVector()
	return row, vectorOK
}

// EndpointOrdinal is the one dense candidate address PresenceRefinement uses.
// It is the ordinal in the Schema's shared endpoint table, not a
// refinement family-local index.
func (row PresenceRefinement) EndpointOrdinal() (uint32, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	ordinal, ordinalOK := vector.Ordinal()
	if !ordinalOK || uint64(ordinal) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(ordinal), true
}

// Ordinal is the owner-issued dense candidate address. It is an alias of
// EndpointOrdinal: there is no second refinement candidate index.
func (row PresenceRefinement) Ordinal() (uint32, bool) {
	return row.EndpointOrdinal()
}

// PresenceRefinementOrdinal returns the shared endpoint-table ordinal of one
// owner-issued refinement row. It deliberately does not consult or create a
// family-local map.
func (schema *Schema) PresenceRefinementOrdinal(row PresenceRefinement) (uint32, bool) {
	if schema == nil || !schema.OwnsPresenceRefinement(row) {
		return 0, false
	}
	return row.EndpointOrdinal()
}

// PresenceRefinementAt redeems a dense refinement candidate by the ordinal of
// the existing endpoint table. Endpoint rows belonging to another family
// refuse; they remain part of the shared denominator and are never
// renumbered.
func (schema *Schema) PresenceRefinementAt(index int) (PresenceRefinement, bool) {
	if schema == nil || !schema.endpointsSealed() || index < 0 || index >= len(schema.endpoints) {
		return PresenceRefinement{}, false
	}
	endpoint := schema.endpoints[index]
	if endpoint.family != endpointFamilyPresenceRefinement {
		return PresenceRefinement{}, false
	}
	row, ok := schema.presenceRefinements[endpoint.key]
	if !ok || row.endpoints != uint32(index+1) || !schema.OwnsPresenceRefinement(row) {
		return PresenceRefinement{}, false
	}
	return row, true
}

// EndpointOrdinal is the one dense candidate address ModuleLoadCall uses. It
// is the ordinal in the Schema's shared endpoint table, not a module-load
// family-local index, so the rule that folds these rows addresses them through
// the directory every other endpoint family is already addressed by.
func (row ModuleLoadCall) EndpointOrdinal() (uint32, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	ordinal, ordinalOK := vector.Ordinal()
	if !ordinalOK || uint64(ordinal) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(ordinal), true
}

// Ordinal is the owner-issued dense candidate address. It is an alias of
// EndpointOrdinal: there is no second module-load candidate index.
func (row ModuleLoadCall) Ordinal() (uint32, bool) {
	return row.EndpointOrdinal()
}

// Result projects the owner-issued call-result coordinate this row publishes
// at. The endpoint vector is the authority for the declaration, so a pre-seal
// or foreign row refuses.
func (row ModuleLoadCall) Result() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointWrite)
}

// Argument projects the owner-issued coordinate of the single actual the
// scoped loader is applied to.
func (row ModuleLoadCall) Argument() (Coordinate, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return Coordinate{}, false
	}
	return vector.Coordinate(EndpointLeft)
}

// ModuleLoadCallForMountedOccurrence resolves the owner-issued module-load row
// for one mounted Program call occurrence. Its candidate ordinal is
// subsequently redeemed through ModuleLoadCallAt, which addresses the shared
// endpoint table directly.
func (schema *Schema) ModuleLoadCallForMountedOccurrence(module, occurrence identity.ContentID) (ModuleLoadCall, bool) {
	if schema == nil || !module.Available() || !occurrence.Available() {
		return ModuleLoadCall{}, false
	}
	row, ok := schema.ModuleLoadCall(module, occurrence)
	if !ok || !schema.OwnsModuleLoadCall(row) {
		return ModuleLoadCall{}, false
	}
	_, vectorOK := row.EndpointVector()
	return row, vectorOK
}

// ModuleLoadCallOrdinal returns the shared endpoint-table ordinal of one
// owner-issued module-load row.
func (schema *Schema) ModuleLoadCallOrdinal(row ModuleLoadCall) (uint32, bool) {
	if schema == nil || !schema.OwnsModuleLoadCall(row) {
		return 0, false
	}
	return row.EndpointOrdinal()
}

// ModuleLoadCallAt redeems a dense module-load candidate by the ordinal of the
// existing endpoint table. Endpoint rows belonging to another family refuse;
// they remain part of the shared denominator and are never renumbered.
func (schema *Schema) ModuleLoadCallAt(index int) (ModuleLoadCall, bool) {
	if schema == nil || !schema.endpointsSealed() || index < 0 || index >= len(schema.endpoints) {
		return ModuleLoadCall{}, false
	}
	endpoint := schema.endpoints[index]
	if endpoint.family != endpointFamilyModuleLoad {
		return ModuleLoadCall{}, false
	}
	row, ok := schema.moduleLoadCalls[endpoint.key]
	if !ok || row.endpoints != uint32(index+1) || !schema.OwnsModuleLoadCall(row) {
		return ModuleLoadCall{}, false
	}
	return row, true
}

func (row BinaryEquality) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}

func (row BinaryOrder) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}

func (row PresenceRefinement) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}

func (row ModuleLoadCall) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}

func (row RuntimeKindCall) Endpoint(role EndpointRole) (uint64, bool) {
	vector, ok := row.EndpointVector()
	if !ok {
		return 0, false
	}
	return vector.Index(role)
}
