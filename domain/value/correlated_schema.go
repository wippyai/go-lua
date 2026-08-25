package value

import (
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkmodule "github.com/wippyai/go-lua/analysis/program/link/module"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
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
	// atomComputedLiteral is an exact owner-produced scalar: either a
	// Program-owned arithmetic summary or a schema-declared runtime result.
	// It retains the concrete scalar payload but deliberately has no Link key;
	// Heap key identity remains available only through atomLiteral.
	atomComputedLiteral
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
	case atomComputedLiteral:
		if row.key.Kind == keyspace.LiteralFloat && math.IsNaN(math.Float64frombits(row.key.FloatBits)) {
			return TableKeyInvalid
		}
		return TableKeyValid
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

// ExactKey projects the normalized literal identity only for an authored
// storable literal alternative.  Nil and NaN deliberately have no key atom;
// callers must retain their separate Lua semantics rather than fabricating a
// table-key identity.  The dense Key is Link-owned, not a Value key space.
func (atom Atom) ExactKey() (keyspace.LiteralValue, bool) {
	if !atom.valid() {
		return keyspace.LiteralValue{}, false
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

// BootRootID returns the exact actor-local boot-root receipt identity.
func (reference Reference) BootRootID() (identity.ContentID, bool) {
	if !reference.valid() {
		return identity.ContentID{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.boot, row.source == referenceSourceBoot
}

// EndpointID returns the exact host Function endpoint receipt identity.
func (reference Reference) EndpointID() (identity.ContentID, bool) {
	if !reference.valid() {
		return identity.ContentID{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.endpoint, row.source == referenceSourceEndpoint
}

// Callable returns an exact admitted or explicitly denied bootstrap callable
// Seed. Its disposition remains Link/Call-owned; Value retains only runtime
// identity so aliases and table reads do not collapse a denied callable to an
// opaque function.
func (reference Reference) CallableID() (identity.ContentID, bool) {
	if !reference.valid() {
		return identity.ContentID{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.callable, row.source == referenceSourceCallable
}

// ScopedLoader returns the exact Target operation classified by Boundary as
// the scoped require ingress. Unlike Callable, this reference carries no
// global seed: Call resolves its shard-local loader from the bound
// Application at dispatch time.
func (reference Reference) ScopedLoader() (vocabulary.Operation, bool) {
	if !reference.valid() {
		return 0, false
	}
	row := reference.schema.references[reference.id-1]
	return row.operation, row.source == referenceSourceScopedLoader
}

// TypeValueSource returns the existing Link Value used for one executable
// Program TypeValue source. Its descriptor remains TypeValue-owned.
func (reference Reference) TypeValueSourceID() (identity.ContentID, bool) {
	if !reference.valid() {
		return identity.ContentID{}, false
	}
	row := reference.schema.references[reference.id-1]
	return row.value, row.source == referenceSourceRuntimeType
}

type referenceRow struct {
	source           referenceSource
	kind             ReferenceKind
	allocation       heap.Key
	allocationResult *AllocationResult
	boot             identity.ContentID
	endpoint         identity.ContentID
	callable         identity.ContentID
	operation        vocabulary.Operation
	value            identity.ContentID
}

type atomRow struct {
	kind      atomKind
	runtime   runtimekind.Kind
	reference uint32
	role      materialization.Role
	key       keyspace.LiteralValue
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

// returnBoundaryMember is one already-issued Value coordinate in a sealed
// ReturnBoundary's dense fixed-member arena. Keeping this row in Schema makes
// member access owner-fenced and avoids retaining Program slices or a second
// topology authority in the published operand.
//
// content is the owner-issued identity of the member at this arena position,
// qualified by the mount and the return occurrence that carries it. A Program
// Values member row is reachable from more than one mounted return, so the
// arena position is named by the boundary that seals it rather than by the
// Program row alone.
type returnBoundaryMember struct {
	coordinate Coordinate
	content    identity.ContentID
}

type mountedCoordinateKey struct {
	module identity.ContentID
	value  identity.ContentID
}

type literalSourceRow struct {
	family  keyspace.Family
	literal keyspace.LiteralValue
}

type targetInitialKey struct {
	root    identity.ContentID
	initial vocabulary.InitialValue
}

// Schema is the sealed Value alternative universe for one Link.  It is
// neither a registry nor a second identity/key space: source Values remain
// Link Values and every structural reference delegates back to Link's
// existing opaque handle. Its complete finite alternative universe is sealed
// before any Factor declaration; Schema queries never mutate it.
type Schema struct {
	owner link.OwnerCapability
	// linkID is the sealed owner identity used by published receipts. The
	// mounted Link pointer above is consumed only by the cold Seal pipeline;
	// hot receipt validation must use this scalar fence instead.
	linkID identity.ContentID
	heap   heap.Schema

	atoms           []atomRow
	atomByRow       map[atomRow]uint32
	coordinateCount uint32
	mountCount      uint32
	// coordinates is the complete detached Value identity range: Link's
	// boundary Values followed by the mounted finite CallResult-slot suffix.
	// Suffix rows are declaration-only dynamic coordinates and carry no source
	// atom; they are named by Program's derived portable slot identity.
	coordinates map[identity.ContentID]coordinateRow
	// coordinateOrder is the canonical publication order of the coordinate
	// range: ascending by portable identity, with the dense factor position of
	// each row beside it. A detached result is content addressed, so the order
	// its rows are published in must be a function of the coordinates it holds
	// and never of the private declaration position they were sealed at. It is
	// built once with the range and read by every publication, so a publisher
	// neither sorts nor allocates.
	coordinateOrder []canonicalCoordinate
	// coordinateIdentities is the dense-indexed inverse of the coordinate
	// range: the portable identity this schema issued for the coordinate held
	// at each dense factor position. It is derived once with the canonical
	// order, so naming a coordinate is a read and never a scan, and the name
	// is the one the seal assigned rather than a second numbering.
	coordinateIdentities []identity.ContentID
	// mountedCoordinates is the detached semantic lookup consumed by downstream
	// domains after the Boundary source graph has been released.
	mountedCoordinates map[mountedCoordinateKey]uint32
	// sourceSeedMounts is the sealed, Link-qualified mounted source directory.
	// SourceSeedForValueID resolves canonical source rows directly from
	// coordinates; no parallel source-ID directory is retained.
	sourceSeedMounts     []sourceSeedMount
	sourceSeedMountIndex map[identity.ContentID]uint32
	exactKeys            map[keyspace.LiteralValue]keyspace.LiteralValue
	literalSources       map[identity.ContentID]literalSourceRow

	// storageTransfers is Value's complete fixed Read/Bind/Write relation.
	// It is derived once from canonical Program terms and existing Link Values;
	// Link deliberately retains no computation-storage projection for it.
	storageTransfers        []storageTransferRow
	storageTransferOrdinals map[StorageTransferRef]uint32
	// storageTransferOccurrences is the Link-qualified bridge from a reusable
	// Program artifact occurrence to this Link-owned transfer operand.
	storageTransferOccurrences map[storageTransferOccurrenceKey]uint32

	// computation rows are Value-owned interpretations issued while the Link
	// is still open during sealing.  Published operands resolve through these
	// dense, owner-fenced maps; they never reopen a mounted Program.
	// endpoints is the sealed operand -> dense Value coordinate projection
	// issued once every operand family is sealed.
	endpoints              []endpointRow
	endpointTable          identity.ContentID
	binaryEqualities       map[computationKey]BinaryEquality
	binaryArithmetics      map[computationKey]BinaryArithmetic
	binaryOrders           map[computationKey]BinaryOrder
	runtimeKindCalls       map[computationKey]RuntimeKindCall
	moduleLoadCalls        map[computationKey]ModuleLoadCall
	presenceRefinements    map[computationKey]PresenceRefinement
	unaryNots              map[computationKey]UnaryNot
	selectBranches         map[selectBranchKey]SelectBranch
	valueClaims            map[computationKey]ValueClaim
	returnBoundaries       map[computationKey]ReturnBoundary
	returnBoundariesByBody map[computationKey][]computationKey
	returnBoundaryMembers  []returnBoundaryMember
	// returnBoundaryOrder is the dense return-boundary candidate directory:
	// mount order, then Program occurrence order. returnBoundaryMemberIndex is
	// the inverse of the member arena, from a member's owner-issued mounted
	// identity to its dense arena position.
	returnBoundaryOrder       []computationKey
	returnBoundaryMemberIndex map[computationKey]uint32
	// freshResultCalls is the detached Value-owned admission directory for
	// Target fresh results that have an existing fixed CallResultValue
	// coordinate. Heap remains the sole issuer of the key; this map only joins
	// that key to Call's operation and the existing Boundary Value coordinate.
	// allocationResultKeys is the dense order of Value's issued allocation
	// receipts, and allocationResultOrdinals its exact inverse. Both are the
	// reference order sealAllocationResults issued them in: the directory is a
	// projection of that one order, never a second one.
	allocationResultKeys     []heap.Key
	allocationResultOrdinals map[heap.Key]uint32

	freshResultCalls        map[heap.Key]FreshResultCall
	freshResultCallKeys     []heap.Key
	freshResultCallOrdinals map[heap.Key]uint32
	// moduleExportFresh is the narrow composition proof for a fresh require
	// result. Its roots are existing Heap allocation keys recovered from an
	// authored Module Import's exported root fact; it is not an operation×root
	// dispatch table and is consumed only by Heap index topology sealing.
	moduleExportFresh map[heap.Key]moduleExportFreshRow
	// mountedCallResultSlots is the immutable, Value-owned projection of
	// admitted Program CallResultSlot geometry. It carries only the mounted
	// semantic IDs and the already-issued Value coordinate; the canonical
	// Program CallResult/CallResultSlot rows remain in the cold builder
	// directory.
	mountedCallResultSlots map[mountedCallResultSlotKey]MountedCallResultSlot
	// mountedCallResultSlotDirectory is the dense candidate order of the
	// result-zero rows above. A rule that folds one mounted call's first
	// result is addressed through it, so the family has one owner-issued
	// numbering of those rows rather than a directory of its own.
	mountedCallResultSlotDirectory []MountedCallResultSlot
	// mountedCallArguments is the immutable, Value-owned projection of every
	// admitted Program Call actual: the receiver first for a method-form call,
	// then each declared argument in order, matching Pack's fixed endpoint
	// list. mountedCallArgumentOrder is the dense candidate-directory order;
	// mountedCallArgumentOccurrences is the mount-qualified inverse from a
	// row's own owner-issued content identity to its dense ordinal.
	mountedCallArguments           map[mountedCallArgumentKey]MountedCallArgument
	mountedCallArgumentOrder       []mountedCallArgumentKey
	mountedCallArgumentOccurrences map[mountedCallArgumentOccurrenceKey]uint32
	// mountedCallActuals is the per-call parent of those rows: the ordered
	// member set one mounted call carries, grouped by the (module, call)
	// prefix of the actual rows' own key. It mints no call identity and reads
	// no Pack geometry - it is this Schema's own seal order projected once so
	// a member set can be addressed by (parent, ordinal).
	mountedCallActuals         map[mountedCallActualsKey]MountedCallActuals
	mountedCallActualsOrder    []mountedCallActualsKey
	mountedCallActualsOrdinals map[mountedCallActualsKey]uint32

	references     []referenceRow
	allocRefs      map[heap.Key]uint32
	globalResults  map[identity.ContentID]*GlobalBootstrapResult
	globalIDs      []identity.ContentID
	globalOrdinals map[identity.ContentID]uint32
	targetInitials map[targetInitialKey]Value
	bootRefs       map[identity.ContentID]uint32
	endpointRefs   map[identity.ContentID]uint32
	callableRefs   map[identity.ContentID]uint32
	scopedLoader   uint32
	typeRefs       map[identity.ContentID]uint32

	capabilities []identity.ContentID
	capabilityID map[identity.ContentID]uint32
	capWords     int

	capabilitySeeds []capabilitySeedRow
	hostMembers     []hostMember

	potential uint64
	bottom    Value
	top       Value

	// These cold precomputed reductions are immutable views over dense atom
	// rows. They keep common projections allocation-free and do not create a
	// second state plane. Stored-value reduction has three disjoint local
	// classes: non-reference, untracked reference, and tracked reference. The
	// two range boundaries are private representation order, never identity.
	forRuntimeKinds []Value
	// forRuntimeNames is the immutable schema projection of the presealed
	// string atom issued for each CategoryRuntimeKind member. It is keyed by
	// the same closed may-set as forRuntimeKinds, so a hot rule projects names
	// without constructing strings, atoms, or image slices.
	forRuntimeNames      []Value
	runtimeKindNameAtoms [runtimekind.Count]uint32
	atomTop              []Value
	// atomTopImage is one schema-owned immutable arena for singleton
	// reductions. Keeping one row per atom here avoids allocating a separate
	// backing array for every atom while the sealed schema still owns every
	// returned Value image. Each published row is full-sliced to its exact
	// stride, so callers cannot retain or append beyond that row.
	atomTopImage       []uint64
	storedNoneTop      Value
	storedUnknownTop   Value
	firstStoredUnknown uint32
	firstStoredExact   uint32
}

// valueBuilder is stack-local construction authority. It is never reachable
// from a published Schema: Seal returns only builder.Schema.
type valueBuilder struct {
	*Schema
	project     *linkproject.Component
	boundary    *linkboundary.Component
	host        *linkhost.Component
	module      *linkmodule.Component
	moduleFacts map[moduleLoadFactKey]Value
	// mountedCallResultSlots is the one cold canonical CallResultSlot
	// directory for every finite output coordinate carried by an admitted
	// mounted Artifact/Snapshot. It is keyed by concrete module placement,
	// reusable Program Call identity, and result ordinal.
	mountedCallResultSlots map[mountedCallResultSlotKey]programschema.CallResultSlot
	// mountedCallResultSlotOrder is the canonical parent/child publication
	// order used by the post-mount Value projection. It includes structural
	// slots whose ValueID is absent; those rows are deliberately omitted from
	// Value's admitted directory after semantic coordinate resolution.
	mountedCallResultSlotOrder []mountedCallResultSlotKey
	// artifacts is the exact reusable ProgramArtifact substitution directory.
	// It is cold builder state and cannot survive into the published Schema.
	artifacts map[identity.ContentID]programmount.MountedArtifact
	// formalSources is the construction-only set of Program-issued formal
	// storage coordinates that receive Value Top at callable entry.
	formalSources map[identity.ContentID]struct{}
	// structural is the sealed schema vocabulary supplied by composition. It
	// is construction-only: the published Value Schema retains only the
	// presealed atom rows and projections, never a second catalog reference.
	structural structure.Table
	// calls is Call's sealed algebra for the same Link. Value reads exactly
	// one published projection from it - the mounted-call coordinate of a
	// call-result candidate row - and copies that coordinate into the row, so
	// no hot rule reopens Call to rediscover the occurrence it already owns.
	calls *calldomain.Algebra
}

// callCoordinateForOccurrence copies Call's published coordinate for one
// mounted occurrence. Absence is a refusal: a call-result candidate whose
// occurrence Call does not name has no coordinate to publish, and a zero
// coordinate would be a default standing in for a fact nobody derived.
func (builder *valueBuilder) callCoordinateForOccurrence(module, occurrence identity.ContentID) (calldomain.CallCoordinate, bool) {
	if builder == nil || builder.calls == nil {
		return calldomain.CallCoordinate{}, false
	}
	coordinate, ok := builder.calls.CallCoordinateForOccurrence(module, occurrence)
	return coordinate, ok && builder.calls.OwnsCallCoordinate(coordinate)
}

func (builder *valueBuilder) sealProject() *linkproject.Component   { return builder.project }
func (builder *valueBuilder) sealBoundary() *linkboundary.Component { return builder.boundary }
func (builder *valueBuilder) sealHost() *linkhost.Component         { return builder.host }
func (builder *valueBuilder) sealModule() *linkmodule.Component     { return builder.module }

// sealMountedCallResultGeometry indexes the canonical CallResultSlot family
// carried by each admitted mounted Artifact/Snapshot exactly once while
// validating its parent spans in publication order.
// Program.CallResultForID and Program.CallResultSlotForID are intentionally not
// used by consumers: their cold inverses are linear scans, while these
// directories keep fresh-result and mounted-slot reads O(1) after one O(n)
// seal walk. Duplicate module/call(/ordinal) keys or any mismatch between the
// mounted Program row's placement and immutable publication fail closed.
func (builder *valueBuilder) sealMountedCallResultGeometry() bool {
	if builder == nil || builder.Schema == nil || builder.sealProject() == nil || builder.artifacts == nil || builder.mountedCallResultSlots != nil {
		return false
	}
	slots := make(map[mountedCallResultSlotKey]programschema.CallResultSlot)
	slotOrder := make([]mountedCallResultSlotKey, 0)
	mounts := builder.sealProject().Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		module, moduleOK := builder.sealProject().ModuleKey(shard)
		mount := builder.artifacts[module]
		if !shardOK || !moduleOK || !module.Available() || !mount.Available() || mount.ModuleKey != module {
			return false
		}
		mounted := mount.Program
		if !mounted.Available() || mounted.ModuleKey != module {
			return false
		}
		program := mounted.Program
		count, published := program.CallResultCount()
		slotCount, slotsPublished := program.CallResultSlotCount()
		if !published || !slotsPublished || count < 0 || slotCount < 0 {
			return false
		}
		seenCalls := make(map[identity.ContentID]struct{}, count)
		slotCursor := uint32(0)
		for index := 0; index < count; index++ {
			result, resultOK := program.CallResultAt(index)
			callID := result.CallID()
			offset, width, spanOK := result.SlotSpan()
			open, openOK := result.ResultsOpen()
			if !resultOK || !result.Available() || !callID.Available() || !spanOK || !openOK {
				return false
			}
			// Only an exact result owns a dense child span. An open result
			// publishes the canonical empty span and takes no position in the
			// slot plane, so it neither advances nor is measured by the cursor.
			if open {
				if offset != 0 || width != 0 {
					return false
				}
			} else if offset != slotCursor || uint64(offset)+uint64(width) > uint64(slotCount) {
				return false
			}
			if _, duplicate := seenCalls[callID]; duplicate {
				return false
			}
			seenCalls[callID] = struct{}{}
			for childIndex := uint32(0); childIndex < width; childIndex++ {
				slot, slotOK := program.CallResultSlotAt(int(offset + childIndex))
				ordinal, ordinalOK := slot.Ordinal()
				if !slotOK || !slot.Available() || !ordinalOK || ordinal != childIndex || slot.CallID() != callID {
					return false
				}
				slotKey := mountedCallResultSlotKey{module: module, call: callID, ordinal: ordinal}
				if _, duplicate := slots[slotKey]; duplicate {
					return false
				}
				slots[slotKey] = slot
				slotOrder = append(slotOrder, slotKey)
			}
			slotCursor += width
		}
		if slotCursor > uint32(slotCount) {
			return false
		}
		if slotCursor != uint32(slotCount) {
			return false
		}
	}
	if len(slotOrder) != len(slots) {
		return false
	}
	builder.mountedCallResultSlots = slots
	builder.mountedCallResultSlotOrder = slotOrder
	return true
}

// LinkID is the detached owner identity for this sealed Value schema. Hot
// callers must compare this scalar fence; the Link object is construction-only.
func (schema *Schema) LinkID() identity.ContentID {
	if schema == nil {
		return identity.ContentID{}
	}
	return schema.linkID
}

// Valid reports a fully sealed, published Value universe. It is the
// cross-domain ownership fence for consumers that previously tested for a
// Project or Boundary pointer.
func (schema *Schema) Valid() bool {
	return schema != nil && schema.linkID.Available() && schema.owner.Available() &&
		uint64(len(schema.coordinates)) == uint64(schema.coordinateCount) && schema.heap.Valid()
}

func (schema *Schema) MountCount() int {
	if schema == nil {
		return 0
	}
	return int(schema.mountCount)
}

// LinkOwner returns the exact detached owner witness issued by the Link that
// admitted this schema. LinkID alone is replayable and cannot authenticate
// independently sealed equal-content Links.
func (schema *Schema) LinkOwner() link.OwnerCapability {
	if schema == nil {
		return link.OwnerCapability{}
	}
	return schema.owner
}

// OwnsHeapSchema reports whether candidate is the exact immutable Heap
// authority retained when this Value schema was sealed.  Heap Schema values
// are owner handles, not content identities: two independent seals of the
// same Link intentionally compare unequal here and must never share Value
// atoms with Heap keys or index topology.
func (schema *Schema) OwnsHeapSchema(candidate heap.Schema) bool {
	return schema != nil && schema.heap.Valid() && candidate.Valid() && schema.heap == candidate
}

// Heap returns the exact immutable Heap authority retained when this Value
// schema was sealed. Value does not issue a second key space; callers that
// need to redeem a Heap occurrence receipt use this owner-fenced projection.
func (schema *Schema) Heap() heap.Schema {
	if schema == nil || !schema.heap.Valid() {
		return heap.Schema{}
	}
	return schema.heap
}

// Coordinate is an exact Schema-issued Value Factor coordinate. Its dense
// position is private declaration layout; the Schema pointer fences the
// coordinate to one exact Link even when another Link has the same content
// and the same private Value index.
type Coordinate struct {
	schema *Schema
	index  uint32 // one based
}

type capabilitySeedRow struct {
	id         identity.ContentID
	capability identity.ContentID
	source     CapabilitySource
	exposure   identity.ContentID
}

// CapabilitySource is Value's detached classification of a sealed Host
// capability receipt. It intentionally carries no Host owner handle.
type CapabilitySource uint8

const (
	CapabilitySourceInvalid CapabilitySource = iota
	CapabilitySourceInitialRoot
	CapabilitySourceABIInput
	CapabilitySourceResult
	CapabilitySourceExposure
)

type hostMember struct {
	capability identity.ContentID
	output     identity.ContentID
	endpoint   identity.ContentID
}

// SealFailure is the first closed Value-schema construction boundary that
// rejected an input. It is detached scalar evidence: no partial Schema or
// Link-owned row escapes on failure.
type SealFailure uint8

const (
	SealFailureNone SealFailure = iota
	SealFailureInput
	SealFailureCoordinates
	SealFailureComputation
	SealFailureStorageTransferInput
	SealFailureStorageTransferMount
	SealFailureStorageTransferReadDenominator
	SealFailureStorageTransferReadProof
	SealFailureStorageTransferReadOccurrence
	SealFailureStorageTransferAddInput
	SealFailureStorageTransferAddFromValue
	SealFailureStorageTransferAddToValue
	SealFailureStorageTransferAddFromCoordinate
	SealFailureStorageTransferAddToCoordinate
	SealFailureStorageTransferAddShard
	SealFailureStorageTransferAddMount
	SealFailureStorageTransferAddOccurrence
	SealFailureStorageTransferAddRef
	SealFailureStorageTransferAddIdentity
	SealFailureStorageTransferAddExecutable
	SealFailureStorageTransferAddDuplicateRef
	SealFailureStorageTransferAddDuplicateOccurrence
	SealFailureStorageTransferAddCapacity
	SealFailureStorageTransferBind
	SealFailureStorageTransferWrite
	SealFailureExactKeys
	SealFailureCapabilities
	SealFailureSources
	SealFailureBootstrapCallables
	SealFailureOpaqueAlternatives
	SealFailureLiteralSourceAtoms
	SealFailureTargetLiteralAtoms
	SealFailureStoredUnknownAtoms
	SealFailureStoredExactAtoms
	SealFailureReferenceSourceAtoms
	SealFailureFinish
	SealFailureAllocationResults
	SealFailureSourceValues
	SealFailureSourceOccurrences
	SealFailureGlobalBootstrapResults
	// SealFailureRuntimeKindAtoms is appended to preserve every prior failure
	// ordinal. The runtime-kind names are a schema-fed Value vocabulary, not a
	// hot fallback, so their admission has its own closed failure boundary.
	SealFailureRuntimeKindAtoms
	// SealFailureFreshResultCalls is appended so existing failure ordinals stay
	// stable while the Target fresh-result CallResult join gets its own closed
	// construction boundary.
	SealFailureFreshResultCalls
	// SealFailureMountedCallArguments is appended so existing failure ordinals
	// stay stable while the mounted Call actual projection gets its own closed
	// construction boundary.
	SealFailureMountedCallArguments
)

func (failure SealFailure) String() string {
	names := [...]string{
		"none", "input", "coordinates", "computation", "storage-transfer-input", "storage-transfer-mount", "storage-transfer-read-denominator", "storage-transfer-read-proof", "storage-transfer-read-occurrence",
		"storage-transfer-add-input", "storage-transfer-add-from-value", "storage-transfer-add-to-value", "storage-transfer-add-from-coordinate", "storage-transfer-add-to-coordinate", "storage-transfer-add-shard", "storage-transfer-add-mount", "storage-transfer-add-occurrence", "storage-transfer-add-ref", "storage-transfer-add-identity", "storage-transfer-add-executable", "storage-transfer-add-duplicate-ref", "storage-transfer-add-duplicate-occurrence", "storage-transfer-add-capacity",
		"storage-transfer-bind", "storage-transfer-write", "exact-keys", "capabilities", "sources", "bootstrap-callables",
		"opaque-alternatives", "literal-source-atoms", "target-literal-atoms", "stored-unknown-atoms", "stored-exact-atoms",
		"reference-source-atoms", "finish", "allocation-results", "source-values", "source-occurrences", "global-bootstrap-results", "runtime-kind-atoms",
		"fresh-result-calls", "mounted-call-arguments",
	}
	if int(failure) < 0 || int(failure) >= len(names) {
		return "invalid"
	}
	return names[failure]
}

// Seal derives the complete finite Value alternative vocabulary from the
// already-sealed Link.  It does not inspect AST/binder state, materialize a
// candidate product, or create a second raw Program identity.
func SealWithFailure(source *link.Link, heaps heap.Schema, calls *calldomain.Algebra, mounts []programmount.MountedArtifact, structural structure.Table) (*Schema, SealFailure) {
	if source == nil || !source.ContentID().Available() || !heaps.LinkOwner().Matches(source.OwnerCapability()) || heaps.LinkContentID() != source.ContentID() || len(mounts) != source.Project().Mounts().Count() || structural.Count(structure.CategoryRuntimeKind) != int(runtimekind.Count)-1 {
		return nil, SealFailureInput
	}
	// Call is this seal's declared peer authority. Its coordinate projection is
	// the earliest owner of the mounted-call coordinate a call-result candidate
	// row publishes, so the algebra is required for the same Link rather than
	// reconstructed here.
	if calls == nil || !calls.Valid() || !calls.LinkOwner().Matches(source.OwnerCapability()) {
		return nil, SealFailureInput
	}
	artifacts := make(map[identity.ContentID]programmount.MountedArtifact, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			return nil, SealFailureInput
		}
		if _, duplicate := artifacts[mount.ModuleKey]; duplicate {
			return nil, SealFailureInput
		}
		artifacts[mount.ModuleKey] = mount
	}
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, shardOK := source.Project().Mounts().At(index)
		module, moduleOK := source.Project().ModuleKey(shard)
		programID, programOK := source.Project().Mounts().ProgramID(shard)
		mount := artifacts[module]
		if !shardOK || !moduleOK || !programOK || !mount.Available() || mount.ProgramID != programID {
			return nil, SealFailureInput
		}
	}
	schema := &Schema{
		owner:                          source.OwnerCapability(),
		linkID:                         source.ContentID(),
		heap:                           heaps,
		atomByRow:                      make(map[atomRow]uint32),
		coordinateCount:                uint32(source.Boundary().Values().Count()),
		mountCount:                     uint32(source.Project().Mounts().Count()),
		coordinates:                    make(map[identity.ContentID]coordinateRow, source.Boundary().Values().Count()),
		mountedCoordinates:             make(map[mountedCoordinateKey]uint32),
		exactKeys:                      make(map[keyspace.LiteralValue]keyspace.LiteralValue, source.Project().Keys().Count()),
		literalSources:                 make(map[identity.ContentID]literalSourceRow),
		storageTransferOrdinals:        make(map[StorageTransferRef]uint32),
		storageTransferOccurrences:     make(map[storageTransferOccurrenceKey]uint32),
		binaryEqualities:               make(map[computationKey]BinaryEquality),
		binaryArithmetics:              make(map[computationKey]BinaryArithmetic),
		binaryOrders:                   make(map[computationKey]BinaryOrder),
		runtimeKindCalls:               make(map[computationKey]RuntimeKindCall),
		moduleLoadCalls:                make(map[computationKey]ModuleLoadCall),
		presenceRefinements:            make(map[computationKey]PresenceRefinement),
		unaryNots:                      make(map[computationKey]UnaryNot),
		selectBranches:                 make(map[selectBranchKey]SelectBranch),
		valueClaims:                    make(map[computationKey]ValueClaim),
		returnBoundaries:               make(map[computationKey]ReturnBoundary),
		returnBoundariesByBody:         make(map[computationKey][]computationKey),
		returnBoundaryMemberIndex:      make(map[computationKey]uint32),
		allocationResultOrdinals:       make(map[heap.Key]uint32),
		freshResultCalls:               make(map[heap.Key]FreshResultCall),
		freshResultCallOrdinals:        make(map[heap.Key]uint32),
		moduleExportFresh:              make(map[heap.Key]moduleExportFreshRow),
		mountedCallResultSlots:         make(map[mountedCallResultSlotKey]MountedCallResultSlot),
		mountedCallArguments:           make(map[mountedCallArgumentKey]MountedCallArgument),
		mountedCallArgumentOccurrences: make(map[mountedCallArgumentOccurrenceKey]uint32),
		mountedCallActuals:             make(map[mountedCallActualsKey]MountedCallActuals),
		mountedCallActualsOrdinals:     make(map[mountedCallActualsKey]uint32),
		allocRefs:                      make(map[heap.Key]uint32),
		globalResults:                  make(map[identity.ContentID]*GlobalBootstrapResult),
		globalOrdinals:                 make(map[identity.ContentID]uint32),
		targetInitials:                 make(map[targetInitialKey]Value),
		bootRefs:                       make(map[identity.ContentID]uint32),
		endpointRefs:                   make(map[identity.ContentID]uint32),
		callableRefs:                   make(map[identity.ContentID]uint32),
		typeRefs:                       make(map[identity.ContentID]uint32),
		capabilityID:                   make(map[identity.ContentID]uint32),
	}
	builder := &valueBuilder{Schema: schema, project: source.Project(), boundary: source.Boundary(), host: source.Host(), module: source.Module(), moduleFacts: make(map[moduleLoadFactKey]Value), artifacts: artifacts, structural: structural, calls: calls}
	if !builder.sealMountedCallResultGeometry() {
		return nil, SealFailureComputation
	}
	if !builder.sealCoordinates() {
		return nil, SealFailureCoordinates
	}
	if !builder.sealMountedCoordinateDirectory() {
		return nil, SealFailureCoordinates
	}
	if !builder.sealMountedCallResultSlots() {
		return nil, SealFailureCoordinates
	}
	if !builder.sealMountedCallArguments() {
		return nil, SealFailureMountedCallArguments
	}
	// The mounted finite tail slots reserve the last coordinates of the range,
	// so the canonical publication order is derived once the range is complete
	// and never again.
	if !builder.Schema.installCanonicalCoordinateOrder() {
		return nil, SealFailureCoordinates
	}
	if !builder.sealFormalSourceDirectory() {
		return nil, SealFailureCoordinates
	}
	if !builder.sealComputationRows() {
		return nil, SealFailureComputation
	}
	if !builder.sealLiteralSourceDirectory() {
		return nil, SealFailureLiteralSourceAtoms
	}
	if failure := builder.sealStorageTransfersWithFailure(); failure != SealFailureNone {
		return nil, failure
	}
	steps := []struct {
		failure SealFailure
		seal    func() bool
	}{
		{SealFailureExactKeys, builder.sealExactKeys},
		{SealFailureCapabilities, builder.sealCapabilities},
		{SealFailureSources, builder.sealSources},
		{SealFailureBootstrapCallables, builder.sealBootstrapCallables},
		{SealFailureOpaqueAlternatives, builder.sealOpaqueAlternatives},
		{SealFailureLiteralSourceAtoms, builder.sealLiteralSourceAtoms},
		{SealFailureLiteralSourceAtoms, builder.sealComputedArithmeticAtoms},
		{SealFailureRuntimeKindAtoms, builder.sealRuntimeKindAtoms},
		{SealFailureTargetLiteralAtoms, builder.sealTargetLiteralAtoms},
		{SealFailureStoredUnknownAtoms, builder.sealStoredUnknownAtoms},
		{SealFailureStoredExactAtoms, builder.sealStoredExactAtoms},
		{SealFailureReferenceSourceAtoms, builder.sealReferenceSourceAtoms},
		{SealFailureFinish, builder.finish},
		// Module-root operands contain canonical Value images. Build them only
		// after reference atoms and the lattice potential are sealed; the cold
		// literal and mounted-call directories remain builder-owned here.
		{SealFailureComputation, builder.sealModuleLoadRows},
		{SealFailureFinish, builder.sealTargetInitialResults},
		{SealFailureAllocationResults, builder.sealAllocationResults},
		{SealFailureFreshResultCalls, builder.sealFreshResultCalls},
		{SealFailureFreshResultCalls, builder.sealModuleExportFreshRows},
		{SealFailureSourceValues, builder.sealSourceValues},
		{SealFailureSourceOccurrences, builder.sealSourceSeedOccurrences},
		{SealFailureGlobalBootstrapResults, func() bool { return builder.sealGlobalBootstrapResults(source.Module()) }},
		// Every operand family is sealed by here, so the endpoint projection
		// resolves each operand's coordinates once and for all.
		{SealFailureComputation, builder.sealEndpointVectors},
	}
	for _, step := range steps {
		if !step.seal() {
			return nil, step.failure
		}
	}
	return schema, SealFailureNone
}

// sealFormalSourceDirectory consumes Program's explicit formal-entry rows.
// It does not rediscover formals from syntax or FunctionBoundary geometry.
func (schema *valueBuilder) sealFormalSourceDirectory() bool {
	if schema == nil || schema.sealBoundary() == nil || schema.artifacts == nil || schema.formalSources != nil {
		return false
	}
	schema.formalSources = make(map[identity.ContentID]struct{})
	for module, mount := range schema.artifacts {
		program := mount.Program.Program
		count, countOK := program.OccurrenceKindCount(programschema.OccurrenceFormalEntry)
		if !countOK {
			return false
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.OccurrenceKindAt(programschema.OccurrenceFormalEntry, index)
			_, inputCount, inputSpanOK := row.InputSpan()
			if !rowOK || !inputSpanOK || inputCount != 1 {
				return false
			}
			value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
			valueID, valueIDOK := schema.sealBoundary().Values().ID(value)
			if !valueOK || !valueIDOK || !valueID.Available() {
				return false
			}
			if _, duplicate := schema.formalSources[valueID]; duplicate {
				return false
			}
			schema.formalSources[valueID] = struct{}{}
		}
	}
	return true
}

func (schema *valueBuilder) sealMountedCoordinateDirectory() bool {
	if schema == nil || schema.sealProject() == nil || schema.sealBoundary() == nil {
		return false
	}
	values := schema.sealBoundary().Values()
	quotients := make(map[identity.ContentID]storageCaptureQuotient)
	complete := true
	return values.VisitMountedSemantics(func(module, id identity.ContentID, value linkboundary.Value) bool {
		quotient, quotientOK := quotients[module]
		if !quotientOK {
			quotient, quotientOK = schema.storageCaptureQuotientForModule(module)
			if !quotientOK {
				complete = false
				return false
			}
			quotients[module] = quotient
		}
		canonicalID, captured := quotient.canonical(id)
		if !captured {
			canonicalID = id
		}
		canonicalValue, canonicalValueOK := values.ForMountedSemantic(module, canonicalID)
		coordinate, coordinateOK := schema.coordinateForCold(canonicalValue)
		key := mountedCoordinateKey{module: module, value: id}
		if !canonicalValueOK || !coordinateOK {
			complete = false
			return false
		}
		if previous, duplicate := schema.mountedCoordinates[key]; duplicate && previous != coordinate.index {
			complete = false
			return false
		}
		schema.mountedCoordinates[key] = coordinate.index
		return true
	}) && complete
}

// CoordinateForMountedSemantic resolves an artifact semantic Value ID through
// the cold-built detached directory. It never reopens Boundary or Project.
func (schema *Schema) CoordinateForMountedSemantic(module, value identity.ContentID) (Coordinate, bool) {
	if schema == nil || !module.Available() || !value.Available() {
		return Coordinate{}, false
	}
	index, ok := schema.mountedCoordinates[mountedCoordinateKey{module: module, value: value}]
	coordinate := Coordinate{schema: schema, index: index}
	return coordinate, ok && coordinate.Valid()
}

// sealAllocationResults issues one immutable result receipt beside the
// already-sealed allocation reference row. Only Program allocation roots have
// a Value coordinate; Target fresh roots remain owned by their guarded rule.
func (schema *valueBuilder) sealAllocationResults() bool {
	if schema == nil || schema.sealProject() == nil {
		return false
	}
	for index := range schema.references {
		row := &schema.references[index]
		if row.source != referenceSourceAllocation {
			continue
		}
		if row.allocationResult != nil {
			return false
		}
		module, _, allocationID, _, _, programRoot := schema.heap.AllocationOriginForKey(row.allocation)
		subjectID, subjectOK := schema.heap.AllocationRootValueID(row.allocation)
		if !programRoot || !subjectOK {
			continue
		}
		coordinateRow, coordinateOK := schema.coordinates[subjectID]
		recent, recentOK := schema.referenceAtom(uint32(index+1), materialization.Recent)
		fresh, freshOK := schema.Singleton(Atom{schema: schema.Schema, id: recent})
		keyID, keyOK := row.allocation.ContentID()
		if !subjectOK || !coordinateOK || coordinateRow.coordinate == 0 || !recentOK || !freshOK || !keyOK || !keyID.Available() ||
			!schema.AdmitsCoordinate(Coordinate{schema: schema.Schema, index: coordinateRow.coordinate}, fresh) {
			return false
		}
		_, summaryOK := schema.referenceAtom(uint32(index+1), materialization.Summary)
		if !summaryOK {
			return false
		}
		summary, _ := schema.referenceAtom(uint32(index+1), materialization.Summary)
		routes := []Coordinate{{schema: schema.Schema, index: coordinateRow.coordinate}}
		mount, mounted := schema.artifacts[module]
		if mounted {
			state, stateOK := mount.Program.Program.ColdState()
			targets, targetsOK := calltarget.NewView(state)
			count, countOK := targets.Count()
			if !stateOK || !targetsOK || !countOK {
				return false
			}
			var function identity.ContentID
			for targetIndex := 0; targetIndex < count; targetIndex++ {
				target, targetOK := targets.At(targetIndex)
				if !targetOK {
					return false
				}
				if target.AllocationID() != allocationID {
					continue
				}
				if function.Available() {
					return false
				}
				function = target.FunctionID()
			}
			if function.Available() {
				alias, aliasOK := schema.CoordinateForMountedSemantic(module, function)
				// A Function row is code identity, not necessarily a Value
				// coordinate (named declarations commonly have no expression
				// result). Only an existing Value coordinate participates in the
				// allocation's atomic result image.
				if aliasOK && alias != routes[0] {
					routes = append(routes, alias)
				}
			}
		}
		row.allocationResult = &AllocationResult{
			schema: schema.Schema, key: row.allocation, keyID: keyID,
			coordinate: Coordinate{schema: schema.Schema, index: coordinateRow.coordinate}, routes: routes, fresh: fresh,
			recent: recent, summary: summary,
		}
		if _, duplicate := schema.allocationResultOrdinals[row.allocation]; duplicate {
			return false
		}
		schema.allocationResultOrdinals[row.allocation] = uint32(len(schema.allocationResultKeys))
		schema.allocationResultKeys = append(schema.allocationResultKeys, row.allocation)
	}
	return len(schema.allocationResultKeys) == len(schema.allocationResultOrdinals)
}

// sealExactKeys imports Link's already-normalized key universe once during
// Schema construction.  It is intentionally a cold reverse projection: no
// Value fact stores literal payloads, strings, or a duplicate key identity.
func (schema *valueBuilder) sealExactKeys() bool {
	if schema == nil || schema.sealProject() == nil || schema.exactKeys == nil {
		return false
	}
	for index := 0; index < schema.sealProject().Keys().Count(); index++ {
		key, ok := schema.sealProject().Keys().At(index)
		if !ok {
			return false
		}
		literal, ok := schema.sealProject().Keys().Exact(key)
		if !ok {
			return false
		}
		if _, duplicate := schema.exactKeys[literal]; duplicate {
			return false
		}
		schema.exactKeys[literal] = literal
	}
	return len(schema.exactKeys) == schema.sealProject().Keys().Count()
}

// sealCoordinates enumerates Link's already-canonical Value range once during
// cold Schema construction. It assigns no new identity: the stored one-based
// coordinate is exactly CoordinateAt's existing declaration position.
func (schema *valueBuilder) sealCoordinates() bool {
	if schema == nil || schema.sealProject() == nil || len(schema.coordinates) != 0 || !validCoordinateCount(schema.sealBoundary().Values().Count()) {
		return false
	}
	for index := 0; index < schema.sealBoundary().Values().Count(); index++ {
		value, ok := schema.sealBoundary().Values().At(index)
		id, idOK := schema.sealBoundary().Values().ID(value)
		if !ok || !idOK || !id.Available() {
			return false
		}
		if _, duplicate := schema.coordinates[id]; duplicate {
			return false
		}
		schema.coordinates[id] = coordinateRow{coordinate: uint32(index + 1)}
	}
	return len(schema.coordinates) == schema.sealBoundary().Values().Count()
}

// installCanonicalCoordinateOrder derives the canonical publication order from
// the sealed coordinate range: ascending by portable identity, with the dense
// factor position of each row beside it. It is derived once with the range and
// read by every publication, so a publisher neither sorts nor allocates, and
// the order a detached result is written in is a function of its own content
// rather than of the position a coordinate happened to be sealed at.
func (schema *Schema) installCanonicalCoordinateOrder() bool {
	if schema == nil || schema.coordinateOrder != nil {
		return false
	}
	// A Link with no boundary Value range is a sealed schema with an empty
	// coordinate order, not a hole: the order is derived from the range, and a
	// range of none derives an order of none.
	count := len(schema.coordinates)
	order := make([]canonicalCoordinate, 0, count)
	identities := make([]identity.ContentID, count)
	occupied := make([]bool, count)
	for id, row := range schema.coordinates {
		if !id.Available() || row.coordinate == 0 || uint64(row.coordinate) > uint64(count) || occupied[row.coordinate-1] {
			return false
		}
		occupied[row.coordinate-1] = true
		identities[row.coordinate-1] = id
		order = append(order, canonicalCoordinate{id: id, dense: row.coordinate - 1})
	}
	identity.SortByContentID(order, func(row canonicalCoordinate) identity.ContentID { return row.id })
	for index := 1; index < len(order); index++ {
		if order[index-1].id == order[index].id {
			return false
		}
	}
	schema.coordinateOrder = order
	schema.coordinateIdentities = identities
	return true
}

// canonicalCoordinate is one row of the canonical publication order: the
// portable identity a consumer names the coordinate by, and the dense factor
// position the fact for it is held at.
type canonicalCoordinate struct {
	id    identity.ContentID
	dense uint32
}

func validCoordinateCount(count int) bool {
	return count >= 0 && uint64(count) <= uint64(^uint32(0))
}

func (schema *valueBuilder) sealCapabilities() bool {
	for index := 0; index < schema.sealHost().Capabilities().Count(); index++ {
		capability, ok := schema.sealHost().Capabilities().At(index)
		id, idOK := schema.sealHost().Capabilities().ID(capability)
		if !ok || !idOK || !id.Available() || schema.capabilityID[id] != 0 {
			return false
		}
		schema.capabilities = append(schema.capabilities, id)
		schema.capabilityID[id] = uint32(len(schema.capabilities))
	}
	schema.capWords = (len(schema.capabilities) + 63) / 64
	return true
}

// sealSources admits direct Heap allocation coordinates first, then the
// remaining Link-owned literal/bootstrap source families.  Heap alone
// enumerates allocation and fresh-root structure; Link no longer carries an
// allocation source row that Value could replay or reinterpret.
func (schema *valueBuilder) sealSources() bool {
	contract, ok := schema.sealBoundary().Target()
	if !ok || contract == nil {
		return false
	}
	if !schema.sealAllocationReferences() {
		return false
	}
	for index := 0; index < schema.sealHost().BootRoots().Count(); index++ {
		root, ok := schema.sealHost().BootRoots().At(index)
		if !ok || !schema.addBootReference(contract, root) {
			return false
		}
	}
	endpoints := schema.sealBoundary().Endpoints()
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
	if !schema.forEachExecutableTypeValue(func(value identity.ContentID) bool {
		return schema.addTypeValueReferenceID(value)
	}) {
		return false
	}
	if !schema.sealScopedLoader() {
		return false
	}
	for index := 0; index < schema.sealHost().CapabilitySeeds().Count(); index++ {
		seed, ok := schema.sealHost().CapabilitySeeds().At(index)
		seedID, seedIDOK := schema.sealHost().CapabilitySeeds().ID(seed)
		if !ok || !seedIDOK || !seedID.Available() {
			return false
		}
		capability, ok := schema.sealHost().CapabilitySeeds().Capability(seed)
		capabilityID, capabilityIDOK := schema.sealHost().Capabilities().ID(capability)
		source, sourceOK := schema.sealHost().CapabilitySeeds().Source(seed)
		if !ok || !capabilityIDOK || !sourceOK || schema.capabilityID[capabilityID] == 0 {
			return false
		}
		row := capabilitySeedRow{id: seedID, capability: capabilityID, source: CapabilitySource(source)}
		if source == linkhost.ProviderCapabilitySourceExposure {
			exposure, exposureOK := schema.sealHost().CapabilitySeeds().Exposure(seed)
			exposureID, exposureIDOK := schema.sealBoundary().Values().ID(exposure)
			if !exposureOK || !exposureIDOK || !exposureID.Available() {
				return false
			}
			row.exposure = exposureID
		}
		schema.capabilitySeeds = append(schema.capabilitySeeds, row)
	}
	for index := 0; index < schema.sealHost().Members().Count(); index++ {
		_, _, capability, _, output, endpoint, _, ok := schema.sealHost().Members().At(index)
		capabilityID, capabilityIDOK := schema.sealHost().Capabilities().ID(capability)
		outputID, outputIDOK := schema.sealBoundary().Values().ID(output)
		endpointID, endpointIDOK := schema.sealBoundary().Endpoints().ID(endpoint)
		if !ok || !capabilityIDOK || !outputIDOK || !endpointIDOK || schema.capabilityID[capabilityID] == 0 || schema.endpointRefs[endpointID] == 0 {
			return false
		}
		schema.hostMembers = append(schema.hostMembers, hostMember{capability: capabilityID, output: outputID, endpoint: endpointID})
	}
	return true
}

// sealScopedLoader admits the one nominal Function reference for Target's
// scoped require initial value. Boundary intentionally emits no global seed
// for this operation; Call later chooses the loader seed from the bound
// Application's mounted shard.
func (schema *valueBuilder) sealScopedLoader() bool {
	if schema == nil || schema.sealProject() == nil {
		return false
	}
	require, hasRequire := schema.sealBoundary().RequireOperation()
	if !hasRequire {
		return true
	}
	contract, contractOK := schema.sealBoundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	return schema.visitTargetInitialValues(func(initial vocabulary.InitialValue) bool {
		kind, kindOK := contract.InitialValueKind(initial)
		if !kindOK || kind != vocabulary.InitialValueOperation {
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
func (schema *valueBuilder) sealAllocationReferences() bool {
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

func (schema *valueBuilder) addAllocationReference(key heap.Key) bool {
	if schema == nil || !schema.heap.OwnsKey(key) || key.Kind() != heap.RootAllocation {
		return false
	}
	if schema.allocRefs[key] != 0 {
		return true
	}
	refKind := ReferenceInvalid
	_, _, _, kind, _, sourceRoot := schema.heap.AllocationOriginForKey(key)
	if sourceRoot {
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
		if _, _, _, fresh := key.FreshResultID(); !fresh {
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

func (schema *valueBuilder) addBootReference(contract *contract.Contract, root linkhost.BootRoot) bool {
	id, idOK := schema.sealHost().BootRoots().ID(root)
	if !idOK || !id.Available() {
		return false
	}
	if schema.bootRefs[id] != 0 {
		return true
	}
	_, initial, ok := schema.sealHost().BootRoots().Mapping(root)
	if !ok {
		return false
	}
	shape, ok := contract.InitialRootBootShape(initial)
	if !ok {
		return false
	}
	aggregate, ok := contract.BootShapeAggregate(shape)
	if !ok || (aggregate != vocabulary.BootAggregateTable && aggregate != vocabulary.BootAggregateMetatable) {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceBoot, kind: ReferenceTable, boot: id}) != 0
}

func (schema *valueBuilder) addEndpointReference(endpoint linkboundary.Endpoint) bool {
	id, idOK := schema.sealBoundary().Endpoints().ID(endpoint)
	if !idOK || !id.Available() {
		return false
	}
	if schema.endpointRefs[id] != 0 {
		return true
	}
	if _, ok := schema.sealBoundary().Endpoints().Operation(endpoint); !ok {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceEndpoint, kind: ReferenceFunction, endpoint: id}) != 0
}

func (schema *valueBuilder) sealBootstrapCallables() bool {
	add := func(value vocabulary.InitialValue) bool {
		seed, _, callable := schema.sealBoundary().Seeds().BootstrapCallable(value)
		return !callable || schema.addCallableReference(seed)
	}
	return schema.visitTargetInitialValues(add)
}

// visitTargetInitialValues is the exact Target-reachable initial-value image.
// It is used only while sealing Value's finite owner-local atom universe;
// callers retain no Target value in recurrent State.
func (schema *valueBuilder) visitTargetInitialValues(visit func(vocabulary.InitialValue) bool) bool {
	if schema == nil || schema.sealProject() == nil || visit == nil {
		return false
	}
	contract, ok := schema.sealBoundary().Target()
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

func (schema *valueBuilder) addCallableReference(seed linkboundary.Seed) bool {
	id, idOK := schema.sealBoundary().Seeds().ID(seed)
	if !idOK || !id.Available() {
		return false
	}
	if schema.callableRefs[id] != 0 {
		return true
	}
	if _, _, _, ok := schema.sealBoundary().Seeds().CallableDisposition(seed); !ok {
		return false
	}
	return schema.addReference(referenceRow{source: referenceSourceCallable, kind: ReferenceFunction, callable: id}) != 0
}

func (schema *valueBuilder) addTypeValueReferenceID(id identity.ContentID) bool {
	if !id.Available() {
		return false
	}
	if schema.typeRefs[id] != 0 {
		return true
	}
	return schema.addReference(referenceRow{source: referenceSourceRuntimeType, kind: ReferenceOpaque, value: id}) != 0
}

func (schema *valueBuilder) addReference(row referenceRow) uint32 {
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
		require, ok := schema.sealBoundary().RequireOperation()
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

func (schema *valueBuilder) sealOpaqueAlternatives() bool {
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
	for index := 0; ; index++ {
		kind, ok := runtimekind.Scalar.MemberAt(index)
		if !ok {
			break
		}
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
func (schema *valueBuilder) sealLiteralSourceDirectory() bool {
	if schema == nil || schema.sealProject() == nil || schema.artifacts == nil || schema.literalSources == nil {
		return false
	}
	for module, mount := range schema.artifacts {
		program := mount.Program.Program
		count, countOK := program.OccurrenceCount()
		if !countOK {
			return false
		}
		for index := 0; index < count; index++ {
			row, ok := program.OccurrenceAt(index)
			if !ok || row.Kind() != programschema.OccurrenceValueSource {
				continue
			}
			family, literal, literalOK := row.Literal()
			if !literalOK {
				continue
			}
			value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
			if !valueOK {
				return false
			}
			candidate := literalSourceRow{family: family, literal: literal}
			valueID, valueIDOK := schema.sealBoundary().Values().ID(value)
			if !valueIDOK {
				return false
			}
			if prior, exists := schema.literalSources[valueID]; exists && prior != candidate {
				return false
			}
			schema.literalSources[valueID] = candidate
		}
	}
	return true
}

func (schema *valueBuilder) sealLiteralSourceAtoms() bool {
	for index := 0; index < schema.sealBoundary().Values().Count(); index++ {
		value, ok := schema.sealBoundary().Values().At(index)
		valueID, valueIDOK := schema.sealBoundary().Values().ID(value)
		if !ok || !valueIDOK {
			return false
		}
		family, _, ok := schema.sourceLiteralID(valueID)
		if !ok {
			continue
		}
		switch family {
		case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
			keyspace.FamilyFloat, keyspace.FamilyString:
			atom, subject, present, ok := schema.sourceAtomForValue(valueID)
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
func (schema *valueBuilder) sealTargetLiteralAtoms() bool {
	contract, ok := schema.sealBoundary().Target()
	if !ok || contract == nil {
		return false
	}
	return schema.visitTargetInitialValues(func(value vocabulary.InitialValue) bool {
		kind, ok := contract.InitialValueKind(value)
		if !ok {
			return false
		}
		runtime := runtimekind.Invalid
		switch kind {
		case vocabulary.InitialValueBoolean:
			runtime = runtimekind.Boolean
		case vocabulary.InitialValueInteger, vocabulary.InitialValueFloat:
			runtime = runtimekind.Number
		case vocabulary.InitialValueString:
			runtime = runtimekind.String
		default:
			return true
		}
		key, exact := schema.sealProject().Keys().ForInitial(contract, value)
		if !exact {
			return true
		}
		literal, literalOK := schema.sealProject().Keys().Exact(key)
		if !literalOK {
			return false
		}
		return schema.addAtom(atomRow{
			kind:         atomLiteral,
			runtime:      runtime,
			key:          literal,
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
func (schema *valueBuilder) sealStoredUnknownAtoms() bool {
	if schema == nil || schema.firstStoredUnknown != 0 || schema.firstStoredExact != 0 {
		return false
	}
	schema.firstStoredUnknown = uint32(len(schema.atoms) + 1)
	for index := 0; ; index++ {
		kind, ok := runtimekind.Reference.MemberAt(index)
		if !ok {
			break
		}
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
func (schema *valueBuilder) sealStoredExactAtoms() bool {
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

func (schema *valueBuilder) sealReferenceSourceAtoms() bool {
	return schema.forEachExecutableTypeValue(func(value identity.ContentID) bool {
		atom, subject, present, ok := schema.sourceAtomForValue(value)
		return ok && schema.assignSourceAtom(atom, subject, present)
	})
}

// forEachExecutableTypeValue reads only the canonical Program TypeValue rows
// and validates their retained Link static resolver/expression tuple before a
// Value-domain interpretation is admitted.
func (schema *valueBuilder) forEachExecutableTypeValue(visit func(identity.ContentID) bool) bool {
	if schema == nil || schema.sealProject() == nil || visit == nil || schema.artifacts == nil {
		return false
	}
	for module, mount := range schema.artifacts {
		program := mount.Program.Program
		count, countOK := program.OccurrenceCount()
		if !countOK {
			return false
		}
		for index := 0; index < count; index++ {
			row, rowOK := program.OccurrenceAt(index)
			if !rowOK || row.Kind() != programschema.OccurrenceValueSource || row.Code() != 6 {
				continue
			}
			value, valueOK := schema.sealBoundary().Values().ForMountedSemantic(module, row.ID())
			valueID, valueIDOK := schema.sealBoundary().Values().ID(value)
			if !valueOK || !valueIDOK || !visit(valueID) {
				return false
			}
		}
	}
	return true
}

func (schema *valueBuilder) assignSourceAtom(atom uint32, value identity.ContentID, present bool) bool {
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

func (schema *valueBuilder) addReferenceAtoms(reference uint32, exact bool) bool {
	if schema == nil || reference == 0 || int(reference) > len(schema.references) {
		return false
	}
	runtime := runtimeForReference(schema.references[reference-1].kind)
	for _, role := range materialization.Roles() {
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
		return runtimekind.Reference.Contains(row.runtime)
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
func (schema *valueBuilder) sealSourceValues() bool {
	if schema == nil || schema.sealProject() == nil || schema.potential == 0 || schema.formalSources == nil {
		return false
	}
	for subject, row := range schema.coordinates {
		if _, formal := schema.formalSources[subject]; formal {
			if row.atom != 0 || row.source.schema != nil {
				return false
			}
			fact := schema.Top()
			if !schema.owns(fact) || schema.Equal(fact, schema.Bottom()) {
				return false
			}
			row.source = fact
			schema.coordinates[subject] = row
			continue
		}
		if row.atom == 0 {
			continue
		}
		if row.source.schema != nil {
			return false
		}
		_, _, literal := schema.sourceLiteralID(subject)
		if schema.typeRefs[subject] == 0 && !literal {
			continue
		}
		fact, ok := schema.Singleton(Atom{schema: schema.Schema, id: row.atom})
		if !ok || !schema.owns(fact) || schema.Equal(fact, schema.Bottom()) {
			return false
		}
		id := subject
		if !id.Available() {
			return false
		}
		row.source = fact
		schema.coordinates[subject] = row
	}
	// These directories exist only to construct source atoms and immutable
	// source values. No published query reads them after this phase.
	schema.literalSources = nil
	schema.exactKeys = nil
	return true
}

// sealGlobalBootstrapResults issues immutable receipts for valid Host global
// bindings after all Value source facts have been sealed. Invalid or
// source-overlapping bindings simply have no bootstrap candidate; no hot path
// is permitted to reconstruct their Host/Module/Boundary/Target tuple.
func (schema *valueBuilder) sealGlobalBootstrapResults(module *linkmodule.Component) bool {
	if schema == nil || schema.sealProject() == nil || schema.globalResults == nil || schema.sealHost() == nil || module == nil || schema.sealBoundary() == nil {
		return false
	}
	contract, contractOK := schema.sealBoundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	globals := schema.sealHost().Globals()
	for index := 0; index < globals.Count(); index++ {
		binding, bindingOK := globals.At(index)
		if !bindingOK {
			return false
		}
		id, idOK := globals.ID(binding)
		analysis, boot, cell, _, class, initial, mappingOK := globals.Mapping(binding)
		if !idOK || !id.Available() || !mappingOK || class == vocabulary.InitialBindingInvalid || initial == 0 {
			continue
		}
		canonical, canonicalOK := globals.For(analysis, cell)
		shard, _, _, rootOK := module.Roots().Mapping(analysis)
		if !canonicalOK || canonical != binding || !rootOK {
			continue
		}
		subject, subjectOK := schema.sealBoundary().Values().Of(shard, cell)
		subjectID, subjectIDOK := schema.sealBoundary().Values().ID(subject)
		row, coordinateOK := schema.coordinates[subjectID]
		if !subjectOK || !subjectIDOK || !coordinateOK || row.coordinate == 0 {
			continue
		}
		if _, overlap := schema.SourceSeedForValueID(subjectID); overlap {
			continue
		}
		coordinate := Coordinate{schema: schema.Schema, index: row.coordinate}
		kind, kindOK := contract.InitialValueKind(initial)
		if !kindOK {
			continue
		}
		var fact Value
		absent := kind == vocabulary.InitialValueAbsent
		if !absent {
			var factOK bool
			fact, factOK = schema.targetInitialCold(boot, initial)
			if !factOK || !schema.AdmitsCoordinate(coordinate, fact) || schema.Equal(fact, schema.Default()) {
				continue
			}
		}
		result, resultOK := NewGlobalBootstrapResult(schema.Schema, id, coordinate, fact, absent)
		if !resultOK {
			continue
		}
		bindingID, bindingIDOK := globals.ID(binding)
		if !bindingIDOK {
			return false
		}
		schema.globalResults[bindingID] = result
		schema.globalOrdinals[bindingID] = uint32(len(schema.globalIDs))
		schema.globalIDs = append(schema.globalIDs, bindingID)
	}
	return true
}

// sourceLiteral resolves one existing Link Value through its canonical Source
// owner. Nil has no LiteralValue payload; its FamilyNil result is the direct
// Source discriminator. Non-literal Values are rejected so runtime TypeValue
// sources remain under their separate Link/static relation.
func (schema *Schema) sourceLiteralID(id identity.ContentID) (keyspace.Family, keyspace.LiteralValue, bool) {
	if schema == nil || schema.literalSources == nil || !id.Available() {
		return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
	}
	return schema.sourceLiteralRow(id)
}

func (schema *Schema) sourceLiteralRow(id identity.ContentID) (keyspace.Family, keyspace.LiteralValue, bool) {
	row, ok := schema.literalSources[id]
	if ok {
		return row.family, row.literal, true
	}
	return keyspace.FamilyInvalid, keyspace.LiteralValue{}, false
}

func (schema *valueBuilder) sourceAtomForValue(value identity.ContentID) (uint32, identity.ContentID, bool, bool) {
	family, literal, literalOK := schema.sourceLiteralID(value)
	switch family {
	case keyspace.FamilyNil:
		return schema.atomByRow[atomRow{kind: atomNil}], value, literalOK, literalOK
	case keyspace.FamilyBool:
		if !literalOK {
			return 0, identity.ContentID{}, false, false
		}
		if literal.Bool {
			return schema.literalSourceAtom(value, runtimekind.Boolean, atomRow{kind: atomTrue})
		}
		return schema.literalSourceAtom(value, runtimekind.Boolean, atomRow{kind: atomFalse})
	case keyspace.FamilyInteger:
		return schema.literalSourceAtom(value, runtimekind.Number, atomRow{kind: atomPrimitive, runtime: runtimekind.Number})
	case keyspace.FamilyFloat:
		if !literalOK {
			return 0, identity.ContentID{}, false, false
		}
		if atom, _, present, exact := schema.literalSourceAtom(value, runtimekind.Number, atomRow{}); exact && present {
			return atom, value, true, true
		}
		atom, atomOK := schema.sourceFloatAtom(math.Float64frombits(literal.FloatBits))
		if !atomOK {
			return 0, identity.ContentID{}, false, false
		}
		return atom, value, true, true
	case keyspace.FamilyString:
		return schema.literalSourceAtom(value, runtimekind.String, atomRow{kind: atomPrimitive, runtime: runtimekind.String})
	case keyspace.FamilyInvalid:
		if reference := schema.typeRefs[value]; reference != 0 {
			atom, found := schema.referenceAtom(reference, materialization.Exact)
			return atom, value, found, found
		}
		return 0, identity.ContentID{}, false, false
	default:
		return 0, identity.ContentID{}, false, true
	}
}

// literalSourceAtom attaches only an existing Link-normalized exact key to an
// authored literal.  If Link has no key (notably NaN), the caller's existing
// Value-owned fallback remains authoritative; Value never invents one.
func (schema *valueBuilder) literalSourceAtom(value identity.ContentID, runtime runtimekind.Kind, fallback atomRow) (uint32, identity.ContentID, bool, bool) {
	key, keyOK := schema.sourceExactKey(value)
	if keyOK {
		_, literal, literalOK := schema.sourceLiteralID(value)
		if !literalOK {
			return 0, identity.ContentID{}, false, false
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
		return 0, identity.ContentID{}, false, false
	}
	atom := schema.atomByRow[fallback]
	return atom, value, atom != 0, atom != 0
}

func (schema *valueBuilder) sourceExactKey(value identity.ContentID) (keyspace.LiteralValue, bool) {
	if schema == nil || schema.exactKeys == nil {
		return keyspace.LiteralValue{}, false
	}
	family, literal, ok := schema.sourceLiteralID(value)
	if !ok || family == keyspace.FamilyNil {
		return keyspace.LiteralValue{}, false
	}
	switch family {
	case keyspace.FamilyBool, keyspace.FamilyInteger, keyspace.FamilyFloat, keyspace.FamilyString:
	default:
		return keyspace.LiteralValue{}, false
	}
	literal, ok = scalar.Normalize(literal)
	if !ok {
		return keyspace.LiteralValue{}, false
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

func (schema *valueBuilder) finish() bool {
	if schema == nil || schema.sealProject() == nil || len(schema.atoms) == 0 {
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
	schema.bottom = Value{schema: schema.Schema}
	schema.top = Value{schema: schema.Schema, top: true}
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
	// Runtime-kind result names are a distinct presealed atom family. Their
	// atom kind is also String, so selecting by atomKinds alone would conflate
	// them with authored string literals; the schema-issued dense atom IDs keep
	// this projection exact without a second string vocabulary.
	schema.forRuntimeNames = make([]Value, limit)
	for mask := 0; mask < limit; mask++ {
		image := schema.fullRows(func(id uint32) bool {
			for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
				if runtimekind.Set(mask)&runtimekind.Bit(kind) != 0 && schema.runtimeKindNameAtoms[kind] == id {
					return true
				}
			}
			return false
		})
		schema.forRuntimeNames[mask] = schema.canonical(image)
	}
	// atomTop is a singleton projection for every atom. Build one compact,
	// schema-owned arena rather than allocating one backing array per atom.
	// The exact row bounds preserve the old owner/capacity fence, while the
	// arena makes this table one O(A*stride) allocation.
	stride := schema.stride()
	schema.atomTop = make([]Value, len(schema.atoms)+1)
	schema.atomTopImage = make([]uint64, len(schema.atoms)*stride)
	for id := range schema.atoms {
		start := id * stride
		row := schema.atomTopImage[start : start+stride : start+stride]
		row[0] = uint64(id + 1)
		for word := 0; word < schema.capWords; word++ {
			row[word+1] = schema.fullCapabilityWord(word)
		}
		schema.atomTop[id+1] = schema.canonical(row)
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
	// Count the selected atoms before allocating. The previous capacity used
	// the complete atom denominator for every projection (including each
	// singleton atomTop row), retaining O(A^2*stride) backing storage during
	// sealing. Exact capacity keeps the immutable reduction tables linear in
	// the number of selected rows.
	selected := 0
	for index := range schema.atoms {
		if include(uint32(index + 1)) {
			selected++
		}
	}
	image := make([]uint64, 0, selected*schema.stride())
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
	} else if row.kind == atomPrimitive || row.kind == atomNaN || row.kind == atomOpaqueKind || row.kind == atomLiteral || row.kind == atomComputedLiteral {
		if !row.runtime.Valid() {
			return 0
		}
		if row.kind == atomLiteral {
			if !row.hasKey {
				return 0
			}
			literal := row.key
			if schema.exactKeys[literal] != row.key ||
				row.literalFalsy != (literal.Kind == keyspace.LiteralBool && !literal.Bool) {
				return 0
			}
		} else if row.kind == atomComputedLiteral {
			if row.hasKey || row.literalFalsy ||
				(row.runtime == runtimekind.Number && row.key.Kind != keyspace.LiteralInteger && row.key.Kind != keyspace.LiteralFloat) ||
				(row.runtime == runtimekind.String && row.key.Kind != keyspace.LiteralString) ||
				(row.runtime != runtimekind.Number && row.runtime != runtimekind.String) {
				return 0
			}
		} else if row.hasKey || row.literalFalsy || row.key.Kind != 0 {
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
	case atomPrimitive, atomNaN, atomOpaqueKind, atomLiteral, atomComputedLiteral:
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
		return runtimekind.Reference
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
	case atomTrue, atomPrimitive, atomNaN, atomReference, atomOpaqueReference, atomComputedLiteral:
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
	if schema == nil {
		return 0
	}
	return int(schema.coordinateCount)
}

// CoordinateAt returns the exact opaque coordinate at one dense local
// declaration index. Raw Link Values remain structural Schema payloads and
// are never admitted as Factor coordinates.
func (schema *Schema) CoordinateAt(index int) (Coordinate, bool) {
	if schema == nil || index < 0 || index >= schema.CoordinateCount() || uint64(index) >= uint64(^uint32(0)) {
		return Coordinate{}, false
	}
	return Coordinate{schema: schema, index: uint32(index + 1)}, true
}

// CanonicalCoordinateAt returns one coordinate in the schema's canonical
// publication order: the portable identity it is named by and the dense factor
// position its fact is held at. The order is ascending by identity, so a
// detached result's row order is a function of its own content.
func (schema *Schema) CanonicalCoordinateAt(index int) (identity.ContentID, uint32, bool) {
	if schema == nil || index < 0 || index >= len(schema.coordinateOrder) {
		return identity.ContentID{}, 0, false
	}
	row := schema.coordinateOrder[index]
	return row.id, row.dense, true
}

// CoordinateForID returns the preissued local coordinate for one portable
// Boundary Value identity. It is the only post-seal coordinate lookup.
func (schema *Schema) CoordinateForID(value identity.ContentID) (Coordinate, bool) {
	if schema == nil || !value.Available() {
		return Coordinate{}, false
	}
	row, ok := schema.coordinates[value]
	if !ok || row.coordinate == 0 || uint64(row.coordinate) > uint64(schema.CoordinateCount()) {
		return Coordinate{}, false
	}
	return Coordinate{schema: schema, index: row.coordinate}, true
}

// coordinateForCold consumes a Link Value while the seal context is live and
// immediately converts it to the published detached identity.
func (schema *valueBuilder) coordinateForCold(value linkboundary.Value) (Coordinate, bool) {
	if schema == nil || schema.sealBoundary() == nil {
		return Coordinate{}, false
	}
	id, ok := schema.sealBoundary().Values().ID(value)
	if !ok {
		return Coordinate{}, false
	}
	return schema.CoordinateForID(id)
}

// CoordinateContentID answers the portable identity this Schema issued for one
// of its own coordinates. It is the exact inverse of CoordinateForID over the
// sealed range, and it reads the name the seal already assigned rather than
// deriving a second one, so a consumer that must address a coordinate row by
// content never mints an identity of its own.
func (schema *Schema) CoordinateContentID(coordinate Coordinate) (identity.ContentID, bool) {
	dense, ok := schema.CoordinateIndex(coordinate)
	if !ok || uint64(dense) >= uint64(len(schema.coordinateIdentities)) {
		return identity.ContentID{}, false
	}
	value := schema.coordinateIdentities[dense]
	if !value.Available() {
		return identity.ContentID{}, false
	}
	return value, true
}

// CoordinateIndex maps only a Coordinate issued by this exact Schema to its
// private dense Factor position. Same-content foreign Links deliberately do
// not share an owner coordinate.
func (schema *Schema) CoordinateIndex(coordinate Coordinate) (uint32, bool) {
	if schema == nil || coordinate.schema != schema || coordinate.index == 0 || uint64(coordinate.index) > uint64(schema.CoordinateCount()) {
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
func (schema *Schema) SourceID(value identity.ContentID) (Atom, bool) {
	if schema == nil || !value.Available() {
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
func (schema *Schema) BootID(root identity.ContentID) (Atom, bool) {
	if schema == nil || !root.Available() {
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
func (schema *Schema) EndpointID(endpoint identity.ContentID) (Atom, bool) {
	if schema == nil || !endpoint.Available() {
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
func (schema *Schema) CallableID(seed identity.ContentID) (Atom, bool) {
	if schema == nil || !seed.Available() {
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
func (schema *Schema) ScopedLoader(operation vocabulary.Operation) (Atom, bool) {
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
