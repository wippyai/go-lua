package value

import (
	"math"

	"github.com/wippyai/go-lua/analysis/domain/heap"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programsource "github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/target"
)

// Presence is the non-nil/nil projection of a Value relation.  Bottom means
// no reachable concrete alternative, while nil is an ordinary absent runtime
// value.  This distinction avoids the old product's ambiguous presence axis.
type Presence uint8

const (
	PresenceNone    Presence = 0
	PresenceAbsent  Presence = 1 // nil
	PresencePresent Presence = 2 // every non-nil Lua value
)

func (presence Presence) HasAbsent() bool  { return presence&PresenceAbsent != 0 }
func (presence Presence) HasPresent() bool { return presence&PresencePresent != 0 }

// Truth is the read-only Lua truthiness projection.  It is deliberately not
// a Boolean fact plane; Program decision Rules consume this projection.
type Truth uint8

const (
	TruthNone  Truth = 0
	TruthFalse Truth = 1
	TruthTrue  Truth = 2
)

func (truth Truth) MayBeFalse() bool { return truth&TruthFalse != 0 }
func (truth Truth) MayBeTrue() bool  { return truth&TruthTrue != 0 }

// ReferenceKind is the reference-capable runtime portion of Value.  Opaque
// is a non-nil reference of an exact structural root whose concrete runtime
// family is deliberately not claimed by Target/Link.
type ReferenceKind uint8

const (
	ReferenceInvalid ReferenceKind = iota
	ReferenceTable
	ReferenceFunction
	ReferenceThread
	ReferenceUserdata
	ReferenceOpaque
	ReferenceCount
)

func (kind ReferenceKind) valid() bool { return kind > ReferenceInvalid && kind < ReferenceCount }

type atomKind uint8

const (
	atomInvalid atomKind = iota
	atomNil
	atomFalse
	atomTrue
	atomPrimitive
	// atomNaN is the one Value-owned exact class of numeric values that Lua
	// rejects as table keys. Numeric/equality still owns IEEE payloads; Value
	// retains only the key-validity distinction required by Heap.
	atomNaN
	// atomLiteral is one authored, storable Lua literal.  Its identity is the
	// existing normalized Link Key; the recurrent Value image still retains
	// only this Schema-local dense atom ID.
	atomLiteral
	atomReference
	atomOpaqueKind
	atomOpaqueReference
)

type referenceSource uint8

const (
	referenceSourceInvalid referenceSource = iota
	referenceSourceAllocation
	referenceSourceBoot
	referenceSourceEndpoint
	referenceSourceCallable
	referenceSourceScopedLoader
	referenceSourceRuntimeType
)

// Atom is an owner-bound capability for exactly one sealed correlated
// alternative.  It can construct a Value only through its owning Schema.
// Atom is not State: recurrent Value state stores compact IDs, not a pointer
// to an allocation root, a Link ContentID, a string, or a type graph.
type Atom struct {
	schema *Schema
	id     uint32 // one based
}

func (atom Atom) valid() bool {
	return atom.schema != nil && atom.id != 0 && int(atom.id) <= len(atom.schema.atoms)
}

// OwnsAtom reports whether atom was issued by this exact Schema. Structural
// child projections use this owner fence before consuming one correlated
// alternative, so independently sealed same-content Links cannot mix atoms.
func (schema *Schema) OwnsAtom(atom Atom) bool {
	return schema != nil && atom.valid() && atom.schema == schema
}

// Kind reports the exact atom form without exposing its private identity
// payload.  Reference and opaque alternatives are intentionally distinct.
func (atom Atom) Kind() uint8 {
	if !atom.valid() {
		return 0
	}
	return uint8(atom.schema.atoms[atom.id-1].kind)
}

// RuntimeKinds is the atom's exact kind projection.
func (atom Atom) RuntimeKinds() runtimekind.Set {
	if !atom.valid() {
		return 0
	}
	return atom.schema.atomKinds(atom.id)
}

// Truthiness is the atom's exact Lua truthiness projection.
func (atom Atom) Truthiness() Truth {
	if !atom.valid() {
		return TruthNone
	}
	return atom.schema.atomTruth(atom.id)
}

// TableKeyValidity is an exact Value atom's may projection to Lua table-key
// admissibility. It does not create Heap key identity: ExactKey and Reference
// remain the only exact-key and rooted-reference authorities.
type TableKeyValidity uint8

const (
	TableKeyNeither TableKeyValidity = 0
	TableKeyValid   TableKeyValidity = 1 << iota
	TableKeyInvalid
)

// MayBeValid reports whether this atom has at least one legal Lua table-key
// realization.
func (validity TableKeyValidity) MayBeValid() bool { return validity&TableKeyValid != 0 }

// MayBeInvalid reports whether this atom may be nil or NaN, both of which Lua
// rejects as table keys.
func (validity TableKeyValidity) MayBeInvalid() bool { return validity&TableKeyInvalid != 0 }

// TableKeyValidity is the sole Value-side table-key validity projection.
// It is exact for literal, rooted, nil, and NaN alternatives. The opaque
// numeric fallback deliberately remains both valid and invalid because Value
// does not own Numeric's remaining IEEE distinctions.
func (atom Atom) TableKeyValidity() TableKeyValidity {
	if !atom.valid() {
		return TableKeyNeither
	}
	row := atom.schema.atoms[atom.id-1]
	switch row.kind {
	case atomNil, atomNaN:
		return TableKeyInvalid
	case atomOpaqueKind:
		if row.runtime == runtimekind.Number {
			return TableKeyValid | TableKeyInvalid
		}
		return TableKeyValid
	case atomFalse, atomTrue, atomPrimitive, atomLiteral, atomReference, atomOpaqueReference:
		return TableKeyValid
	default:
		return TableKeyNeither
	}
}

// ExactKey projects the existing normalized Link key only for an authored
// storable literal alternative.  Nil and NaN deliberately have no key atom;
// callers must retain their separate Lua semantics rather than fabricating a
// table-key identity.  The dense Key is Link-owned, not a Value key space.
func (atom Atom) ExactKey() (linkproject.Key, bool) {
	if !atom.valid() {
		return linkproject.Key{}, false
	}
	row := atom.schema.atoms[atom.id-1]
	return row.key, row.kind == atomLiteral && row.hasKey
}

// Reference returns the exact structural root and role only for a rooted
// reference atom.  Opaque-reference alternatives intentionally have no root.
func (atom Atom) Reference() (Reference, materialization.Role, bool) {
	if !atom.valid() {
		return Reference{}, materialization.Invalid, false
	}
	row := atom.schema.atoms[atom.id-1]
	if row.kind != atomReference || row.reference == 0 || !row.role.Valid() {
		return Reference{}, materialization.Invalid, false
	}
	return Reference{schema: atom.schema, id: row.reference}, row.role, true
}

// Reference is an owner-bound handle for one Link structural root.  It is
// cold declaration/query vocabulary, never a recurrent State field.
type Reference struct {
	schema *Schema
	id     uint32 // one based
}

func (reference Reference) valid() bool {
	return reference.schema != nil && reference.id != 0 && int(reference.id) <= len(reference.schema.references)
}

// OwnsReference reports whether reference was issued by this exact Schema.
// Structural child projections use this owner fence before translating a
// Value reference into another domain's relation operand.
func (schema *Schema) OwnsReference(reference Reference) bool {
	return schema != nil && reference.valid() && reference.schema == schema
}

// Kind reports the sealed reference family.  Opaque means Link/Target proved
// nominal freshness but did not prove one concrete Lua reference family.
func (reference Reference) Kind() ReferenceKind {
	if !reference.valid() {
		return ReferenceInvalid
	}
	return reference.schema.references[reference.id-1].kind
}

// AllocationKey returns the exact Heap allocation coordinate carried by this
// Value reference.  Heap is the sole root authority; Value retains only the
// owner-issued key needed to correlate reference alternatives.
func (reference Reference) AllocationKey() (heap.Key, bool) {
	if !reference.valid() {
		return heap.Key{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.allocation, row.source == referenceSourceAllocation
}

// BootRoot returns the exact actor-local Link boot root for this reference.
func (reference Reference) BootRoot() (linkhost.BootRoot, bool) {
	if !reference.valid() {
		return linkhost.BootRoot{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.boot, row.source == referenceSourceBoot
}

// Endpoint returns the exact host Function endpoint for this reference.
func (reference Reference) Endpoint() (linkboundary.Endpoint, bool) {
	if !reference.valid() {
		return linkboundary.Endpoint{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.endpoint, row.source == referenceSourceEndpoint
}

// Callable returns an exact admitted or explicitly denied bootstrap callable
// Seed. Its disposition remains Link/Call-owned; Value retains only runtime
// identity so aliases and table reads do not collapse a denied callable to an
// opaque function.
func (reference Reference) Callable() (linkboundary.Seed, bool) {
	if !reference.valid() {
		return linkboundary.Seed{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.callable, row.source == referenceSourceCallable
}

// ScopedLoader returns the exact Target operation classified by Boundary as
// the scoped require ingress. Unlike Callable, this reference carries no
// global seed: Call resolves its shard-local loader from the bound
// Application at dispatch time.
func (reference Reference) ScopedLoader() (target.Operation, bool) {
	if !reference.valid() {
		return 0, false
	}
	row := reference.schema.references[reference.id-1]
	return row.operation, row.source == referenceSourceScopedLoader
}

// TypeValueSource returns the existing Link Value used for one executable
// Program TypeValue source. Its descriptor remains TypeValue-owned.
func (reference Reference) TypeValueSource() (linkboundary.Value, bool) {
	if !reference.valid() {
		return linkboundary.Value{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.value, row.source == referenceSourceRuntimeType
}

type referenceRow struct {
	source     referenceSource
	kind       ReferenceKind
	allocation heap.Key
	boot       linkhost.BootRoot
	endpoint   linkboundary.Endpoint
	callable   linkboundary.Seed
	operation  target.Operation
	value      linkboundary.Value
}

type atomRow struct {
	kind      atomKind
	runtime   runtimekind.Kind
	reference uint32
	role      materialization.Role
	key       linkproject.Key
	hasKey    bool
	// literalFalsy is a private decoded property of an atomLiteral's existing
	// normalized Link Key. It is not a second truth axis and is valid only for
	// the exact boolean-false key.
	literalFalsy bool
}

// coordinateRow is the one sealed reverse projection for an existing Link
// Value. coordinate is the already-declared one-based Value coordinate; atom
// is optional because dynamic Values acquire facts only from later Rules.
// source is the optional prebuilt singleton for atom. Keeping all three in
// this row lets Source, SourceValue, and CoordinateFor share one lookup
// authority rather than retaining parallel maps or identity planes.
type coordinateRow struct {
	coordinate uint32
	atom       uint32
	source     Value
}

// Schema is the sealed Value alternative universe for one Link.  It is
// neither a registry nor a second identity/key space: source Values remain
// Link Values and every structural reference delegates back to Link's
// existing opaque handle. Its complete finite alternative universe is sealed
// before any Factor declaration; Schema queries never mutate it.
type Schema struct {
	source *link.Link
	heap   heap.Schema

	atoms       []atomRow
	atomByRow   map[atomRow]uint32
	coordinates map[linkboundary.Value]coordinateRow // complete exact ownerful Value range
	exactKeys   map[keyspace.LiteralValue]linkproject.Key

	// storageTransfers is Value's complete fixed Read/Bind/Write relation.
	// It is derived once from canonical Program terms and existing Link Values;
	// Link deliberately retains no computation-storage projection for it.
	storageTransfers        []storageTransferRow
	storageTransferOrdinals map[StorageTransferRef]uint32

	references   []referenceRow
	allocRefs    map[heap.Key]uint32
	bootRefs     map[linkhost.BootRoot]uint32
	endpointRefs map[linkboundary.Endpoint]uint32
	callableRefs map[linkboundary.Seed]uint32
	scopedLoader uint32
	typeRefs     map[linkboundary.Value]uint32

	capabilities []linkhost.ProviderCapability
	capabilityID map[linkhost.ProviderCapability]uint32
	capWords     int

	capabilitySeeds []linkhost.ProviderCapabilitySeed
	hostMembers     []hostMember

	potential uint64
	bottom    Value
	top       Value

	// These cold precomputed reductions are immutable views over dense atom
	// rows. They keep common projections allocation-free and do not create a
	// second state plane. Stored-value reduction has three disjoint local
	// classes: non-reference, untracked reference, and tracked reference. The
	// two range boundaries are private representation order, never identity.
	forRuntimeKinds    []Value
	atomTop            []Value
	storedNoneTop      Value
	storedUnknownTop   Value
	firstStoredUnknown uint32
	firstStoredExact   uint32
}

// OwnsHeapSchema reports whether candidate is the exact immutable Heap
// authority retained when this Value schema was sealed.  Heap Schema values
// are owner handles, not content identities: two independent seals of the
// same Link intentionally compare unequal here and must never share Value
// atoms with Heap keys or index topology.
func (schema *Schema) OwnsHeapSchema(candidate heap.Schema) bool {
	return schema != nil && schema.heap.Valid() && candidate.Valid() && schema.heap == candidate
}

// Coordinate is an exact Schema-issued Value Factor coordinate. Its dense
// position is private declaration layout; the Schema pointer fences the
// coordinate to one exact Link even when another Link has the same content
// and the same private Value index.
type Coordinate struct {
	schema *Schema
	index  uint32 // one based
}

type hostMember struct {
	capability linkhost.ProviderCapability
	output     linkboundary.Value
	endpoint   linkboundary.Endpoint
}

// Seal derives the complete finite Value alternative vocabulary from the
// already-sealed Link.  It does not inspect AST/binder state, materialize a
// candidate product, or create a second raw Program identity.
func Seal(source *link.Link, heaps heap.Schema) (*Schema, bool) {
	if source == nil || !source.ContentID().Available() || heaps.LinkContentID() != source.ContentID() || heaps.Link() != source {
		return nil, false
	}
	schema := &Schema{
		source:                  source,
		heap:                    heaps,
		atomByRow:               make(map[atomRow]uint32),
		coordinates:             make(map[linkboundary.Value]coordinateRow, source.Boundary().Values().Count()),
		exactKeys:               make(map[keyspace.LiteralValue]linkproject.Key, source.Project().Keys().Count()),
		storageTransferOrdinals: make(map[StorageTransferRef]uint32),
		allocRefs:               make(map[heap.Key]uint32),
		bootRefs:                make(map[linkhost.BootRoot]uint32),
		endpointRefs:            make(map[linkboundary.Endpoint]uint32),
		callableRefs:            make(map[linkboundary.Seed]uint32),
		typeRefs:                make(map[linkboundary.Value]uint32),
		capabilityID:            make(map[linkhost.ProviderCapability]uint32),
	}
	if !schema.sealCoordinates() || !schema.sealStorageTransfers() || !schema.sealExactKeys() || !schema.sealCapabilities() || !schema.sealSources() || !schema.sealBootstrapCallables() ||
		!schema.sealOpaqueAlternatives() || !schema.sealLiteralSourceAtoms() || !schema.sealTargetLiteralAtoms() || !schema.sealStoredUnknownAtoms() || !schema.sealStoredExactAtoms() ||
		!schema.sealReferenceSourceAtoms() || !schema.finish() || !schema.sealSourceValues() {
		return nil, false
	}
	return schema, true
}

// sealExactKeys imports Link's already-normalized key universe once during
// Schema construction.  It is intentionally a cold reverse projection: no
// Value fact stores literal payloads, strings, or a duplicate key identity.
func (schema *Schema) sealExactKeys() bool {
	if schema == nil || schema.source == nil || schema.exactKeys == nil {
		return false
	}
	for index := 0; index < schema.source.Project().Keys().Count(); index++ {
		key, ok := schema.source.Project().Keys().At(index)
		if !ok {
			return false
		}
		literal, ok := schema.source.Project().Keys().Exact(key)
		if !ok {
			return false
		}
		if _, duplicate := schema.exactKeys[literal]; duplicate {
			return false
		}
		schema.exactKeys[literal] = key
	}
	return len(schema.exactKeys) == schema.source.Project().Keys().Count()
}

// sealCoordinates enumerates Link's already-canonical Value range once during
// cold Schema construction. It assigns no new identity: the stored one-based
// coordinate is exactly CoordinateAt's existing declaration position.
func (schema *Schema) sealCoordinates() bool {
	if schema == nil || schema.source == nil || len(schema.coordinates) != 0 || !validCoordinateCount(schema.source.Boundary().Values().Count()) {
		return false
	}
	for index := 0; index < schema.source.Boundary().Values().Count(); index++ {
		value, ok := schema.source.Boundary().Values().At(index)
		if !ok {
			return false
		}
		if _, duplicate := schema.coordinates[value]; duplicate {
			return false
		}
		schema.coordinates[value] = coordinateRow{coordinate: uint32(index + 1)}
	}
	return len(schema.coordinates) == schema.source.Boundary().Values().Count()
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))
}

func (schema *Schema) sealCapabilities() bool {
	for index := 0; index < schema.source.Host().Capabilities().Count(); index++ {
		capability, ok := schema.source.Host().Capabilities().At(index)
		if !ok || schema.capabilityID[capability] != 0 {
			return false
		}
		schema.capabilities = append(schema.capabilities, capability)
		schema.capabilityID[capability] = uint32(len(schema.capabilities))
	}
	schema.capWords = (len(schema.capabilities) + 63) / 64
	return true
}

// sealSources admits direct Heap allocation coordinates first, then the
// remaining Link-owned literal/bootstrap source families.  Heap alone
// enumerates allocation and fresh-root structure; Link no longer carries an
// allocation source row that Value could replay or reinterpret.
func (schema *Schema) sealSources() bool {
	contract, ok := schema.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	if !schema.sealAllocationReferences() {
		return false
	}
	for index := 0; index < schema.source.Host().BootRoots().Count(); index++ {
		root, ok := schema.source.Host().BootRoots().At(index)
		if !ok || !schema.addBootReference(contract, root) {
			return false
		}
	}
	endpoints := schema.source.Boundary().Endpoints()
	for index := 0; index < endpoints.Count(); index++ {
		endpoint, ok := endpoints.At(index)
		if !ok {
			return false
		}
		// An endpoint is nominal even when another endpoint names the same
		// operation. Query the operation only to reject a malformed Boundary
		// row; do not collapse the nominal endpoint to its callable Seed.
		if _, ok := endpoints.Operation(endpoint); !ok || !schema.addEndpointReference(endpoint) {
			return false
		}
	}
	if !schema.forEachExecutableTypeValue(func(value linkboundary.Value) bool {
		return schema.addTypeValueReference(value)
	}) {
		return false
	}
	if !schema.sealScopedLoader() {
		return false
	}
	for index := 0; index < schema.source.Host().CapabilitySeeds().Count(); index++ {
		seed, ok := schema.source.Host().CapabilitySeeds().At(index)
		if !ok {
			return false
		}
		capability, ok := schema.source.Host().CapabilitySeeds().Capability(seed)
		if !ok || schema.capabilityID[capability] == 0 {
			return false
		}
		schema.capabilitySeeds = append(schema.capabilitySeeds, seed)
	}
	for index := 0; index < schema.source.Host().Members().Count(); index++ {
		_, _, capability, _, output, endpoint, _, ok := schema.source.Host().Members().At(index)
		if !ok || schema.capabilityID[capability] == 0 || schema.endpointRefs[endpoint] == 0 {
			return false
		}
		schema.hostMembers = append(schema.hostMembers, hostMember{capability: capability, output: output, endpoint: endpoint})
	}
	return true
}

// sealScopedLoader admits the one nominal Function reference for Target's
// scoped require initial value. Boundary intentionally emits no global seed
// for this operation; Call later chooses the loader seed from the bound
// Application's mounted shard.
func (schema *Schema) sealScopedLoader() bool {
	if schema == nil || schema.source == nil {
		return false
	}
	require, hasRequire := schema.source.Boundary().RequireOperation()
	if !hasRequire {
		return true
	}
	contract, contractOK := schema.source.Boundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	return schema.visitTargetInitialValues(func(initial target.InitialValue) bool {
		kind, kindOK := contract.InitialValueKind(initial)
		if !kindOK || kind != target.InitialValueOperation {
			return true
		}
		op, opOK := contract.InitialValueOperation(initial)
		if !opOK || op != require || schema.scopedLoader != 0 {
			return opOK
		}
		id := schema.addReference(referenceRow{source: referenceSourceScopedLoader, kind: ReferenceFunction, operation: op})
		if id == 0 {
			return false
		}
		schema.scopedLoader = id
		return true
	})
}

// sealAllocationReferences imports every Heap-owned allocation coordinate.
// Program table/closure roots and Target fresh-result roots share the same
// Heap allocation key and therefore the same reference provenance. Fresh
// roots deliberately acquire no Link source coordinate: their guarded
// Call/Target rule still owns result construction and kind admission.
func (schema *Schema) sealAllocationReferences() bool {
	if schema == nil || !schema.heap.LinkContentID().Available() {
		return false
	}
	for index := 0; index < schema.heap.KeyCount(); index++ {
		key, ok := schema.heap.KeyAt(index)
		if !ok {
			return false
		}
		if key.Kind() != heap.RootAllocation {
			continue
		}
		if !schema.addAllocationReference(key) {
			return false
		}
	}
	return true
}

func (schema *Schema) addAllocationReference(key heap.Key) bool {
	if schema == nil || !schema.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return false
	}
	if schema.allocRefs[key] != 0 {
		return true
	}
	refKind := ReferenceInvalid
	if _, _, kind, sourceRoot := key.ProgramAllocation(); sourceRoot {
		switch kind {
		case heap.AllocationTable:
			refKind = ReferenceTable
		case heap.AllocationClosure:
			refKind = ReferenceFunction
		default:
			return false
		}
	} else {
		// A Target fresh result has no authored Program occurrence or Link
		// source coordinate. Its Heap key is nevertheless the exact owner-issued
		// reference identity, while its nominal runtime kind remains guarded by
		// Call/Target. Keep that uncertainty as one opaque reference family.
		if _, _, _, _, _, _, fresh := key.FreshResult(); !fresh {
			return false
		}
		refKind = ReferenceOpaque
	}
	if !refKind.valid() {
		return false
	}
	id := schema.addReference(referenceRow{source: referenceSourceAllocation, kind: refKind, allocation: key})
	if id == 0 {
		return false
	}
	return true
}

func (schema *Schema) addBootReference(contract *target.Contract, root linkhost.BootRoot) bool {
	if schema.bootRefs[root] != 0 {
		return true
	}
	_, initial, ok := schema.source.Host().BootRoots().Mapping(root)
	if !ok {
		return false
	}
	shape, ok := contract.InitialRootBootShape(initial)
	if !ok {
		return false
	}
	aggregate, ok := contract.BootShapeAggregate(shape)
	if !ok || (aggregate != target.BootAggregateTable && aggregate != target.BootAggregateMetatable) {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceBoot, kind: ReferenceTable, boot: root}) != 0
}

func (schema *Schema) addEndpointReference(endpoint linkboundary.Endpoint) bool {
	if schema.endpointRefs[endpoint] != 0 {
		return true
	}
	if _, ok := schema.source.Boundary().Endpoints().Operation(endpoint); !ok {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceEndpoint, kind: ReferenceFunction, endpoint: endpoint}) != 0
}

func (schema *Schema) sealBootstrapCallables() bool {
	add := func(value target.InitialValue) bool {
		seed, _, callable := schema.source.Boundary().Seeds().BootstrapCallable(value)
		return !callable || schema.addCallableReference(seed)
	}
	return schema.visitTargetInitialValues(add)
}

// visitTargetInitialValues is the exact Target-reachable initial-value image.
// It is used only while sealing Value's finite owner-local atom universe;
// callers retain no Target value in recurrent State.
func (schema *Schema) visitTargetInitialValues(visit func(target.InitialValue) bool) bool {
	if schema == nil || schema.source == nil || visit == nil {
		return false
	}
	contract, ok := schema.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, rootOK := contract.InitialRootAt(index)
		shape, shapeOK := contract.InitialRootBootShape(root)
		value, valueOK := contract.BootShapeValue(shape)
		if !rootOK || !shapeOK || !valueOK || !visit(value) {
			return false
		}
	}
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, _, value, _, valueOK := contract.InitialEntryAt(index)
		if !valueOK || !visit(value) {
			return false
		}
	}
	for index := 0; index < contract.InitialBindingCount(); index++ {
		_, _, value, _, _, valueOK := contract.InitialBindingAt(index)
		if !valueOK || !visit(value) {
			return false
		}
	}
	return true
}

func (schema *Schema) addCallableReference(seed linkboundary.Seed) bool {
	if schema.callableRefs[seed] != 0 {
		return true
	}
	if _, _, _, ok := schema.source.Boundary().Seeds().CallableDisposition(seed); !ok {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceCallable, kind: ReferenceFunction, callable: seed}) != 0
}

func (schema *Schema) addTypeValueReference(value linkboundary.Value) bool {
	if schema.typeRefs[value] != 0 {
		return true
	}
	return schema.addReference(referenceRow{source: referenceSourceRuntimeType, kind: ReferenceOpaque, value: value}) != 0
}

func (schema *Schema) addReference(row referenceRow) uint32 {
	if !row.kind.valid() {
		return 0
	}
	var existing uint32
	switch row.source {
	case referenceSourceAllocation:
		if !schema.heap.OwnsKey(row.allocation) || row.allocation.Kind() != heap.RootAllocation {
			return 0
		}
		existing = schema.allocRefs[row.allocation]
	case referenceSourceBoot:
		existing = schema.bootRefs[row.boot]
	case referenceSourceEndpoint:
		existing = schema.endpointRefs[row.endpoint]
	case referenceSourceCallable:
		existing = schema.callableRefs[row.callable]
	case referenceSourceScopedLoader:
		require, ok := schema.source.Boundary().RequireOperation()
		if !ok || row.operation == 0 || row.operation != require {
			return 0
		}
		existing = schema.scopedLoader
	case referenceSourceRuntimeType:
		existing = schema.typeRefs[row.value]
	default:
		return 0
	}
	if existing != 0 {
		return existing
	}
	if uint64(len(schema.references)) >= uint64(^uint32(0)) {
		return 0
	}
	id := uint32(len(schema.references) + 1)
	schema.references = append(schema.references, row)
	switch row.source {
	case referenceSourceAllocation:
		schema.allocRefs[row.allocation] = id
	case referenceSourceBoot:
		schema.bootRefs[row.boot] = id
	case referenceSourceEndpoint:
		schema.endpointRefs[row.endpoint] = id
	case referenceSourceCallable:
		schema.callableRefs[row.callable] = id
	case referenceSourceScopedLoader:
		schema.scopedLoader = id
	case referenceSourceRuntimeType:
		schema.typeRefs[row.value] = id
	}
	return id
}

func (schema *Schema) sealOpaqueAlternatives() bool {
	if schema.addAtom(atomRow{kind: atomNil}) == 0 || schema.addAtom(atomRow{kind: atomFalse}) == 0 || schema.addAtom(atomRow{kind: atomTrue}) == 0 {
		return false
	}
	for _, kind := range []runtimekind.Kind{runtimekind.Number, runtimekind.String} {
		if schema.addAtom(atomRow{kind: atomPrimitive, runtime: kind}) == 0 {
			return false
		}
	}
	if schema.addAtom(atomRow{kind: atomNaN, runtime: runtimekind.Number}) == 0 {
		return false
	}
	// atomNil is already the sole exact nil alternative. Keeping a second
	// opaque-nil atom makes a non-nil filter discontinuous and falsely suggests
	// that Lua has more than one nil identity.
	// Scalar opaque kinds are non-reference alternatives. Reference-capable
	// opaque kinds are emitted with the Unknown stored class below, so finite
	// stored reductions can select each class as one immutable image range.
	for kind := runtimekind.Boolean; kind < runtimekind.Table; kind++ {
		if schema.addAtom(atomRow{kind: atomOpaqueKind, runtime: kind}) == 0 {
			return false
		}
	}
	return true
}

// sealLiteralSourceAtoms emits authored literal atoms before stored-reference
// atoms. That order is semantic only in the sense that it makes the three
// stored projection classes contiguous immutable image ranges; it never
// exposes an atom ordinal as an identity.
func (schema *Schema) sealLiteralSourceAtoms() bool {
	for index := 0; index < schema.source.Boundary().Values().Count(); index++ {
		value, ok := schema.source.Boundary().Values().At(index)
		if !ok {
			return false
		}
		family, _, ok := schema.sourceLiteral(value)
		if !ok {
			continue
		}
		switch family {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
			keyspace.FamilyFloat, keyspace.FamilyString:
			atom, subject, present, ok := schema.sourceAtomForValue(value)
			if !ok || !schema.assignSourceAtom(atom, subject, present) {
				return false
			}
		}
	}
	return true
}

// sealTargetLiteralAtoms makes Target literals use the same owner-issued
// atom as an equal Program literal whenever Link has already issued its
// normalized key. It deliberately does not decode/search raw key payloads:
// InitialValueExactKey is the sole Link authority, and NaN has no key atom.
func (schema *Schema) sealTargetLiteralAtoms() bool {
	contract, ok := schema.source.Boundary().Target()
	if !ok || contract == nil {
		return false
	}
	return schema.visitTargetInitialValues(func(value target.InitialValue) bool {
		kind, ok := contract.InitialValueKind(value)
		if !ok {
			return false
		}
		runtime := runtimekind.Invalid
		switch kind {
		case target.InitialValueBoolean:
			runtime = runtimekind.Boolean
		case target.InitialValueInteger, target.InitialValueFloat:
			runtime = runtimekind.Number
		case target.InitialValueString:
			runtime = runtimekind.String
		default:
			return true
		}
		key, exact := schema.source.Project().Keys().ForInitial(contract, value)
		if !exact {
			return true
		}
		literal, literalOK := schema.source.Project().Keys().Exact(key)
		if !literalOK {
			return false
		}
		return schema.addAtom(atomRow{
			kind:         atomLiteral,
			runtime:      runtime,
			key:          key,
			hasKey:       true,
			literalFalsy: literal.Kind == keyspace.LiteralBool && !literal.Bool,
		}) != 0
	})
}

// sealStoredUnknownAtoms closes the non-reference prefix and emits every
// reference alternative that cannot name one tracked structural child. In
// particular, a source allocation's Exact role is deliberately Unknown:
// Value's tracked recurrence begins at Recent, not at a raw allocation
// occurrence. Endpoint, callable, runtime-TypeValue, opaque-reference, and
// unsupported boot-role alternatives are likewise retained as Unknown rather
// than silently dropped.
func (schema *Schema) sealStoredUnknownAtoms() bool {
	if schema == nil || schema.firstStoredUnknown != 0 || schema.firstStoredExact != 0 {
		return false
	}
	schema.firstStoredUnknown = uint32(len(schema.atoms) + 1)
	for kind := runtimekind.Table; kind < runtimekind.Count; kind++ {
		if schema.addAtom(atomRow{kind: atomOpaqueKind, runtime: kind}) == 0 {
			return false
		}
	}
	for kind := ReferenceTable; kind < ReferenceCount; kind++ {
		if schema.addAtom(atomRow{kind: atomOpaqueReference, runtime: runtimeForReference(kind)}) == 0 {
			return false
		}
	}
	for reference := range schema.references {
		if !schema.addReferenceAtoms(uint32(reference+1), false) {
			return false
		}
	}
	return true
}

// sealStoredExactAtoms emits only rooted alternatives that can name one
// tracked structural child: allocation Recent/Summary and boot Exact. Exact
// source allocation atoms intentionally remain in the preceding Unknown
// class until their allocation contribution strongly writes a Recent result.
func (schema *Schema) sealStoredExactAtoms() bool {
	if schema == nil || schema.firstStoredUnknown == 0 || schema.firstStoredExact != 0 {
		return false
	}
	schema.firstStoredExact = uint32(len(schema.atoms) + 1)
	for reference := range schema.references {
		if !schema.addReferenceAtoms(uint32(reference+1), true) {
			return false
		}
	}
	return true
}

func (schema *Schema) sealReferenceSourceAtoms() bool {
	return schema.forEachExecutableTypeValue(func(value linkboundary.Value) bool {
		atom, subject, present, ok := schema.sourceAtomForValue(value)
		return ok && schema.assignSourceAtom(atom, subject, present)
	})
}

// forEachExecutableTypeValue reads only the canonical Program TypeValue rows
// and validates their retained Link static resolver/expression tuple before a
// Value-domain interpretation is admitted.
func (schema *Schema) forEachExecutableTypeValue(visit func(linkboundary.Value) bool) bool {
	if schema == nil || schema.source == nil || visit == nil {
		return false
	}
	for shardIndex := 0; shardIndex < schema.source.Project().Mounts().Count(); shardIndex++ {
		shard, ok := schema.source.Project().Mounts().At(shardIndex)
		if !ok {
			return false
		}
		p, ok := schema.source.Project().Mounts().Program(shard)
		if !ok || p == nil {
			return false
		}
		resolver, ok := schema.source.Static().Namespaces().ResolverForShard(shard)
		if !ok {
			return false
		}
		typeValues := p.Flow().Authored().TypeValues()
		for index := 0; index < typeValues.Count(); index++ {
			term, ok := typeValues.At(index)
			if !ok {
				return false
			}
			if !p.Flow().Executable().Contains(term) {
				continue
			}
			if _, ok := typeValues.Get(term); !ok {
				return false
			}
			target, ok := p.Static().Operands().TypeValues().Target(term)
			if !ok {
				return false
			}
			reference, ok := p.Static().StaticTypes().Ref(target)
			if !ok {
				return false
			}
			expression, ok := schema.source.Static().Expressions().For(resolver, reference)
			if !ok {
				return false
			}
			actualReference, referenceOK := schema.source.Static().Expressions().Reference(expression)
			actualResolver, resolverOK := schema.source.Static().Expressions().Resolver(expression)
			if !referenceOK || !resolverOK || actualReference != reference || actualResolver != resolver {
				return false
			}
			value, ok := schema.source.Boundary().Values().Of(shard, term)
			if !ok || !visit(value) {
				return false
			}
		}
	}
	return true
}

func (schema *Schema) assignSourceAtom(atom uint32, value linkboundary.Value, present bool) bool {
	if !present {
		return atom == 0
	}
	row, exists := schema.coordinates[value]
	if !exists || row.atom != 0 || atom == 0 {
		return false
	}
	row.atom = atom
	schema.coordinates[value] = row
	return true
}

func (schema *Schema) addReferenceAtoms(reference uint32, exact bool) bool {
	if schema == nil || reference == 0 || int(reference) > len(schema.references) {
		return false
	}
	runtime := runtimeForReference(schema.references[reference-1].kind)
	for _, role := range []materialization.Role{materialization.Exact, materialization.Recent, materialization.Summary} {
		row := atomRow{kind: atomReference, runtime: runtime, reference: reference, role: role}
		if schema.storedExactReference(row) != exact {
			continue
		}
		if schema.addAtom(row) == 0 {
			return false
		}
	}
	return true
}

// storedExactReference recognizes the only Value alternatives that can name
// one tracked structural child. It is deliberately local to Value and derives
// only from sealed reference provenance and materialization role.
func (schema *Schema) storedExactReference(row atomRow) bool {
	if schema == nil || row.kind != atomReference || row.reference == 0 || int(row.reference) > len(schema.references) {
		return false
	}
	switch schema.references[row.reference-1].source {
	case referenceSourceAllocation:
		return row.role == materialization.Recent || row.role == materialization.Summary
	case referenceSourceBoot:
		return row.role == materialization.Exact
	default:
		return false
	}
}

// storedUnknownReference recognizes every Value alternative that may retain
// a structural or opaque reference edge but cannot name a tracked child. It
// includes raw allocation Exact occurrences, so an unmaterialized allocation
// remains observable rather than becoming Bottom at a stored read.
func (schema *Schema) storedUnknownReference(id uint32) bool {
	if schema == nil || id == 0 || int(id) > len(schema.atoms) {
		return false
	}
	row := schema.atoms[id-1]
	if schema.storedExactReference(row) {
		return false
	}
	switch row.kind {
	case atomReference, atomOpaqueReference:
		return true
	case atomOpaqueKind:
		return row.runtime >= runtimekind.Table && row.runtime < runtimekind.Count
	default:
		return false
	}
}

// storedProjectionOrderValid proves the private atom ordering used by the
// allocation-free stored reductions. The ranges are representation-only; the
// classification itself is derived from sealed Value reference provenance and
// materialization role.
func (schema *Schema) storedProjectionOrderValid() bool {
	if schema == nil || schema.firstStoredUnknown == 0 || schema.firstStoredExact == 0 || schema.firstStoredUnknown > schema.firstStoredExact || int(schema.firstStoredExact) > len(schema.atoms)+1 {
		return false
	}
	for index := range schema.atoms {
		id := uint32(index + 1)
		unknown := schema.storedUnknownReference(id)
		exact := schema.storedExactReference(schema.atoms[index])
		if exact && unknown {
			return false
		}
		switch {
		case id < schema.firstStoredUnknown:
			if unknown || exact {
				return false
			}
		case id < schema.firstStoredExact:
			if !unknown || exact {
				return false
			}
		default:
			if !exact || unknown {
				return false
			}
		}
	}
	return true
}

// sealSourceValues caches the immutable singleton carried by each
// unconditional source coordinate. SourceSeed.Result and SourceValue can
// therefore remain allocation-free without introducing a second source map.
// Dynamic coordinates retain the zero Value and are still produced only by
// their eventual Rule.
func (schema *Schema) sealSourceValues() bool {
	if schema == nil || schema.source == nil || schema.potential == 0 {
		return false
	}
	for subject, row := range schema.coordinates {
		if row.atom == 0 {
			continue
		}
		if row.source.schema != nil {
			return false
		}
		fact, ok := schema.Singleton(Atom{schema: schema, id: row.atom})
		if !ok || !schema.owns(fact) || schema.Equal(fact, schema.Bottom()) {
			return false
		}
		row.source = fact
		schema.coordinates[subject] = row
	}
	return true
}

// sourceLiteral resolves one existing Link Value through its canonical Source
// owner. Nil has no LiteralValue payload; its FamilyNil result is the direct
// Source discriminator. Non-literal Values are rejected so runtime TypeValue
// sources remain under their separate Link/static relation.
func (schema *Schema) sourceLiteral(value linkboundary.Value) (keyspace.Family, keyspace.LiteralValue, bool) {
	if schema == nil || schema.source == nil {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	shard, term, ok := schema.source.Boundary().Values().Origin(value)
	if !ok {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	p, ok := schema.source.Project().Mounts().Program(shard)
	if !ok || p == nil {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	literals := p.Source().Literals()
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		actual, _, present := literals.Nils().At(int(ordinal - 1))
		if !present || actual != term {
			return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
		}
		return keyspace.FamilyNil, keyspace.LiteralValue{}, true
	case keyspace.FamilyBool:
		actual, _, boolean, present := literals.Bools().At(int(ordinal - 1))
		if !present || actual != term {
			return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
		}
		return keyspace.FamilyBool, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: boolean}, true
	case keyspace.FamilyInteger:
		actual, _, integer, present := literals.Integers().At(int(ordinal - 1))
		if !present || actual != term {
			return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
		}
		return keyspace.FamilyInteger, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: integer}, true
	case keyspace.FamilyFloat:
		actual, _, bits, present := literals.Floats().At(int(ordinal - 1))
		if !present || actual != term {
			return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
		}
		return keyspace.FamilyFloat, keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}, true
	case keyspace.FamilyString:
		actual, _, stringValue, present := literals.Strings().At(int(ordinal - 1))
		if !present || actual != term {
			return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
		}
		return keyspace.FamilyString, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: stringValue}, true
	default:
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
}

func (schema *Schema) sourceAtomForValue(value linkboundary.Value) (uint32, linkboundary.Value, bool, bool) {
	family, literal, literalOK := schema.sourceLiteral(value)
	switch family {
	case keyspace.FamilyNil:
		return schema.atomByRow[atomRow{kind: atomNil}], value, literalOK, literalOK
	case keyspace.FamilyBool:
		if !literalOK {
			return 0, linkboundary.Value{}, false, false
		}
		if literal.Bool {
			return schema.literalSourceAtom(value, runtimekind.Boolean, atomRow{kind: atomTrue})
		}
		return schema.literalSourceAtom(value, runtimekind.Boolean, atomRow{kind: atomFalse})
	case keyspace.FamilyInteger:
		return schema.literalSourceAtom(value, runtimekind.Number, atomRow{kind: atomPrimitive, runtime: runtimekind.Number})
	case keyspace.FamilyFloat:
		if !literalOK {
			return 0, linkboundary.Value{}, false, false
		}
		if atom, _, present, exact := schema.literalSourceAtom(value, runtimekind.Number, atomRow{}); exact && present {
			return atom, value, true, true
		}
		atom, atomOK := schema.sourceFloatAtom(math.Float64frombits(literal.FloatBits))
		if !atomOK {
			return 0, linkboundary.Value{}, false, false
		}
		return atom, value, true, true
	case keyspace.FamilyString:
		return schema.literalSourceAtom(value, runtimekind.String, atomRow{kind: atomPrimitive, runtime: runtimekind.String})
	case keyspace.FamilyInvalid:
		if reference := schema.typeRefs[value]; reference != 0 {
			atom, found := schema.referenceAtom(reference, materialization.Exact)
			return atom, value, found, found
		}
		return 0, linkboundary.Value{}, false, false
	default:
		return 0, linkboundary.Value{}, false, true
	}
}

// literalSourceAtom attaches only an existing Link-normalized exact key to an
// authored literal.  If Link has no key (notably NaN), the caller's existing
// Value-owned fallback remains authoritative; Value never invents one.
func (schema *Schema) literalSourceAtom(value linkboundary.Value, runtime runtimekind.Kind, fallback atomRow) (uint32, linkboundary.Value, bool, bool) {
	key, keyOK := schema.sourceExactKey(value)
	if keyOK {
		literal, literalOK := schema.source.Project().Keys().Exact(key)
		if !literalOK {
			return 0, linkboundary.Value{}, false, false
		}
		atom := schema.addAtom(atomRow{
			kind:         atomLiteral,
			runtime:      runtime,
			key:          key,
			hasKey:       true,
			literalFalsy: literal.Kind == keyspace.LiteralBool && !literal.Bool,
		})
		return atom, value, atom != 0, atom != 0
	}
	if fallback.kind == atomInvalid {
		return 0, linkboundary.Value{}, false, false
	}
	atom := schema.atomByRow[fallback]
	return atom, value, atom != 0, atom != 0
}

func (schema *Schema) sourceExactKey(value linkboundary.Value) (linkproject.Key, bool) {
	if schema == nil || schema.source == nil || schema.exactKeys == nil {
		return linkproject.Key{}, false
	}
	family, literal, ok := schema.sourceLiteral(value)
	if !ok || family == keyspace.FamilyNil {
		return linkproject.Key{}, false
	}
	switch family {
	case keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString:
	default:
		return linkproject.Key{}, false
	}
	literal, ok = programsource.NormalizeExactKey(literal)
	if !ok {
		return linkproject.Key{}, false
	}
	key, ok := schema.exactKeys[literal]
	return key, ok
}

// sourceFloatAtom deliberately classifies only the Value-owned observation.
// Numeric/equality retains the exact IEEE payload. NaN and negative zero use
// the already-sealed opaque Number alternative because the primitive source
// atom would falsely claim that Value retained their distinct representation.
func (schema *Schema) sourceFloatAtom(number float64) (uint32, bool) {
	if schema == nil {
		return 0, false
	}
	if !sourceFloatRepresentable(number) {
		atom := schema.atomByRow[atomRow{kind: atomNaN, runtime: runtimekind.Number}]
		return atom, atom != 0
	}
	// Negative zero retains no Value-owned IEEE payload, but it is still a
	// definitely valid Lua table key (normalized to integer zero by Link).
	// Only NaN needs the separate key-invalid atom.
	atom := schema.atomByRow[atomRow{kind: atomPrimitive, runtime: runtimekind.Number}]
	return atom, atom != 0
}

func (schema *Schema) finish() bool {
	if schema == nil || schema.source == nil || len(schema.atoms) == 0 {
		return false
	}
	atomCapacity := uint64(len(schema.atoms))
	if atomCapacity > uint64(^uint32(0)) {
		return false
	}
	capabilityMultiplier := uint64(len(schema.capabilities)) + 1
	if capabilityMultiplier == 0 || atomCapacity > ^uint64(0)/capabilityMultiplier {
		return false
	}
	potential := atomCapacity * capabilityMultiplier
	if potential == 0 {
		return false
	}
	schema.potential = potential
	schema.bottom = Value{schema: schema}
	schema.top = Value{schema: schema, top: true}
	return schema.finishReductions()
}

func (schema *Schema) finishReductions() bool {
	if schema == nil || schema.potential == 0 || len(schema.atoms) == 0 || schema.firstStoredUnknown == 0 || schema.firstStoredExact == 0 || !schema.storedProjectionOrderValid() {
		return false
	}
	// runtimekind has a deliberately closed small vocabulary. The indexed
	// table is an immutable cold artifact, not an on-demand cache and not a
	// new Factor. Intersection is the correct may-kind reduction: an opaque
	// reference that may be a table remains possible when table is requested.
	limit := 1 << uint(runtimekind.Count-1)
	schema.forRuntimeKinds = make([]Value, limit)
	for mask := 0; mask < limit; mask++ {
		image := schema.fullRows(func(id uint32) bool {
			return schema.atomKinds(id)&runtimekind.Set(mask) != 0
		})
		schema.forRuntimeKinds[mask] = schema.canonical(image)
	}
	schema.atomTop = make([]Value, len(schema.atoms)+1)
	for id := range schema.atoms {
		schema.atomTop[id+1] = schema.canonical(schema.fullRows(func(candidate uint32) bool { return candidate == uint32(id+1) }))
	}
	schema.storedNoneTop = schema.canonical(schema.fullRows(func(id uint32) bool {
		return id < schema.firstStoredUnknown
	}))
	schema.storedUnknownTop = schema.canonical(schema.fullRows(func(id uint32) bool {
		return id >= schema.firstStoredUnknown && id < schema.firstStoredExact
	}))
	return schema.owns(schema.storedNoneTop) && schema.owns(schema.storedUnknownTop)
}

func (schema *Schema) fullRows(include func(uint32) bool) []uint64 {
	if schema == nil || include == nil {
		return nil
	}
	image := make([]uint64, 0, len(schema.atoms)*schema.stride())
	for index := range schema.atoms {
		id := uint32(index + 1)
		if !include(id) {
			continue
		}
		image = append(image, uint64(id))
		for word := 0; word < schema.capWords; word++ {
			image = append(image, schema.fullCapabilityWord(word))
		}
	}
	return image
}

func (schema *Schema) addAtom(row atomRow) uint32 {
	if schema == nil || row.kind == atomInvalid || uint64(len(schema.atoms)) >= uint64(^uint32(0)) {
		return 0
	}
	if row.kind == atomReference {
		if row.reference == 0 || int(row.reference) > len(schema.references) || !row.role.Valid() || row.hasKey || row.literalFalsy {
			return 0
		}
		if row.runtime != runtimekind.Invalid && !row.runtime.Valid() {
			return 0
		}
	} else if row.kind == atomPrimitive || row.kind == atomNaN || row.kind == atomOpaqueKind || row.kind == atomLiteral {
		if !row.runtime.Valid() {
			return 0
		}
		if row.kind == atomLiteral {
			if !row.hasKey || schema.source == nil {
				return 0
			}
			literal, ok := schema.source.Project().Keys().Exact(row.key)
			if !ok || schema.exactKeys[literal] != row.key ||
				row.literalFalsy != (literal.Kind == keyspace.LiteralBool && !literal.Bool) {
				return 0
			}
		} else if row.hasKey || row.literalFalsy {
			return 0
		}
	} else if row.kind == atomOpaqueReference {
		if (row.runtime != runtimekind.Invalid && !row.runtime.Valid()) || row.hasKey || row.literalFalsy {
			return 0
		}
	} else if row.runtime != runtimekind.Invalid || row.reference != 0 || row.role != materialization.Invalid || row.hasKey || row.literalFalsy {
		return 0
	}
	if existing := schema.atomByRow[row]; existing != 0 {
		return existing
	}
	id := uint32(len(schema.atoms) + 1)
	schema.atoms = append(schema.atoms, row)
	schema.atomByRow[row] = id
	return id
}

func (schema *Schema) atomKinds(id uint32) runtimekind.Set {
	if schema == nil || id == 0 || int(id) > len(schema.atoms) {
		return 0
	}
	row := schema.atoms[id-1]
	switch row.kind {
	case atomNil:
		return runtimekind.Bit(runtimekind.Nil)
	case atomFalse, atomTrue:
		return runtimekind.Bit(runtimekind.Boolean)
	case atomPrimitive, atomNaN, atomOpaqueKind, atomLiteral:
		return runtimekind.Bit(row.runtime)
	case atomReference:
		if row.runtime.Valid() {
			return runtimekind.Bit(row.runtime)
		}
		return referenceKinds(schema.references[row.reference-1].kind)
	case atomOpaqueReference:
		if row.runtime.Valid() {
			return runtimekind.Bit(row.runtime)
		}
		return referenceKinds(ReferenceOpaque)
	default:
		return 0
	}
}

func referenceKinds(kind ReferenceKind) runtimekind.Set {
	switch kind {
	case ReferenceTable:
		return runtimekind.Bit(runtimekind.Table)
	case ReferenceFunction:
		return runtimekind.Bit(runtimekind.Function)
	case ReferenceThread:
		return runtimekind.Bit(runtimekind.Thread)
	case ReferenceUserdata:
		return runtimekind.Bit(runtimekind.Userdata)
	case ReferenceOpaque:
		return runtimekind.Bit(runtimekind.Table) | runtimekind.Bit(runtimekind.Function) | runtimekind.Bit(runtimekind.Thread) | runtimekind.Bit(runtimekind.Userdata)
	default:
		return 0
	}
}

func runtimeForReference(kind ReferenceKind) runtimekind.Kind {
	switch kind {
	case ReferenceTable:
		return runtimekind.Table
	case ReferenceFunction:
		return runtimekind.Function
	case ReferenceThread:
		return runtimekind.Thread
	case ReferenceUserdata:
		return runtimekind.Userdata
	case ReferenceOpaque:
		return runtimekind.Invalid
	default:
		return runtimekind.Invalid
	}
}

func (schema *Schema) atomTruth(id uint32) Truth {
	if schema == nil || id == 0 || int(id) > len(schema.atoms) {
		return TruthNone
	}
	switch schema.atoms[id-1].kind {
	case atomNil, atomFalse:
		return TruthFalse
	case atomTrue, atomPrimitive, atomNaN, atomReference, atomOpaqueReference:
		return TruthTrue
	case atomLiteral:
		if schema.atoms[id-1].literalFalsy {
			return TruthFalse
		}
		return TruthTrue
	case atomOpaqueKind:
		if schema.atoms[id-1].runtime == runtimekind.Nil {
			return TruthFalse
		}
		if schema.atoms[id-1].runtime == runtimekind.Boolean {
			return TruthFalse | TruthTrue
		}
		return TruthTrue
	default:
		return TruthNone
	}
}

// AtomCount reports the sealed finite alternative vocabulary.
func (schema *Schema) AtomCount() int {
	if schema == nil {
		return 0
	}
	return len(schema.atoms)
}

// CoordinateCount is the complete pre-existing Link Value range sealed into
// this Schema. It is declaration vocabulary only; Value facts never retain a
// coordinate key.
func (schema *Schema) CoordinateCount() int {
	if schema == nil || schema.source == nil {
		return 0
	}
	return schema.source.Boundary().Values().Count()
}

// CoordinateAt returns the exact opaque coordinate at one dense local
// declaration index. Raw Link Values remain structural Schema payloads and
// are never admitted as Factor coordinates.
func (schema *Schema) CoordinateAt(index int) (Coordinate, bool) {
	if schema == nil || schema.source == nil || index < 0 || index >= schema.CoordinateCount() || uint64(index) >= uint64(^uint32(0)) {
		return Coordinate{}, false
	}
	return Coordinate{schema: schema, index: uint32(index + 1)}, true
}

// CoordinateFor returns the existing Value coordinate selected by one exact
// Link-issued Value. It neither exposes Link's private ordinal nor constructs
// a second coordinate vocabulary. Values from an independently sealed Link,
// including one with identical content, fail the owner fence.
func (schema *Schema) CoordinateFor(value linkboundary.Value) (Coordinate, bool) {
	if schema == nil || schema.source == nil {
		return Coordinate{}, false
	}
	row, ok := schema.coordinates[value]
	if !ok || row.coordinate == 0 || uint64(row.coordinate) > uint64(schema.CoordinateCount()) {
		return Coordinate{}, false
	}
	return Coordinate{schema: schema, index: row.coordinate}, true
}

// CoordinateIndex maps only a Coordinate issued by this exact Schema to its
// private dense Factor position. Same-content foreign Links deliberately do
// not share an owner coordinate.
func (schema *Schema) CoordinateIndex(coordinate Coordinate) (uint32, bool) {
	if schema == nil || schema.source == nil || coordinate.schema != schema || coordinate.index == 0 || uint64(coordinate.index) > uint64(schema.CoordinateCount()) {
		return 0, false
	}
	return coordinate.index - 1, true
}

// Valid reports whether this coordinate remains issued by a live Schema.
func (coordinate Coordinate) Valid() bool {
	if coordinate.schema == nil {
		return false
	}
	_, ok := coordinate.schema.CoordinateIndex(coordinate)
	return ok
}

// AdmitsCoordinate is the complete Value coordinate admission law. It
// accepts only a coordinate issued by this exact Schema and an owned fact.
func (schema *Schema) AdmitsCoordinate(coordinate Coordinate, fact Value) bool {
	_, ok := schema.CoordinateIndex(coordinate)
	return ok && schema.owns(fact)
}

// Source returns the exact source atom for literal/runtime-type Link Values.
// Allocation coordinates deliberately have no source atom: their sole
// executable authority is the allocation Rule's atomic Age+Fresh patch.
// Dynamic reads, calls, cells and outcomes likewise have no source atom.
func (schema *Schema) Source(value linkboundary.Value) (Atom, bool) {
	if schema == nil || schema.source == nil {
		return Atom{}, false
	}
	row, ok := schema.coordinates[value]
	if !ok || row.atom == 0 {
		return Atom{}, false
	}
	return Atom{schema: schema, id: row.atom}, true
}

// Allocation returns the presealed rooted reference atom for one exact
// Heap-owned allocation key and materialization role. This query never
// changes Schema state.
func (schema *Schema) Allocation(key heap.Key, role materialization.Role) (Atom, bool) {
	if schema == nil || !schema.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation || !role.Valid() {
		return Atom{}, false
	}
	reference := schema.allocRefs[key]
	id, ok := schema.referenceAtom(reference, role)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// Boot returns the exact actor-local boot root alternative.  Boot roots are
// existing Link identities and therefore never become fabricated heap roots.
func (schema *Schema) Boot(root linkhost.BootRoot) (Atom, bool) {
	if schema == nil {
		return Atom{}, false
	}
	reference := schema.bootRefs[root]
	id, ok := schema.referenceAtom(reference, materialization.Exact)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// Endpoint returns one exact host Function endpoint alternative.
func (schema *Schema) Endpoint(endpoint linkboundary.Endpoint) (Atom, bool) {
	if schema == nil {
		return Atom{}, false
	}
	reference := schema.endpointRefs[endpoint]
	id, ok := schema.referenceAtom(reference, materialization.Exact)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// Callable returns an exact bootstrap callable alternative. Link owns whether
// it is admitted or denied and Call owns its outcomes; this method supplies
// only the identity-preserving Value fact used by bootstrap/Heap projection.
func (schema *Schema) Callable(seed linkboundary.Seed) (Atom, bool) {
	if schema == nil {
		return Atom{}, false
	}
	reference := schema.callableRefs[seed]
	id, ok := schema.referenceAtom(reference, materialization.Exact)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// ScopedLoader returns the nominal Function atom for Target's exact scoped
// require operation. The loader's shard-specific Boundary seed is deliberately
// resolved later by Call dispatch from the bound Application.
func (schema *Schema) ScopedLoader(operation target.Operation) (Atom, bool) {
	if schema == nil || operation == 0 || schema.scopedLoader == 0 || int(schema.scopedLoader) > len(schema.references) {
		return Atom{}, false
	}
	row := schema.references[schema.scopedLoader-1]
	if row.source != referenceSourceScopedLoader || row.operation != operation {
		return Atom{}, false
	}
	id, ok := schema.referenceAtom(schema.scopedLoader, materialization.Exact)
	if !ok {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// OpaqueKind returns the closed fallback alternative for one runtime kind.
// It carries no identity and cannot be confused with a rooted opaque ref.
func (schema *Schema) OpaqueKind(kind runtimekind.Kind) (Atom, bool) {
	if schema == nil || !kind.Valid() {
		return Atom{}, false
	}
	id := schema.atomByRow[atomRow{kind: atomOpaqueKind, runtime: kind}]
	if id == 0 {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

// OpaqueReference returns the identity-free fallback alternative for a known
// reference family.  It is distinct from a rooted opaque exact reference.
func (schema *Schema) OpaqueReference(kind ReferenceKind) (Atom, bool) {
	if schema == nil || !kind.valid() {
		return Atom{}, false
	}
	runtime := runtimeForReference(kind)
	id := schema.atomByRow[atomRow{kind: atomOpaqueReference, runtime: runtime}]
	if id == 0 {
		return Atom{}, false
	}
	return Atom{schema: schema, id: id}, true
}

func (schema *Schema) referenceAtom(reference uint32, role materialization.Role) (uint32, bool) {
	if schema == nil || reference == 0 || int(reference) > len(schema.references) || !role.Valid() {
		return 0, false
	}
	id := schema.atomByRow[atomRow{
		kind:      atomReference,
		runtime:   runtimeForReference(schema.references[reference-1].kind),
		reference: reference,
		role:      role,
	}]
	return id, id != 0
}

// Link returns this schema's exact immutable Link authority.  It is provided
// only for typed structural child declarations, never as an engine fact.
func (schema *Schema) Link() *link.Link {
	if schema == nil {
		return nil
	}
	return schema.source
}
