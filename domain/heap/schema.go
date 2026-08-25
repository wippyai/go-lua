package heap

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkboundary "github.com/wippyai/go-lua/analysis/program/link/boundary"
	linkhost "github.com/wippyai/go-lua/analysis/program/link/host"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	fresh "github.com/wippyai/go-lua/domain/heap/internal/fresh"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

const schemaFormat uint64 = 0x686561702d7639 // "heap-v9"

// Schema is the one immutable Heap family authority derived from one sealed
// Link. It is deliberately not a per-root schema: all Values share this fence
// and the Factor's sparse default is therefore constant and lawful.
type Schema struct{ owner *schema }

// OccurrenceMount is Heap's opaque, mount-scoped occurrence issuer.  The
// issuer is obtained only from the sealed Heap owner, so callers cannot turn
// an arbitrary (module, occurrence) pair into a coordinate.  Its lookups use
// Heap's sealed occurrence inverses and retain the exact artifact pointer
// fence carried by the canonical mounted artifact row.
type OccurrenceMount struct {
	owner     *schema
	module    identity.ContentID
	programID identity.ContentID
	snapshot  *ingress.Snapshot
}

func (issuer OccurrenceMount) valid() bool {
	if issuer.owner == nil || !issuer.module.Available() || !issuer.programID.Available() || issuer.snapshot == nil || !issuer.snapshot.Available() || issuer.owner.artifacts == nil {
		return false
	}
	canonical, ok := issuer.owner.artifacts[issuer.module]
	return ok && canonical.Snapshot == issuer.snapshot && canonical.ProgramID == issuer.programID
}

// OccurrenceMountForModule returns Heap's canonical occurrence issuer for one
// concrete mounted module.  The returned issuer is the only hot lookup input;
// module and occurrence identities are never accepted as a free-standing
// directory key by consumers.
func (schema Schema) OccurrenceMountForModule(module identity.ContentID) (OccurrenceMount, bool) {
	if !schema.valid() || !module.Available() || schema.owner.artifacts == nil {
		return OccurrenceMount{}, false
	}
	mount, ok := schema.owner.artifacts[module]
	issuer := OccurrenceMount{owner: schema.owner, module: mount.ModuleKey, programID: mount.ProgramID, snapshot: mount.Snapshot}
	return issuer, ok && issuer.valid()
}

func (issuer OccurrenceMount) Module() identity.ContentID {
	if !issuer.valid() {
		return identity.ContentID{}
	}
	return issuer.module
}

func (issuer OccurrenceMount) ProgramID() identity.ContentID {
	if !issuer.valid() {
		return identity.ContentID{}
	}
	return issuer.programID
}

// IndexAccessForOccurrence resolves one exact mounted Read/Write occurrence
// through Heap's sealed inverse map.  Artifact and Program identity remain
// part of the opaque issuer fence; duplicate mounts cannot alias.
func (issuer OccurrenceMount) IndexAccessForOccurrence(id identity.ContentID, read bool) (IndexAccess, bool) {
	if !issuer.valid() || !id.Available() {
		return IndexAccess{}, false
	}
	// The inverse map is sealed from the canonical artifact rows, so no
	// artifact occurrence directory needs to be reopened for this lookup.
	schema := Schema{owner: issuer.owner}
	index := schema.owner.indexAccessOrdinals[indexAccessOccurrence{module: issuer.module, id: id}]
	if index == 0 || int(index) > len(schema.owner.indexAccesses) {
		return IndexAccess{}, false
	}
	row := schema.owner.indexAccesses[index-1]
	if row.module != issuer.module || row.programID != issuer.programID || row.isRead != read {
		return IndexAccess{}, false
	}
	access := IndexAccess{owner: issuer.owner, index: index}
	return access, schema.ownsIndexAccess(access)
}

// AllocationRootForOccurrence resolves one exact mounted Program allocation
// through Heap's sealed inverse map.  Allocation kind/form are read from the
// sealed Heap row, so callers do not restate or reconstruct them.
func (issuer OccurrenceMount) AllocationRootForOccurrence(id identity.ContentID) (Key, bool) {
	if !issuer.valid() || !id.Available() {
		return Key{}, false
	}
	schema := Schema{owner: issuer.owner}
	index := schema.owner.programAllocationOrdinals[programAllocationOccurrence{module: issuer.module, allocationID: id}]
	if index == 0 {
		return Key{}, false
	}
	row, ok := schema.owner.rootAt(index)
	if !ok || row.kind != RootAllocation || row.allocation.module != issuer.module || row.allocation.programID != issuer.programID || row.allocation.allocationID != id {
		return Key{}, false
	}
	key := Key{owner: issuer.owner, slot: index}
	return key, schema.OwnsKey(key)
}

// AllocationCount returns the mounted artifact's stable allocation order.
// It is a cold enumeration bridge for binding-specific source receipts; it
// does not expose or duplicate Heap's occurrence inverse.
func (issuer OccurrenceMount) AllocationCount() int {
	if !issuer.valid() {
		return 0
	}
	program := issuer.snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	count, published := heapallocation.AllocationFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !published {
		return 0
	}
	return count
}

// AllocationAt returns one mounted allocation ID and its exact Heap root in
// artifact order. The occurrence-to-root resolution itself remains the
// sealed Heap O(1) inverse.
func (issuer OccurrenceMount) AllocationAt(index int) (identity.ContentID, Key, bool) {
	if !issuer.valid() {
		return identity.ContentID{}, Key{}, false
	}
	program := issuer.snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	count, countOK := heapallocation.AllocationFamily().Count(&program.Frozen, catalog)
	if !program.Available() || !catalogOK || !countOK || index < 0 || index >= count {
		return identity.ContentID{}, Key{}, false
	}
	allocation, allocationOK := heapallocation.AllocationFamily().At(&program.Frozen, catalog, index)
	if !allocationOK || !allocation.ID().Available() {
		return identity.ContentID{}, Key{}, false
	}
	key, keyOK := issuer.AllocationRootForOccurrence(allocation.ID())
	return allocation.ID(), key, keyOK
}

// AllocationOrdinal returns the stable mounted artifact ordinal for one
// allocation occurrence. The ordinal is a private index into a binding's
// source-receipt vector, never a public identity or a second occurrence map.
func (issuer OccurrenceMount) AllocationOrdinal(id identity.ContentID) (int, bool) {
	if !issuer.valid() || !id.Available() {
		return 0, false
	}
	schema := Schema{owner: issuer.owner}
	index := schema.owner.programAllocationOrdinals[programAllocationOccurrence{module: issuer.module, allocationID: id}]
	if index == 0 {
		return 0, false
	}
	row, ok := schema.owner.rootAt(index)
	if !ok || row.kind != RootAllocation || row.allocation.artifactRow == 0 || row.allocation.module != issuer.module || row.allocation.allocationID != id {
		return 0, false
	}
	ordinal := int(row.allocation.artifactRow - 1)
	program := issuer.snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	count, published := heapallocation.AllocationFamily().Count(&program.Frozen, catalog)
	return ordinal, program.Available() && catalogOK && published && ordinal < count
}

// MountedArtifactCount returns the sealed Link mount denominator without
// copying its rows. Consumers that enumerate mounts use Count/At so a hot
// lookup never allocates an exported slice merely to read owner state.
func (schema Schema) MountedArtifactCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.artifactOrder)
}

// MountedArtifactAt returns one owner-issued mount in the Link's canonical
// order. The row remains authenticated by this exact Heap schema.
func (schema Schema) MountedArtifactAt(index int) (programmount.MountedArtifact, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.artifactOrder) {
		return programmount.MountedArtifact{}, false
	}
	mount := schema.owner.artifactOrder[index]
	canonical, ok := schema.owner.artifacts[mount.ModuleKey]
	return mount, ok && canonical.Snapshot == mount.Snapshot && canonical.ProgramID == mount.ProgramID && mount.Available()
}

// MountedArtifacts returns the artifact mounts this schema sealed, in the Link's
// own mount order. It is Heap's own enumeration of the mount set it admitted,
// so a consumer that needs the whole list reads it from the sealed owner
// instead of holding a second copy beside the schema.
func (schema Schema) MountedArtifacts() []programmount.MountedArtifact {
	if !schema.valid() || len(schema.owner.artifactOrder) == 0 {
		return nil
	}
	return append([]programmount.MountedArtifact(nil), schema.owner.artifactOrder...)
}

// MountedArtifactForModule returns Heap's canonical seal-time artifact receipt
// for one concrete Link module. Consumers that admit a second mounted owner
// must compare this exact artifact pointer and ProgramID; module identity
// alone is not sufficient to fence a no-index module.
func (schema Schema) MountedArtifactForModule(module identity.ContentID) (programmount.MountedArtifact, bool) {
	if !schema.valid() || !module.Available() || schema.owner.artifacts == nil {
		return programmount.MountedArtifact{}, false
	}
	mount, ok := schema.owner.artifacts[module]
	return mount, ok && mount.Available()
}

// SealFailure is the closed admission phase for Heap's cold source
// projection. It is diagnostic-only: a failure never produces a partial
// Schema or changes the ordinary Seal contract.
type SealFailure uint8

const (
	SealFailureNone SealFailure = iota
	SealFailureSource
	SealFailureProgramAllocations
	SealFailureFreshResults
	SealFailureProgramRootsOverflow
	SealFailureBootRoot
	SealFailureRootsOverflow
	SealFailureBootProjection
	SealFailureIndexAccess
	SealFailureUnknownSlot
	SealFailureFinishRootKinds
	SealFailureFinishAtomicKeys
	SealFailureFinishOccurrenceInverses
	SealFailureFinishReferencePotential
	SealFailureFinishContainments
	SealFailureFinishPresentSlots
	SealFailureFinishPresentContainments
	SealFailureFinishWidenRanks
	SealFailureFinishContentID
	SealFailureFinishKeyInverse
	SealFailureFinishAllocationForms
)

func (failure SealFailure) String() string {
	switch failure {
	case SealFailureNone:
		return "none"
	case SealFailureSource:
		return "source"
	case SealFailureProgramAllocations:
		return "program-allocations"
	case SealFailureFreshResults:
		return "fresh-results"
	case SealFailureProgramRootsOverflow:
		return "program-roots-overflow"
	case SealFailureBootRoot:
		return "boot-root"
	case SealFailureRootsOverflow:
		return "roots-overflow"
	case SealFailureBootProjection:
		return "boot-projection"
	case SealFailureIndexAccess:
		return "index-access"
	case SealFailureUnknownSlot:
		return "unknown-slot"
	case SealFailureFinishRootKinds:
		return "finish-root-kinds"
	case SealFailureFinishAtomicKeys:
		return "finish-atomic-keys"
	case SealFailureFinishOccurrenceInverses:
		return "finish-occurrence-inverses"
	case SealFailureFinishReferencePotential:
		return "finish-reference-potential"
	case SealFailureFinishContainments:
		return "finish-containments"
	case SealFailureFinishPresentSlots:
		return "finish-present-slots"
	case SealFailureFinishPresentContainments:
		return "finish-present-containments"
	case SealFailureFinishWidenRanks:
		return "finish-widen-ranks"
	case SealFailureFinishContentID:
		return "finish-content-id"
	case SealFailureFinishKeyInverse:
		return "finish-key-inverse"
	case SealFailureFinishAllocationForms:
		return "finish-allocation-forms"
	default:
		return "unknown"
	}
}

type schema struct {
	linkOwner link.OwnerCapability
	id        identity.ContentID

	roots            []rootRow // physical Program roots followed by Boot roots
	programRootCount uint32
	bootIndex        map[identity.ContentID]uint32
	// allocationFormRoots and allocationFormOrdinal are the sealed dense
	// global directory of Program allocation roots grouped by constructor
	// form, and its exact inverse indexed by root slot. They are published as
	// the heap axis's per-form candidate relations.
	allocationFormRoots   [allocationFormDirectoryCount][]uint32
	allocationFormOrdinal []uint32
	fresh                 *fresh.Catalog
	// atomicKeys is the exact finite universe of selectors that may become
	// Partition exceptions. atomMaskCounts compresses that universe by its
	// owner-derived possible-kind mask for the fixed-coordinate rank law.
	atomicKeys     []keyAtom
	atomMaskCounts [1 << runtimekind.Count]uint64

	slots        []slotRow
	slotSupport  []slotSupport
	exactSlots   map[uint32]uint32
	exactKeys    []exactKeyRow
	exactIndex   map[keyspace.LiteralValue]uint32
	dynamicSlots map[identity.ContentID]uint32
	unknownSlot  uint32

	payloads       []payloadRow
	payloadSupport []payloadSupport
	payloadIndex   map[payloadRow]uint32
	localSlots     map[rootSlot]struct{}
	bootEntries    map[rootSlot]bootEntryRow
	bootEntryOrder []rootSlot
	bootInitials   map[rootPayload]bootEntryRow

	metatableRoutes []metatableRouteRow
	// fields and indexAccesses are Heap's direct sealed projections of Flow's
	// AccessGeometry. No authored Flow owner or generic Lens/access graph is
	// retained after sealing.
	fields []fieldRow

	indexAccesses []indexAccessRow

	// These inverse maps are derived only after every semantic root and
	// access row has sealed.  They are occurrence indexes, not a second
	// identity plane: each value is the already-issued dense Key/IndexAccess
	// selector for the exact owner-fenced Program tuple.
	programAllocationOrdinals map[programAllocationOccurrence]uint32
	indexAccessOrdinals       map[indexAccessOccurrence]uint32
	// keyByID is Heap's one sealed inverse for its owner-issued KeyID. It is
	// populated only after ContentID and every root row have sealed; values are
	// dense Key selectors, never a second identity or source representation.
	keyByID        map[identity.ContentID]uint32
	freshSlotsByID []uint32
	artifacts      map[identity.ContentID]programmount.MountedArtifact
	// artifactOrder is the same sealed mount set in the Link's own mount
	// order. The map answers by module; this answers "which mounts, in which
	// order", so a consumer never carries a second copy of the mount list.
	artifactOrder []programmount.MountedArtifact

	// These are sealed finite denominators for Heap's canonical Mu carrier;
	// they are rank witnesses, never work budgets or cardinality caps.
	referenceCount       uint64
	presentPotential     uint64
	fixedObjectRankBound uint64
	maxObjectRankSum     uint64
	bottom               Value
	top                  Value
}

type heapSealContext struct {
	project  *linkproject.Component
	boundary *linkboundary.Component
	host     *linkhost.Component
}

// heapBuilder is a stack-confined cold authority.  It embeds the eventually
// published scalar schema only while deriving its rows; no builder value or
// Link component can cross SealWithArtifacts' return boundary.
type heapBuilder struct {
	*schema
	seal heapSealContext
}

func (owner *heapBuilder) sealProject() *linkproject.Component {
	if owner == nil {
		return nil
	}
	return owner.seal.project
}
func (owner *heapBuilder) sealBoundary() *linkboundary.Component {
	if owner == nil {
		return nil
	}
	return owner.seal.boundary
}
func (owner *heapBuilder) sealHost() *linkhost.Component {
	if owner == nil {
		return nil
	}
	return owner.seal.host
}

type rootRow struct {
	kind          RootKind
	allocation    allocationSource
	bootID        identity.ContentID
	bootImmutable bool
	// bootContentID and bootValue are the sealed Heap-owned bootstrap row.
	// Bootstrap rules consume this row through Key; they never carry a second
	// Root image or rebuild the immutable object at solve time.
	bootContentID identity.ContentID
	bootValue     Value
	fieldStart    uint32
	fieldCount    uint32
}

// AllocationKind is the closed Program allocation family.  It belongs to
// Heap because a Heap Key, not Link, is the sole allocation coordinate.
type AllocationKind uint8

const (
	AllocationInvalid AllocationKind = iota
	AllocationTable
	AllocationClosure
)

// AllocationForm is the sealed constructor geometry of one Heap allocation
// root. The ordinals are the occurrence Code the artifact compiler writes for
// allocation rows.
type AllocationForm uint8

const (
	AllocationFormInvalid AllocationForm = iota
	AllocationFormEmpty
	AllocationFormClosed
	AllocationFormFinalOpen
)

func (form AllocationForm) Valid() bool {
	return form >= AllocationFormEmpty && form <= AllocationFormFinalOpen
}

func sealedAllocationKind(role heapallocation.Role) (AllocationKind, bool) {
	switch role {
	case heapallocation.RoleTable:
		return AllocationTable, true
	case heapallocation.RoleClosure:
		return AllocationClosure, true
	default:
		return AllocationInvalid, false
	}
}

func sealedAllocationForm(form heapallocation.Form) (AllocationForm, bool) {
	switch form {
	case heapallocation.FormEmpty:
		return AllocationFormEmpty, true
	case heapallocation.FormClosed:
		return AllocationFormClosed, true
	case heapallocation.FormFinalOpen:
		return AllocationFormFinalOpen, true
	default:
		return AllocationFormInvalid, false
	}
}

func flowFieldKind(kind heapallocation.FieldKind) (flowkind.FieldKind, bool) {
	switch kind {
	case heapallocation.FieldKindList:
		return flowkind.FieldList, true
	case heapallocation.FieldKindName:
		return flowkind.FieldName, true
	case heapallocation.FieldKindExact:
		return flowkind.FieldExact, true
	case heapallocation.FieldKindKey:
		return flowkind.FieldKey, true
	default:
		return 0, false
	}
}

type allocationSource struct {
	// module is the compact mounted-artifact identity.  A module key is
	// deliberately mount-local: duplicate mounts of one Program therefore
	// remain distinct without retaining the Link Project Shard authority.
	module       identity.ContentID
	kind         AllocationKind
	form         AllocationForm
	programID    identity.ContentID
	allocationID identity.ContentID
	artifactRow  uint32
	rootValueID  identity.ContentID
}

// exactKeyRow is Heap's detached exact-key universe.  Link's canonical
// quotient is consumed while sealing; the published schema keeps only the
// normalized scalar literal and a Heap-local dense ordinal.
type exactKeyRow struct{ literal keyspace.LiteralValue }

type fieldSource struct {
	root         uint32
	allocationID identity.ContentID
	fieldID      identity.ContentID
}

type fieldRow struct {
	fieldSource
	kind         flowkind.FieldKind
	width        int
	finalOpen    bool
	normalized   keyspace.Key
	normalizedOK bool
	slot         uint32
	payload      uint32
	openTail     uint32
	artifactRow  uint32
	valuesRow    uint32
}

type payloadKind uint8

const (
	payloadInvalid payloadKind = iota
	payloadValues
	payloadInitial
)

type slotRow struct {
	kind    SlotKind
	exact   uint32
	dynamic identity.ContentID
	field   uint32
}

type payloadRow struct {
	kind     payloadKind
	module   identity.ContentID
	valuesID identity.ContentID
	index    uint32
	initial  vocabulary.InitialValue
}

type bootEntryRow struct {
	raw        RawPresence
	payload    uint32
	initial    vocabulary.InitialValue
	kind       vocabulary.InitialValueKind
	valueChild uint32
	mutability vocabulary.InitialMutability
}

// metatableRouteRow is the sealed bootstrap projection of an existing Link
// primitive attachment. Mutable table attachment remains ordinary Heap state
// and never mutates this cold ledger.
type metatableRouteRow struct {
	primitive vocabulary.InitialValueKind
	metatable uint32
	role      materialization.Role
}

// slotSupport is the cold root-incidence law for one symbolic partition.
// Exact and unknown partitions are semantic key classes and therefore apply
// to every root. A dynamic access is likewise global because Value/identity
// selects its base at runtime. Constructor-only dynamic and open-tail
// partitions retain only their exact creation roots.
type slotSupport struct {
	global bool
}

type rootSlot struct {
	root uint32
	slot uint32
}

type rootPayload struct {
	root    uint32
	payload uint32
}

type indexAccessRow struct {
	module      identity.ContentID
	programID   identity.ContentID
	occurrence  identity.ContentID
	isRead      bool
	baseValueID identity.ContentID
	keyValueID  identity.ContentID
	valuesID    identity.ContentID
	position    int
	dynamic     bool
	resultID    identity.ContentID
	slot        uint32
	payload     uint32
}

// programAllocationOccurrence is the one exact inverse key for a Program
// aggregate. Shard distinguishes duplicate mounts; allocationID is the
// opaque Program proof identity. No raw Term is part of the inverse key.
type programAllocationOccurrence struct {
	module       identity.ContentID
	allocationID identity.ContentID
}

// indexAccessOccurrence is the portable mounted occurrence key already
// issued by Program's semantic-path authority. It contains no raw Term or
// mount ordinal; duplicate mounts remain distinct through ModuleKey.
type indexAccessOccurrence struct {
	module identity.ContentID
	id     identity.ContentID
}

// Field is one schema-issued Program TableField route.  It has no Link
// counterpart and no independently constructible root/term pair.
type Field struct {
	owner *schema
	index uint32
}

// IndexAccess is one Heap-scoped direct typed AccessGeometry candidate row.
type IndexAccess struct {
	owner *schema
	index uint32
}

// IndexGeometry is Heap's direct immutable copy of one sealed candidate row.
// Read rows have Position -1 and no Values receipt; write rows retain their
// authored Values occurrence identity and position.
type IndexGeometry struct {
	// Module is the compact mounted-artifact receipt. It replaces the
	// Link-owned Shard proof after sealing.
	Module    identity.ContentID
	ProgramID identity.ContentID
	// BaseValueID and KeyValueID are Link-owned Boundary Value identities,
	// not reusable artifact semantic IDs. ValuesID remains the artifact-owned
	// Values occurrence used to resolve the write payload through Pack.
	BaseValueID identity.ContentID
	KeyValueID  identity.ContentID
	ValuesID    identity.ContentID
	Position    int
	DynamicKey  bool
	Read        bool
}

// payloadSupport prevents a source payload from being paired with an
// unrelated key or allocation root. Assignment payloads are factorized by
// slot and remain applicable to any selected base root; constructor payloads
// retain their exact root/slot incidence.
type payloadSupport struct {
	globalSlots map[uint32]struct{}
	local       map[rootSlot]struct{}
	// roots is the exact source-root projection used only for canonical
	// Unknown coarsening.  It preserves the original payload universe rather
	// than admitting payloads merely because their target slot is common.
	roots map[uint32]struct{}
}

// SealWithArtifacts is the artifact-native Heap admission seam. It consumes
// only immutable artifact rows and Link's sealed substitution inverses; it
// never reopens a mounted Program or Flow geometry.
func SealWithArtifacts(source *link.Link, mounts []programmount.MountedArtifact) (Schema, SealFailure) {
	builder, byModule, failure := newArtifactSchemaOwner(source, mounts)
	if failure != SealFailureNone {
		return Schema{}, failure
	}
	owner := builder.schema
	owner.artifacts = byModule
	if !builder.addArtifactAllocations(byModule) {
		return Schema{}, SealFailureProgramAllocations
	}
	owner.programRootCount = uint32(len(owner.roots))
	mountedPrograms := make([]fresh.MountedProgram, 0, len(owner.artifactOrder))
	for _, mount := range owner.artifactOrder {
		if !mount.Available() {
			return Schema{}, SealFailureFreshResults
		}
		program := mount.Snapshot.Program()
		if !program.Available() {
			return Schema{}, SealFailureFreshResults
		}
		mountedPrograms = append(mountedPrograms, fresh.MountedProgram{Module: mount.ModuleKey, Program: program})
	}
	freshCatalog, freshOK := fresh.Build(source, mountedPrograms)
	if !freshOK {
		return Schema{}, SealFailureFreshResults
	}
	owner.fresh = freshCatalog
	if owner.rootCount() > uint64(^uint32(0)) {
		return Schema{}, SealFailureProgramRootsOverflow
	}
	for index := 0; index < source.Host().BootRoots().Count(); index++ {
		root, ok := source.Host().BootRoots().At(index)
		rootID, idOK := source.Host().BootRoots().ID(root)
		if !ok || !idOK || !builder.addBootRoot(root, rootID) {
			return Schema{}, SealFailureBootRoot
		}
	}
	if owner.rootCount() > uint64(^uint32(0)) {
		return Schema{}, SealFailureRootsOverflow
	}
	if !builder.addBootEntries() || !builder.addBootMetatableRoutes() {
		return Schema{}, SealFailureBootProjection
	}
	if !builder.addArtifactIndexes(byModule) {
		return Schema{}, SealFailureIndexAccess
	}
	owner.unknownSlot = owner.ensureUnknownSlot()
	if owner.unknownSlot == 0 {
		return Schema{}, SealFailureUnknownSlot
	}
	if failure := builder.finishWithFailure(); failure != SealFailureNone {
		return Schema{}, failure
	}
	return Schema{owner: owner}, SealFailureNone
}

func newArtifactSchemaOwner(source *link.Link, mounts []programmount.MountedArtifact) (*heapBuilder, map[identity.ContentID]programmount.MountedArtifact, SealFailure) {
	if source == nil {
		return nil, nil, SealFailureSource
	}
	linkOwner := source.OwnerCapability()
	if !linkOwner.Available() || source.Project() == nil || len(mounts) != source.Project().Mounts().Count() {
		return nil, nil, SealFailureSource
	}
	byModule := make(map[identity.ContentID]programmount.MountedArtifact, len(mounts))
	for _, mount := range mounts {
		if !mount.Available() {
			return nil, nil, SealFailureProgramAllocations
		}
		if _, duplicate := byModule[mount.ModuleKey]; duplicate {
			return nil, nil, SealFailureProgramAllocations
		}
		byModule[mount.ModuleKey] = mount
	}
	ordered := make([]programmount.MountedArtifact, 0, len(mounts))
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, shardOK := source.Project().Mounts().At(index)
		module, moduleOK := source.Project().ModuleKey(shard)
		programID, programOK := source.Project().Mounts().ProgramID(shard)
		mount, mountOK := byModule[module]
		if !shardOK || !moduleOK || !programOK || !mountOK || !mount.Available() || mount.ModuleKey != module || mount.ProgramID != programID || mount.Snapshot.ProgramID() != programID {
			return nil, nil, SealFailureProgramAllocations
		}
		ordered = append(ordered, mount)
	}
	owner := &schema{linkOwner: linkOwner, artifactOrder: ordered, bootIndex: make(map[identity.ContentID]uint32), exactSlots: make(map[uint32]uint32), exactIndex: make(map[keyspace.LiteralValue]uint32), payloadIndex: make(map[payloadRow]uint32), localSlots: make(map[rootSlot]struct{}), bootEntries: make(map[rootSlot]bootEntryRow), bootInitials: make(map[rootPayload]bootEntryRow), dynamicSlots: make(map[identity.ContentID]uint32)}
	builder := &heapBuilder{schema: owner, seal: heapSealContext{project: source.Project(), boundary: source.Boundary(), host: source.Host()}}
	return builder, byModule, SealFailureNone
}

// mountedValue is Heap's sole artifact-to-Boundary substitution. The artifact
// supplies a Program-issued span identity and Boundary reissues its opaque
// mounted Value; neither an authored Term nor a Program handle survives.
func (owner *heapBuilder) mountedValue(module, span identity.ContentID) (linkboundary.Value, bool) {
	if owner == nil || owner.sealProject() == nil || !module.Available() || !span.Available() {
		return linkboundary.Value{}, false
	}
	value, valueOK := owner.sealBoundary().Values().ForMountedSpan(module, span)
	return value, valueOK && value != (linkboundary.Value{})
}

func (owner *heapBuilder) addArtifactAllocations(mounts map[identity.ContentID]programmount.MountedArtifact) bool {
	if owner == nil || owner.sealProject() == nil {
		return false
	}
	for mountIndex := 0; mountIndex < owner.sealProject().Mounts().Count(); mountIndex++ {
		shard, shardOK := owner.sealProject().Mounts().At(mountIndex)
		module, moduleOK := owner.sealProject().ModuleKey(shard)
		mount, mountOK := mounts[module]
		if !shardOK || !moduleOK || !mountOK {
			return false
		}
		artifact := mount.Snapshot
		if artifact == nil {
			return false
		}
		program := artifact.Program()
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		allocationCount, allocationsPublished := heapallocation.AllocationFamily().Count(&program.Frozen, catalog)
		valuesCount, valuesPublished := programschema.ValuesFamily().Count(&program.Frozen, catalog)
		if !program.Available() || !catalogOK || !allocationsPublished || !valuesPublished {
			return false
		}
		valuesRows := make(map[identity.ContentID]uint32, valuesCount)
		for valuesIndex := 0; valuesIndex < valuesCount; valuesIndex++ {
			values, valuesOK := programschema.ValuesFamily().At(&program.Frozen, catalog, valuesIndex)
			if !valuesOK || !values.ID().Available() || uint64(valuesIndex) >= uint64(^uint32(0)) {
				return false
			}
			if _, duplicate := valuesRows[values.ID()]; duplicate {
				return false
			}
			valuesRows[values.ID()] = uint32(valuesIndex + 1)
		}
		for index := 0; index < allocationCount; index++ {
			allocation, allocationOK := heapallocation.AllocationFamily().At(&program.Frozen, catalog, index)
			root, rootOK := owner.mountedValue(mount.ModuleKey, allocation.RootSpan())
			rootID, rootIDOK := owner.sealBoundary().Values().ID(root)
			if !allocationOK || !rootOK || !rootIDOK || allocation.ID() == (identity.ContentID{}) || uint64(index) >= uint64(^uint32(0)) {
				return false
			}
			kind, kindOK := sealedAllocationKind(allocation.Role())
			form, formOK := sealedAllocationForm(allocation.Form())
			if !kindOK || !formOK {
				return false
			}
			owner.roots = append(owner.roots, rootRow{kind: RootAllocation, allocation: allocationSource{module: mount.ModuleKey, kind: kind, form: form, programID: mount.ProgramID, allocationID: allocation.ID(), artifactRow: uint32(index + 1), rootValueID: rootID}})
			rootIndex := uint32(len(owner.roots))
			if kind != AllocationTable {
				continue
			}
			fieldOffset, fieldCount, fieldsOK := allocation.FieldSpan()
			if !fieldsOK {
				return false
			}
			for fieldIndex := uint32(0); fieldIndex < fieldCount; fieldIndex++ {
				field, fieldOK := heapallocation.FieldFamily().At(&program.Frozen, catalog, int(fieldOffset+fieldIndex))
				if !fieldOK || !owner.addArtifactField(rootIndex, mount, allocation, uint32(index+1), field, uint32(fieldIndex+1), valuesRows[field.ValuesID()]) {
					return false
				}
			}
		}
	}
	return true
}

func (owner *heapBuilder) addArtifactField(root uint32, mount programmount.MountedArtifact, allocation heapallocation.Allocation, allocationRow uint32, field heapallocation.Field, fieldRowIndex, valuesRow uint32) bool {
	if owner == nil || root == 0 || int(root) > len(owner.roots) || !mount.Available() || !allocation.Available() || allocationRow == 0 || !field.Available() || fieldRowIndex == 0 || valuesRow == 0 {
		return false
	}
	row := owner.roots[root-1]
	if row.kind != RootAllocation || row.allocation.module != mount.ModuleKey || row.allocation.kind != AllocationTable || row.allocation.allocationID != allocation.ID() {
		return false
	}
	_, width, finalOpen, valuesOK := field.Values()
	if !valuesOK {
		return false
	}
	var slotID uint32
	var normalized keyspace.Key
	var normalizedOK bool
	switch field.Kind() {
	case heapallocation.FieldKindKey:
		keyValue, keyOK := owner.mountedValue(mount.ModuleKey, field.SelectorSpan())
		if !keyOK {
			return false
		}
		keyValueID, keyValueIDOK := owner.sealBoundary().Values().ID(keyValue)
		if !keyValueIDOK {
			return false
		}
		slotID = owner.addDynamicSlot(keyValueID)
	case heapallocation.FieldKindList, heapallocation.FieldKindName, heapallocation.FieldKindExact:
		raw, rawOK := field.NormalizedKey()
		normalized, normalizedOK = keyspace.Key(raw), rawOK
		if !normalizedOK || normalized == 0 {
			return false
		}
		key, keyOK := owner.sealProject().Keys().ForMounted(mount.ModuleKey, normalized)
		literal, literalOK := owner.sealProject().Keys().Exact(key)
		if !keyOK || !literalOK {
			return false
		}
		slotID = owner.addExactSlot(literal)
	default:
		return false
	}
	if slotID == 0 || !owner.addLocalSlot(slotID, root) {
		return false
	}
	payloadID := owner.addArtifactPayload(payloadRow{kind: payloadValues, module: mount.ModuleKey, valuesID: field.ValuesID()})
	if payloadID == 0 || !owner.addLocalPayload(payloadID, root, slotID) {
		return false
	}
	dense := uint32(len(owner.fields) + 1)
	kind, kindOK := flowFieldKind(field.Kind())
	if !kindOK {
		return false
	}
	owner.fields = append(owner.fields, fieldRow{fieldSource: fieldSource{root: root, allocationID: allocation.ID(), fieldID: field.ID()}, kind: kind, width: width, finalOpen: finalOpen, normalized: normalized, normalizedOK: normalizedOK, slot: slotID, payload: payloadID, artifactRow: fieldRowIndex, valuesRow: valuesRow})
	if owner.roots[root-1].fieldStart == 0 {
		owner.roots[root-1].fieldStart = dense
	}
	owner.roots[root-1].fieldCount++
	if finalOpen {
		id := owner.addSlot(slotRow{kind: SlotOpenTail, field: dense})
		if id == 0 || !owner.addLocalSlot(id, root) || !owner.addLocalPayload(payloadID, root, id) {
			return false
		}
		owner.fields[dense-1].openTail = id
	}
	return true
}

func (owner *schema) addArtifactPayload(row payloadRow) uint32 {
	if owner == nil || row.kind != payloadValues || !row.module.Available() || !row.valuesID.Available() {
		return 0
	}
	if id := owner.payloadIndex[row]; id != 0 {
		return id
	}
	if uint64(len(owner.payloads)) >= uint64(^uint32(0)) {
		return 0
	}
	owner.payloads = append(owner.payloads, row)
	owner.payloadSupport = append(owner.payloadSupport, payloadSupport{})
	id := uint32(len(owner.payloads))
	owner.payloadIndex[row] = id
	return id
}

func (owner *heapBuilder) addArtifactIndexes(mounts map[identity.ContentID]programmount.MountedArtifact) bool {
	if owner == nil || owner.sealProject() == nil {
		return false
	}
	for mountIndex := 0; mountIndex < owner.sealProject().Mounts().Count(); mountIndex++ {
		shard, shardOK := owner.sealProject().Mounts().At(mountIndex)
		module, moduleOK := owner.sealProject().ModuleKey(shard)
		mount, mountOK := mounts[module]
		if !shardOK || !moduleOK || !mountOK {
			return false
		}
		artifact := mount.Snapshot
		if artifact == nil {
			return false
		}
		program := artifact.Program()
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		indexCount, indexesPublished := heapindex.Family().Count(&program.Frozen, catalog)
		if !program.Available() || !catalogOK || !indexesPublished {
			return false
		}
		for index := 0; index < indexCount; index++ {
			access, accessOK := heapindex.Family().At(&program.Frozen, catalog, index)
			if !accessOK || !access.Available() {
				return false
			}
			baseValue, baseValueOK := owner.mountedValue(mount.ModuleKey, access.BaseSpan())
			if !baseValueOK {
				return false
			}
			baseValueID, baseValueIDOK := owner.sealBoundary().Values().ID(baseValue)
			if !baseValueIDOK {
				return false
			}
			dynamic := access.DynamicKeySpan().Available()
			var keyValueID identity.ContentID
			var keyValueIDOK bool
			var slot uint32
			if dynamic {
				keyValue, keyOK := owner.mountedValue(mount.ModuleKey, access.DynamicKeySpan())
				if !keyOK {
					return false
				}
				keyValueID, keyValueIDOK = owner.sealBoundary().Values().ID(keyValue)
				if !keyValueIDOK {
					return false
				}
				slot = owner.addDynamicSlot(keyValueID)
				if slot == 0 || !owner.markGlobalSlot(slot) {
					return false
				}
			} else {
				exact, exactOK := access.ExactKey()
				if !exactOK {
					return false
				}
				key, keyOK := owner.sealProject().Keys().ForMounted(mount.ModuleKey, keyspace.Key(exact))
				literal, literalOK := owner.sealProject().Keys().Exact(key)
				if !keyOK || !literalOK {
					return false
				}
				slot = owner.addExactSlot(literal)
			}
			if access.Read() {
				result, resultOK := owner.mountedValue(mount.ModuleKey, access.ResultSpan())
				if !resultOK {
					return false
				}
				resultID, resultIDOK := owner.sealBoundary().Values().ID(result)
				if !resultIDOK {
					return false
				}
				owner.indexAccesses = append(owner.indexAccesses, indexAccessRow{module: mount.ModuleKey, programID: mount.ProgramID, occurrence: access.ID(), isRead: true, baseValueID: baseValueID, keyValueID: keyValueID, position: -1, dynamic: dynamic, resultID: resultID, slot: slot})
				continue
			}
			_, position, valuesOK := access.Values()
			if !valuesOK {
				return false
			}
			payload := owner.addArtifactPayload(payloadRow{kind: payloadValues, module: mount.ModuleKey, valuesID: access.ValuesID(), index: uint32(position)})
			if payload == 0 || !owner.addGlobalPayload(payload, slot) {
				return false
			}
			owner.indexAccesses = append(owner.indexAccesses, indexAccessRow{module: mount.ModuleKey, programID: mount.ProgramID, occurrence: access.ID(), baseValueID: baseValueID, keyValueID: keyValueID, valuesID: access.ValuesID(), position: position, dynamic: dynamic, slot: slot, payload: payload})
		}
	}
	return true
}

// freshCount returns the catalog-owned fresh-root denominator without scanning
// Heap Keys or reconstructing Target/Application products.
func (owner *schema) freshCount() uint64 {
	if owner == nil || owner.fresh == nil {
		return 0
	}
	return owner.fresh.Count()
}

func (owner *schema) freshRoot(slot uint32) (fresh.Root, bool) {
	if owner == nil || owner.fresh == nil || slot <= owner.programRootCount {
		return fresh.Root{}, false
	}
	index := uint64(slot) - uint64(owner.programRootCount) - 1
	if index >= owner.fresh.Count() {
		return fresh.Root{}, false
	}
	return owner.fresh.At(index)
}

func (owner *schema) rootCount() uint64 { return uint64(len(owner.roots)) + owner.freshCount() }

// rootAt validates one Heap slot and returns its physical row shape. Fresh
// rows are owned by the internal catalog; their semantic columns are queried
// directly from that catalog by fresh-root accessors.
func (owner *schema) rootAt(slot uint32) (rootRow, bool) {
	if owner == nil || slot == 0 || uint64(slot) > owner.rootCount() {
		return rootRow{}, false
	}
	if slot <= owner.programRootCount {
		return owner.roots[slot-1], true
	}
	freshStart := uint64(owner.programRootCount) + 1
	if uint64(slot) >= freshStart {
		freshIndex := uint64(slot) - freshStart
		if freshIndex < owner.freshCount() {
			_, freshOK := owner.freshRoot(slot)
			return rootRow{kind: RootAllocation}, freshOK
		}
	}
	bootIndex := uint64(slot) - owner.freshCount()
	if bootIndex == 0 || bootIndex > uint64(len(owner.roots)) {
		return rootRow{}, false
	}
	return owner.roots[bootIndex-1], true
}

func (owner *heapBuilder) addBootRoot(root linkhost.BootRoot, rootID identity.ContentID) bool {
	if owner == nil || owner.sealProject() == nil || !rootID.Available() || owner.bootIndex[rootID] != 0 || uint64(len(owner.roots)) >= uint64(^uint32(0)) {
		return false
	}
	_, initial, ok := owner.sealHost().BootRoots().Mapping(root)
	if !ok || initial == 0 {
		return false
	}
	contract, contractOK := owner.sealBoundary().Target()
	if !contractOK || contract == nil {
		return false
	}
	shape, shapeOK := contract.InitialRootBootShape(initial)
	immutable, immutableOK := contract.BootShapeImmutable(shape)
	if !shapeOK || !immutableOK {
		return false
	}
	owner.roots = append(owner.roots, rootRow{kind: RootBoot, bootID: rootID, bootImmutable: immutable})
	virtual := uint64(len(owner.roots)) + owner.freshCount()
	if virtual > uint64(^uint32(0)) {
		return false
	}
	owner.bootIndex[rootID] = uint32(virtual)
	return true
}

// addBootEntries projects Target's immutable per-initial-root ledger through
// Link's actor-local BootRoot image and its one shared exact-key universe.
// This is a cold schema projection only: it neither creates a runtime value
// nor seeds recurrent Heap state.
func (owner *heapBuilder) addBootEntries() bool {
	if owner == nil || owner.sealProject() == nil {
		return false
	}
	contract, ok := owner.sealBoundary().Target()
	if !ok || contract == nil {
		return false
	}
	type entryRow struct {
		literal    keyspace.LiteralValue
		value      vocabulary.InitialValue
		kind       vocabulary.InitialValueKind
		mutability vocabulary.InitialMutability
	}
	entries := make(map[vocabulary.InitialRoot][]entryRow)
	for index := 0; index < contract.InitialEntryCount(); index++ {
		entryRoot, exact, initialValue, mutability, entryOK := contract.InitialEntryAt(index)
		if !entryOK || initialValue == 0 || (mutability != vocabulary.InitialMutable && mutability != vocabulary.InitialFrozen) {
			return false
		}
		kind, valid := contract.InitialValueKind(initialValue)
		key, keyOK := owner.sealProject().Keys().ForTarget(contract, exact)
		literal, literalOK := owner.sealProject().Keys().Exact(key)
		if !valid || kind == vocabulary.InitialValueInvalid || !keyOK || !literalOK {
			return false
		}
		entries[entryRoot] = append(entries[entryRoot], entryRow{literal: literal, value: initialValue, kind: kind, mutability: mutability})
	}
	for _, root := range owner.roots {
		if root.kind != RootBoot {
			continue
		}
		virtualRoot := owner.bootIndex[root.bootID]
		if virtualRoot == 0 {
			return false
		}
		boot, bootOK := owner.bootRootForID(root.bootID)
		actor, initial, mapped := owner.sealHost().BootRoots().Mapping(boot)
		if !bootOK || !mapped {
			return false
		}
		for _, entry := range entries[initial] {
			slot := owner.addExactSlot(entry.literal)
			if slot == 0 || !owner.addLocalSlot(slot, virtualRoot) {
				return false
			}
			raw, rawOK := initialValueRawPresence(entry.kind)
			if !rawOK {
				return false
			}
			row := bootEntryRow{raw: raw, kind: entry.kind, mutability: entry.mutability}
			// Target preserves Nil and Absent as distinct contract values.  Both
			// project to raw absence in Heap: Lua assignment of nil deletes the
			// slot, so Heap must never materialize a present nil payload.
			if raw == RawPresent {
				payload := owner.addPayload(payloadRow{kind: payloadInitial, initial: entry.value})
				if payload == 0 || !owner.addLocalPayload(payload, virtualRoot, slot) {
					return false
				}
				row.raw, row.payload, row.initial = RawPresent, payload, entry.value
				if entry.kind == vocabulary.InitialValueRoot {
					initialRoot, rootValue := contract.InitialValueRoot(entry.value)
					child, found := owner.sealHost().BootRoots().For(actor, initialRoot)
					childIDRaw, childIDOK := owner.sealHost().BootRoots().ID(child)
					childID := owner.bootIndex[childIDRaw]
					if !rootValue || !found || !childIDOK || childID == 0 {
						return false
					}
					row.valueChild = childID
				}
			}
			rootSlot := rootSlot{root: virtualRoot, slot: slot}
			if owner.bootEntries[rootSlot].mutability != vocabulary.InitialMutabilityInvalid {
				return false
			}
			owner.bootEntries[rootSlot] = row
			if row.raw == RawPresent {
				index := rootPayload{root: rootSlot.root, payload: row.payload}
				if previous, exists := owner.bootInitials[index]; exists && (previous.initial != row.initial || previous.valueChild != row.valueChild) {
					return false
				}
				owner.bootInitials[index] = row
			}
		}
	}
	return true
}

// initialValueRawPresence is the one Target-to-Heap boundary law. Target
// retains the Nil/Absent distinction for contract consumers, but neither is a
// raw stored table value in Lua. An unclassified Target value cannot silently
// become a Heap fact: sealing rejects it until its raw-storage law is named.
func initialValueRawPresence(kind vocabulary.InitialValueKind) (RawPresence, bool) {
	switch kind {
	case vocabulary.InitialValueNil, vocabulary.InitialValueAbsent:
		return RawAbsent, true
	case vocabulary.InitialValueBoolean,
		vocabulary.InitialValueInteger,
		vocabulary.InitialValueFloat,
		vocabulary.InitialValueString,
		vocabulary.InitialValueRoot,
		vocabulary.InitialValueOperation,
		vocabulary.InitialValueDeniedOperation:
		return RawPresent, true
	default:
		return RawInvalid, false
	}
}

func (owner *heapBuilder) addBootMetatableRoutes() bool {
	if owner == nil || owner.sealProject() == nil {
		return false
	}
	seen := make(map[linkhost.BootMetatableAttachment]struct{})
	for index := 0; index < owner.sealHost().Attachments().Count(); index++ {
		attachment, ok := owner.sealHost().Attachments().At(index)
		if !ok {
			return false
		}
		if _, duplicate := seen[attachment]; duplicate {
			return false
		}
		seen[attachment] = struct{}{}
		base, metatable, mapped := owner.sealHost().Attachments().Mapping(attachment)
		metatableIDRaw, metatableIDOK := owner.sealHost().BootRoots().ID(metatable)
		metatableID := owner.bootIndex[metatableIDRaw]
		if !mapped || !metatableIDOK || base == vocabulary.InitialValueInvalid || metatableID == 0 || uint64(len(owner.metatableRoutes)) >= uint64(^uint32(0)) {
			return false
		}
		owner.metatableRoutes = append(owner.metatableRoutes, metatableRouteRow{
			primitive: base,
			metatable: metatableID,
			role:      materialization.Exact,
		})
	}
	return true
}

// bootRootForID consumes Host's authority during sealing only.  The returned
// coordinate never enters a published Heap row.
func (owner *heapBuilder) bootRootForID(id identity.ContentID) (linkhost.BootRoot, bool) {
	if owner == nil || !id.Available() || owner.sealHost() == nil {
		return linkhost.BootRoot{}, false
	}
	roots := owner.sealHost().BootRoots()
	for index := 0; index < roots.Count(); index++ {
		root, rootOK := roots.At(index)
		rootID, idOK := roots.ID(root)
		if rootOK && idOK && rootID == id {
			return root, true
		}
	}
	return linkhost.BootRoot{}, false
}

func (owner *schema) addExactSlot(literal keyspace.LiteralValue) uint32 {
	if owner == nil || literal.Kind == 0 {
		return 0
	}
	exact := owner.exactIndex[literal]
	if exact == 0 {
		if uint64(len(owner.exactKeys)) >= uint64(^uint32(0)) {
			return 0
		}
		owner.exactKeys = append(owner.exactKeys, exactKeyRow{literal: literal})
		exact = uint32(len(owner.exactKeys))
		owner.exactIndex[literal] = exact
	}
	if id := owner.exactSlots[exact]; id != 0 {
		return id
	}
	id := owner.addSlot(slotRow{kind: SlotExact, exact: exact})
	if id != 0 {
		owner.exactSlots[exact] = id
		owner.slotSupport[id-1].global = true
	}
	return id
}

func (owner *schema) addDynamicSlot(dynamic identity.ContentID) uint32 {
	if owner == nil || !dynamic.Available() {
		return 0
	}
	if id := owner.dynamicSlots[dynamic]; id != 0 {
		return id
	}
	id := owner.addSlot(slotRow{kind: SlotDynamic, dynamic: dynamic})
	if id != 0 {
		owner.dynamicSlots[dynamic] = id
	}
	return id
}

func (owner *schema) addSlot(row slotRow) uint32 {
	if owner == nil || uint64(len(owner.slots)) >= uint64(^uint32(0)) {
		return 0
	}
	owner.slots = append(owner.slots, row)
	owner.slotSupport = append(owner.slotSupport, slotSupport{global: row.kind == SlotUnknown})
	return uint32(len(owner.slots))
}

func (owner *schema) ensureUnknownSlot() uint32 {
	if owner == nil {
		return 0
	}
	if owner.unknownSlot != 0 {
		return owner.unknownSlot
	}
	owner.unknownSlot = owner.addSlot(slotRow{kind: SlotUnknown})
	return owner.unknownSlot
}

func (owner *heapBuilder) addPayload(row payloadRow) uint32 {
	if owner == nil || owner.sealProject() == nil {
		return 0
	}
	if id := owner.payloadIndex[row]; id != 0 {
		return id
	}
	switch row.kind {
	case payloadInitial:
		contract, ok := owner.sealBoundary().Target()
		if !ok || contract == nil {
			return 0
		}
		if kind, valid := contract.InitialValueKind(row.initial); !valid || kind == vocabulary.InitialValueInvalid {
			return 0
		}
	default:
		return 0
	}
	if uint64(len(owner.payloads)) >= uint64(^uint32(0)) {
		return 0
	}
	owner.payloads = append(owner.payloads, row)
	owner.payloadSupport = append(owner.payloadSupport, payloadSupport{})
	id := uint32(len(owner.payloads))
	owner.payloadIndex[row] = id
	return id
}

func (owner *schema) markGlobalSlot(id uint32) bool {
	if id == 0 || uint64(id) > uint64(len(owner.slotSupport)) {
		return false
	}
	owner.slotSupport[id-1].global = true
	return true
}

func (owner *schema) addLocalSlot(slot, root uint32) bool {
	if slot == 0 || root == 0 || uint64(slot) > uint64(len(owner.slotSupport)) || uint64(root) > owner.rootCount() {
		return false
	}
	owner.localSlots[rootSlot{root: root, slot: slot}] = struct{}{}
	return true
}

func (owner *schema) addGlobalPayload(payload, slot uint32) bool {
	if payload == 0 || slot == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) || uint64(slot) > uint64(len(owner.slots)) {
		return false
	}
	support := &owner.payloadSupport[payload-1]
	if support.globalSlots == nil {
		support.globalSlots = make(map[uint32]struct{})
	}
	support.globalSlots[slot] = struct{}{}
	return true
}

func (owner *schema) addLocalPayload(payload, root, slot uint32) bool {
	if payload == 0 || root == 0 || slot == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) || uint64(root) > owner.rootCount() || uint64(slot) > uint64(len(owner.slots)) {
		return false
	}
	support := &owner.payloadSupport[payload-1]
	if support.local == nil {
		support.local = make(map[rootSlot]struct{})
		support.roots = make(map[uint32]struct{})
	}
	support.local[rootSlot{root: root, slot: slot}] = struct{}{}
	support.roots[root] = struct{}{}
	return true
}

func (owner *heapBuilder) finish() bool {
	return owner.finishWithFailure() == SealFailureNone
}

func (owner *heapBuilder) finishWithFailure() SealFailure {
	if owner == nil || owner.sealProject() == nil || len(owner.slots) == 0 {
		return SealFailureUnknownSlot
	}
	if !owner.buildRootKinds() {
		return SealFailureFinishRootKinds
	}
	if !owner.buildAtomicKeys() {
		return SealFailureFinishAtomicKeys
	}
	// Build the exact inverse only after all Program roots and index geometry
	// rows are complete. A duplicate tuple is an ambiguity in the canonical
	// relation and therefore rejects the whole Heap seal.
	if !owner.sealOccurrenceInverses() {
		return SealFailureFinishOccurrenceInverses
	}
	owner.bootEntryOrder = owner.bootEntryOrder[:0]
	for entry := range owner.bootEntries {
		owner.bootEntryOrder = append(owner.bootEntryOrder, entry)
	}
	sort.Slice(owner.bootEntryOrder, func(left, right int) bool {
		if owner.bootEntryOrder[left].root != owner.bootEntryOrder[right].root {
			return owner.bootEntryOrder[left].root < owner.bootEntryOrder[right].root
		}
		return owner.bootEntryOrder[left].slot < owner.bootEntryOrder[right].slot
	})

	// A present tuple consists solely of sealed source/payload coordinates and
	// two owner-issued containment facts. Each fact is None, Unknown, or one of
	// the exact references. Its finite denominator proves canonical-widening
	// descent; no threshold decides whether a tuple is retained.
	references, ok := owner.referencePotential()
	if !ok {
		return SealFailureFinishReferencePotential
	}
	owner.referenceCount = references
	containments, ok := safeAdd(references, 2)
	if !ok {
		return SealFailureFinishContainments
	}
	present, ok := safeMul(uint64(len(owner.slots)), uint64(len(owner.payloads)))
	if !ok {
		return SealFailureFinishPresentSlots
	}
	present, ok = safeMul(present, containments)
	if !ok {
		return SealFailureFinishPresentContainments
	}
	present, ok = safeMul(present, containments)
	if !ok {
		return SealFailureFinishPresentContainments
	}
	if present == ^uint64(0) {
		return SealFailureFinishPresentContainments
	}
	owner.presentPotential = present
	if !owner.sealWidenRankBounds() {
		return SealFailureFinishWidenRanks
	}
	owner.id = heapContentID(owner.linkOwner)
	if !owner.id.Available() {
		return SealFailureFinishContentID
	}
	owner.bottom = Value{owner: owner.schema}
	owner.top = Value{owner: owner.schema, top: true}
	if !owner.sealBootRows() {
		return SealFailureBootProjection
	}
	if !owner.sealKeyIDInverse() {
		return SealFailureFinishKeyInverse
	}
	if !owner.sealAllocationFormDirectory() {
		return SealFailureFinishAllocationForms
	}
	return SealFailureNone
}

// sealKeyIDInverse proves the complete KeyID relation once, after all root
// rows are admitted and Heap's content identity is known. Every sealed root
// contributes exactly one ID, duplicate IDs reject the seal, and a second
// totality pass verifies that every root is redeemable through the inverse.
// Hot callers therefore perform one map lookup and never scan Heap roots.
func (owner *heapBuilder) sealKeyIDInverse() bool {
	if owner == nil || owner.keyByID != nil || !owner.id.Available() {
		return false
	}
	count := owner.rootCount()
	if count > uint64(^uint32(0)) || count > uint64(^uint(0)>>1) {
		return false
	}
	ownerSchema := Schema{owner: owner.schema}
	inverse := make(map[identity.ContentID]uint32, int(count))
	for index := uint64(0); index < count; index++ {
		slot := uint32(index + 1)
		key := Key{owner: owner.schema, slot: slot}
		id, idOK := ownerSchema.KeyID(key)
		if !idOK || !id.Available() {
			return false
		}
		if _, duplicate := inverse[id]; duplicate {
			return false
		}
		inverse[id] = slot
	}
	if uint64(len(inverse)) != count {
		return false
	}
	for id, slot := range inverse {
		if slot == 0 {
			return false
		}
		key := Key{owner: owner.schema, slot: slot}
		canonical, canonicalOK := ownerSchema.KeyID(key)
		if !canonicalOK || canonical != id || !key.valid() {
			return false
		}
	}
	if !owner.sealFreshSlotDirectory(ownerSchema) {
		return false
	}
	owner.keyByID = inverse
	return true
}

// sealFreshSlotDirectory records the physical fresh slots in strict owner
// KeyID order. It is one owner-issued directory, not another fresh-root
// representation: the catalog remains the source of each slot's semantics.
func (owner *heapBuilder) sealFreshSlotDirectory(ownerSchema Schema) bool {
	if owner == nil || owner.freshSlotsByID != nil {
		return false
	}
	count := owner.freshCount()
	if count > uint64(^uint32(0)) || count > uint64(^uint(0)>>1) {
		return false
	}
	type candidate struct {
		id   identity.ContentID
		slot uint32
	}
	candidates := make([]candidate, 0, int(count))
	for index := uint64(0); index < count; index++ {
		slot := uint64(owner.programRootCount) + index + 1
		if slot > uint64(^uint32(0)) {
			return false
		}
		key := Key{owner: owner.schema, slot: uint32(slot)}
		id, idOK := ownerSchema.KeyID(key)
		if !idOK || !id.Available() {
			return false
		}
		candidates = append(candidates, candidate{id: id, slot: uint32(slot)})
	}
	sort.Slice(candidates, func(left, right int) bool {
		return bytes.Compare(candidates[left].id[:], candidates[right].id[:]) < 0
	})
	owner.freshSlotsByID = make([]uint32, len(candidates))
	for index, candidate := range candidates {
		if index > 0 && candidates[index-1].id == candidate.id {
			return false
		}
		owner.freshSlotsByID[index] = candidate.slot
	}
	return true
}

// sealOccurrenceInverses derives the two exact occurrence indexes from the
// already-complete semantic rows. It deliberately runs once at the end of
// Heap sealing: callers can look up one supplied Program occurrence without
// walking the root or access denominators, while no new source row or identity
// is introduced.
func (owner *heapBuilder) sealOccurrenceInverses() bool {
	if owner == nil || owner.sealProject() == nil || owner.programAllocationOrdinals != nil || owner.indexAccessOrdinals != nil {
		return false
	}
	project := owner.sealProject()
	if project == nil {
		return false
	}
	mounts := project.Mounts()
	allocations := make(map[programAllocationOccurrence]uint32, owner.programRootCount)
	if uint64(owner.programRootCount) > uint64(len(owner.roots)) {
		return false
	}
	for index := uint32(0); index < owner.programRootCount; index++ {
		row := owner.roots[index]
		if row.kind != RootAllocation || !row.allocation.module.Available() ||
			!row.allocation.programID.Available() || !row.allocation.allocationID.Available() ||
			row.allocation.artifactRow == 0 || (row.allocation.kind != AllocationTable && row.allocation.kind != AllocationClosure) {
			return false
		}
		mount, mountOK := owner.artifacts[row.allocation.module]
		if !mountOK || !mount.Available() || mount.ProgramID != row.allocation.programID {
			return false
		}
		program := mount.Snapshot.Program()
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		allocation, allocationOK := heapallocation.AllocationFamily().At(&program.Frozen, catalog, int(row.allocation.artifactRow-1))
		sealed, sealedOK := sealedAllocationForm(allocation.Form())
		if !program.Available() || !catalogOK || !allocationOK || allocation.ID() != row.allocation.allocationID || !sealedOK || sealed != row.allocation.form {
			return false
		}
		occurrence := programAllocationOccurrence{module: row.allocation.module, allocationID: row.allocation.allocationID}
		if allocations[occurrence] != 0 {
			return false
		}
		allocations[occurrence] = index + 1
	}
	for _, row := range owner.fields {
		if row.artifactRow == 0 || row.valuesRow == 0 || row.root == 0 || uint64(row.root) > uint64(len(owner.roots)) {
			return false
		}
		root := owner.roots[row.root-1]
		mount, mountOK := owner.artifacts[root.allocation.module]
		if !mountOK || !mount.Available() || root.kind != RootAllocation || root.allocation.kind != AllocationTable {
			return false
		}
		program := mount.Snapshot.Program()
		catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
		allocation, allocationOK := heapallocation.AllocationFamily().At(&program.Frozen, catalog, int(root.allocation.artifactRow-1))
		fieldOffset, fieldCount, fieldsOK := allocation.FieldSpan()
		fieldIndex := int(row.artifactRow - 1)
		field, fieldOK := heapallocation.Field{}, false
		if catalogOK && fieldsOK && fieldIndex >= 0 && uint64(fieldIndex) < uint64(fieldCount) {
			field, fieldOK = heapallocation.FieldFamily().At(&program.Frozen, catalog, int(fieldOffset)+fieldIndex)
		}
		values, valuesOK := programschema.ValuesFamily().At(&program.Frozen, catalog, int(row.valuesRow-1))
		if !allocationOK || !fieldOK || !valuesOK || allocation.ID() != row.allocationID || field.ID() != row.fieldID || values.ID() != field.ValuesID() {
			return false
		}
	}

	accesses := make(map[indexAccessOccurrence]uint32, len(owner.indexAccesses))
	for index, row := range owner.indexAccesses {
		if !row.module.Available() || !validIndexAccessRow(row, mounts) {
			return false
		}
		occurrence := indexAccessOccurrence{module: row.module, id: row.occurrence}
		if accesses[occurrence] != 0 {
			return false
		}
		accesses[occurrence] = uint32(index + 1)
	}
	owner.programAllocationOrdinals = allocations
	owner.indexAccessOrdinals = accesses
	return true
}

func validIndexAccessRow(row indexAccessRow, mounts linkproject.Mounts) bool {
	if !row.baseValueID.Available() || !row.module.Available() || !row.programID.Available() || !row.occurrence.Available() || row.dynamic && !row.keyValueID.Available() {
		return false
	}
	// Seal-time validation has already joined the row to ProgramArtifact. The
	// retained Heap row is intentionally independent of the mounted Program;
	// only its scalar geometry and mounted artifact identity survive.
	_ = mounts
	return row.isRead && validIndexReadRow(row) || !row.isRead && validIndexWriteRow(row)
}

func validIndexReadRow(row indexAccessRow) bool {
	if row.isRead {
		if !row.resultID.Available() || row.valuesID.Available() || row.position != -1 || row.payload != 0 {
			return false
		}
		return true
	}
	return false
}

func validIndexWriteRow(row indexAccessRow) bool {
	if row.isRead || !row.valuesID.Available() || row.position < 0 || row.payload == 0 {
		return false
	}
	return true
}

// sealWidenRankBounds proves that every fixed-coordinate Object score, and
// therefore every legal Value's at-most-three-object score, fits uint64.
// This is a representation admission proof performed once for the sealed
// finite schema; it is not a runtime convergence limit.  WidenRank may rely
// on these stored witnesses without repeating arithmetic or declining a
// valid Schema later.
func (owner *schema) sealWidenRankBounds() bool {
	if owner == nil || owner.presentPotential == ^uint64(0) || owner.referenceCount == ^uint64(0) {
		return false
	}
	// Every CellState scores at most presentPotential+1.  Object has one
	// residual coordinate for each legal Lua key kind and one fixed semantic
	// coordinate for each sealed exact/reference atom.  Its header score is
	// at most referenceCount+4: one missing shape, one missing frozen state,
	// and the referenceCount+2 exact/none/unknown metatable powerset capacity.
	cellBound, ok := safeAdd(owner.presentPotential, 1)
	if !ok {
		return false
	}
	coordinates, ok := safeAdd(uint64(legalKeyKindCount), uint64(len(owner.atomicKeys)))
	if !ok {
		return false
	}
	partitionBound, ok := safeMul(coordinates, cellBound)
	if !ok {
		return false
	}
	headerBound, ok := safeAdd(owner.referenceCount, 4)
	if !ok {
		return false
	}
	objectBound, ok := safeAdd(partitionBound, headerBound)
	if !ok || objectBound == 0 {
		return false
	}
	maxObjectSum, ok := safeMul(objectBound, 3)
	if !ok || maxObjectSum == 0 {
		return false
	}
	owner.fixedObjectRankBound = objectBound
	owner.maxObjectRankSum = maxObjectSum
	return true
}

// buildRootKinds records the closed, Link/Target-derived Lua-observable kind
// mask for each structural root. RootKind is deliberately insufficient: a
// Program closure is Function, and one fresh root can be Table or Function
// across sealed Candidate rows while retaining one canonical root identity.
func (owner *heapBuilder) buildRootKinds() bool {
	if owner == nil || owner.sealProject() == nil {
		return false
	}
	for index := uint64(0); index < owner.rootCount(); index++ {
		mask, ok := owner.rootRuntimeKinds(uint32(index + 1))
		if !ok || mask == 0 || !mask.Valid() || mask&^runtimekind.NonNil != 0 {
			return false
		}
	}
	return true
}

func (owner *schema) rootRuntimeKinds(root uint32) (runtimekind.Set, bool) {
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootBoot:
		return runtimekind.Bit(runtimekind.Table), true
	case RootAllocation:
		// Virtual fresh results deliberately share the allocation root carrier:
		// they admit Recent/Summary materialization but have no Program
		// allocation proof. Their Target-derived kind set is owned by Catalog.
		if freshRoot, freshOK := owner.freshRoot(root); freshOK {
			return freshRoot.Kinds, freshRoot.Kinds.Valid() && freshRoot.Kinds&^runtimekind.NonNil == 0
		}
		switch row.allocation.kind {
		case AllocationTable:
			return runtimekind.Bit(runtimekind.Table), true
		case AllocationClosure:
			return runtimekind.Bit(runtimekind.Function), true
		default:
			return 0, false
		}
	default:
		return 0, false
	}
}

func (owner *heapBuilder) buildAtomicKeys() bool {
	if owner == nil || owner.sealProject() == nil || owner.atomicKeys != nil {
		return false
	}
	keys := owner.sealProject().Keys()
	atoms := make([]keyAtom, 0, keys.Count()+int(owner.rootCount()))
	for index := 0; index < keys.Count(); index++ {
		key, ok := keys.At(index)
		literal, literalOK := keys.Exact(key)
		if !ok || !literalOK || literal.Kind == 0 {
			return false
		}
		ordinal := owner.exactIndex[literal]
		if ordinal == 0 {
			if uint64(len(owner.exactKeys)) >= uint64(^uint32(0)) {
				return false
			}
			owner.exactKeys = append(owner.exactKeys, exactKeyRow{literal: literal})
			ordinal = uint32(len(owner.exactKeys))
			owner.exactIndex[literal] = ordinal
		}
		atoms = append(atoms, keyAtom{kind: keyAtomExact, exactOrdinal: ordinal})
	}
	for index := uint64(0); index < owner.rootCount(); index++ {
		root, rootOK := owner.rootAt(uint32(index + 1))
		if !rootOK {
			return false
		}
		role := materialization.Invalid
		switch root.kind {
		case RootAllocation:
			role = materialization.Recent
		case RootBoot:
			role = materialization.Exact
		default:
			return false
		}
		atoms = append(atoms, keyAtom{kind: keyAtomReference, root: uint32(index + 1), role: role})
	}
	atoms = normalizeKeyAtoms(atoms)
	if len(atoms) != 0 && !validExactKeyAtoms(owner.schema, atoms) {
		return false
	}
	for _, atom := range atoms {
		mask := keyAtomRuntimeKinds(owner.schema, atom)
		index := int(mask)
		if index == 0 || index >= len(owner.atomMaskCounts) || owner.atomMaskCounts[index] == ^uint64(0) {
			return false
		}
		owner.atomMaskCounts[index]++
	}
	owner.atomicKeys = atoms
	return true
}

func safeMul(left, right uint64) (uint64, bool) {
	if left != 0 && right > ^uint64(0)/left {
		return 0, false
	}
	return left * right, true
}

func safeAdd(left, right uint64) (uint64, bool) {
	if left > ^uint64(0)-right {
		return 0, false
	}
	return left + right, true
}

func (owner *schema) referencePotential() (uint64, bool) {
	if owner == nil {
		return 0, false
	}
	var result uint64
	for index := uint64(0); index < owner.rootCount(); index++ {
		roles, ok := owner.referenceRolePotential(uint32(index + 1))
		if !ok || result > ^uint64(0)-roles {
			return 0, false
		}
		result += roles
	}
	return result, true
}

func (owner *schema) localRolePotential(root uint32) (uint64, bool) {
	if owner == nil || root == 0 {
		return 0, false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootAllocation:
		return 3, true // one/recent and many/{recent,summary}
	case RootBoot:
		return 1, true // one/exact
	default:
		return 0, false
	}
}

func (owner *schema) referenceRolePotential(root uint32) (uint64, bool) {
	if owner == nil || root == 0 {
		return 0, false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return 0, false
	}
	switch row.kind {
	case RootAllocation:
		return 2, true // Recent and Summary only; Exact belongs solely to boot.
	case RootBoot:
		return 1, true
	default:
		return 0, false
	}
}

func (schema Schema) admitsReferenceRole(root uint32, role materialization.Role) bool {
	return schema.owner != nil && schema.owner.admitsReferenceRole(root, role)
}

func (owner *schema) admitsReferenceRole(root uint32, role materialization.Role) bool {
	if owner == nil || root == 0 || !role.Valid() {
		return false
	}
	row, ok := owner.rootAt(root)
	if !ok {
		return false
	}
	switch row.kind {
	case RootAllocation:
		return role == materialization.Recent || role == materialization.Summary
	case RootBoot:
		return role == materialization.Exact
	default:
		return false
	}
}

func heapContentID(owner link.OwnerCapability) (id identity.ContentID) {
	// schemaFormat is the sole Heap schema/content identity discriminator.
	// Keeping one version word means a row-layout or projection-authority
	// change cannot accidentally leave a second, contradictory revision in
	// the identity hash.
	linkID := owner.ContentID()
	var payload [32 + 8]byte
	copy(payload[:32], linkID[:])
	binary.BigEndian.PutUint64(payload[32:], schemaFormat)
	digest := sha256.Sum256(payload[:])
	copy(id[:], digest[:])
	return id
}

func (schema Schema) valid() bool {
	return schema.owner != nil && schema.owner.linkOwner.Available() && schema.owner.id.Available() &&
		(schema.owner.referenceCount != ^uint64(0) && schema.owner.presentPotential != ^uint64(0) &&
			schema.owner.fixedObjectRankBound != 0 && schema.owner.maxObjectRankSum != 0)
}

// Valid reports whether Schema is a fully sealed Heap authority.
func (schema Schema) Valid() bool { return schema.valid() }

// ContentID identifies the cold Heap declaration, never a recurrent Value.
func (schema Schema) ContentID() identity.ContentID {
	if !schema.valid() {
		return identity.ContentID{}
	}
	return schema.owner.id
}

// LinkContentID identifies the exact Link scope that admitted this family.
func (schema Schema) LinkContentID() identity.ContentID {
	if !schema.valid() {
		return identity.ContentID{}
	}
	return schema.owner.linkOwner.ContentID()
}

// LinkOwner returns Heap's exact detached Link owner witness.
func (schema Schema) LinkOwner() link.OwnerCapability {
	if !schema.valid() {
		return link.OwnerCapability{}
	}
	return schema.owner.linkOwner
}

// Rebind reconstructs the complete cold authority from an equivalent Link.
// State Values intentionally do not rebind or serialize.
func (schema Schema) Rebind(source *link.Link) (Schema, bool) {
	if !schema.valid() || source == nil || schema.owner.artifacts == nil {
		return Schema{}, false
	}
	sourceOwner := source.OwnerCapability()
	if !sourceOwner.Available() || sourceOwner.ContentID() != schema.owner.linkOwner.ContentID() {
		return Schema{}, false
	}
	mounts := make([]programmount.MountedArtifact, 0, source.Project().Mounts().Count())
	for index := 0; index < source.Project().Mounts().Count(); index++ {
		shard, shardOK := source.Project().Mounts().At(index)
		module, moduleOK := source.Project().ModuleKey(shard)
		prior, priorOK := schema.owner.artifacts[module]
		programID, programOK := source.Project().Mounts().ProgramID(shard)
		if !shardOK || !moduleOK || !priorOK || !prior.Available() || !programOK || prior.ProgramID != programID {
			return Schema{}, false
		}
		mount, mountOK := programmount.MountedArtifactFromSnapshot(prior.Snapshot, module)
		if !mountOK {
			return Schema{}, false
		}
		mounts = append(mounts, mount)
	}
	rebound, failure := SealWithArtifacts(source, mounts)
	if failure != SealFailureNone || rebound.ContentID() != schema.ContentID() {
		return Schema{}, false
	}
	return rebound, true
}

func (schema Schema) KeyCount() int {
	if !schema.valid() {
		return 0
	}
	count := schema.owner.rootCount()
	if count > uint64(^uint(0)>>1) {
		return 0
	}
	return int(count)
}

// FreshCount returns the exact Target fresh-root denominator owned by this
// Heap schema. It does not scan Keys or reconstruct a Project/Target product.
func (schema Schema) FreshCount() int {
	if !schema.valid() || schema.owner.fresh == nil || schema.owner.freshSlotsByID == nil {
		return 0
	}
	return len(schema.owner.freshSlotsByID)
}

// FreshAt returns the owner-issued Key and its stable KeyID for one fresh
// root in strict owner KeyID order. The catalog owns the Target source row;
// Heap remains the only issuer of the aggregate Key and its content identity.
func (schema Schema) FreshAt(index int) (identity.ContentID, Key, bool) {
	if !schema.valid() || schema.owner.fresh == nil || schema.owner.freshSlotsByID == nil || index < 0 || index >= len(schema.owner.freshSlotsByID) {
		return identity.ContentID{}, Key{}, false
	}
	slot := schema.owner.freshSlotsByID[index]
	if _, freshOK := schema.owner.freshRoot(slot); !freshOK {
		return identity.ContentID{}, Key{}, false
	}
	key := Key{owner: schema.owner, slot: slot}
	if !schema.OwnsKey(key) {
		return identity.ContentID{}, Key{}, false
	}
	id, idOK := schema.KeyID(key)
	return id, key, idOK
}

// KeyAt returns one owner-issued private dense selector for an exact root.
func (schema Schema) KeyAt(index int) (Key, bool) {
	if !schema.valid() || index < 0 || uint64(index) >= schema.owner.rootCount() || uint64(index) >= uint64(^uint32(0)) {
		return Key{}, false
	}
	return Key{owner: schema.owner, slot: uint32(index + 1)}, true
}

// AllocationKeyCount returns the exact dense allocation-root denominator.
// Program allocations and Target fresh-result allocations occupy the leading
// range of Heap's root carrier; Boot roots are deliberately outside it.
func (schema Schema) AllocationKeyCount() int {
	if !schema.valid() {
		return 0
	}
	count := uint64(schema.owner.programRootCount) + schema.owner.freshCount()
	if count > uint64(^uint(0)>>1) {
		return 0
	}
	return int(count)
}

// AllocationKeyAt issues one allocation root from Heap's canonical dense
// allocation range. It does not construct a second allocation directory.
func (schema Schema) AllocationKeyAt(index int) (Key, bool) {
	if !schema.valid() || index < 0 || index >= schema.AllocationKeyCount() || uint64(index) >= uint64(^uint32(0)) {
		return Key{}, false
	}
	key := Key{owner: schema.owner, slot: uint32(index + 1)}
	return key, key.Kind() == RootAllocation
}

// AllocationKeyIndex projects an allocation Key into Heap's canonical dense
// allocation range. Boot roots are rejected rather than admitted as inert
// coordinates in allocation-only factors.
func (schema Schema) AllocationKeyIndex(key Key) (int, bool) {
	if !schema.OwnsKey(key) || key.Kind() != RootAllocation || key.slot == 0 {
		return 0, false
	}
	raw := uint64(key.slot - 1)
	count := schema.AllocationKeyCount()
	if count == 0 || raw >= uint64(count) || raw > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(raw), true
}

// AllocationOriginForKey projects the canonical sealed Program allocation row
// already owned by this Heap schema. It returns scalar row columns directly;
// no boundary transport object or artifact occurrence receipt is minted.
func (schema Schema) AllocationOriginForKey(key Key) (module, programID, allocationID identity.ContentID, kind AllocationKind, form AllocationForm, ok bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootAllocation {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, AllocationInvalid, AllocationFormInvalid, false
	}
	row, rowOK := schema.owner.rootAt(key.slot)
	if !rowOK || row.kind != RootAllocation || !row.allocation.module.Available() || !row.allocation.programID.Available() || !row.allocation.allocationID.Available() || !row.allocation.kind.Valid() || !row.allocation.form.Valid() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, AllocationInvalid, AllocationFormInvalid, false
	}
	return row.allocation.module, row.allocation.programID, row.allocation.allocationID, row.allocation.kind, row.allocation.form, true
}

// AllocationRootValueID returns the sealed mounted semantic identity for one
// allocation root.  Resolution belongs to Value's detached directory; Heap
// never retains or reissues a Boundary Value carrier after sealing.
func (schema Schema) AllocationRootValueID(key Key) (identity.ContentID, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootAllocation {
		return identity.ContentID{}, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok || row.kind != RootAllocation || !row.allocation.allocationID.Available() || !row.allocation.rootValueID.Available() {
		return identity.ContentID{}, false
	}
	return row.allocation.rootValueID, true
}

// AllocationFormForKey returns the constructor form already sealed on one
// existing Program allocation root.  Consumers that only need the source
// disposition must use this row projection rather than rescanning artifact
// fields to classify the root again.
func (schema Schema) AllocationFormForKey(key Key) (AllocationForm, bool) {
	if !schema.valid() || !schema.OwnsKey(key) || key.Kind() != RootAllocation {
		return AllocationFormInvalid, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok || row.kind != RootAllocation || !row.allocation.form.Valid() {
		return AllocationFormInvalid, false
	}
	return row.allocation.form, true
}

// OwnsKey rejects a forged or foreign Heap coordinate without reconstructing
// its Program/Target source.
func (schema Schema) OwnsKey(key Key) bool {
	return schema.valid() && key.valid() && key.owner == schema.owner
}

// KeyIndex projects Heap's already-issued dense carrier coordinate for one
// exact Key. It is an owner-fenced view of Key's sealed selector, not a new
// lookup table or a source reconstruction path.
func (schema Schema) KeyIndex(key Key) (int, bool) {
	if !schema.OwnsKey(key) || key.slot == 0 || uint64(key.slot) > schema.owner.rootCount() {
		return 0, false
	}
	return int(key.slot - 1), true
}

// DenseKeyIndex is the Factor-local form of KeyIndex.
func (schema Schema) DenseKeyIndex(key Key) (uint32, bool) {
	index, ok := schema.KeyIndex(key)
	if !ok || index < 0 || uint64(index) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(index), true
}

// AllocationRootForMountedOccurrence resolves one mounted Program allocation
// occurrence to its Heap allocation Key.
func (schema Schema) AllocationRootForMountedOccurrence(module, occurrence identity.ContentID) (Key, bool) {
	mount, mountOK := schema.OccurrenceMountForModule(module)
	if !mountOK {
		return Key{}, false
	}
	return mount.AllocationRootForOccurrence(occurrence)
}

// AllocationRootCount is the dense census of allocation Keys.
func (schema Schema) AllocationRootCount() int {
	if !schema.valid() {
		return 0
	}
	count := 0
	for index := 0; index < schema.KeyCount(); index++ {
		key, keyOK := schema.KeyAt(index)
		if keyOK && key.Kind() == RootAllocation {
			count++
		}
	}
	return count
}

// AllocationRootAt returns one dense allocation Key in sealed Key order.
func (schema Schema) AllocationRootAt(index int) (Key, bool) {
	if !schema.valid() || index < 0 {
		return Key{}, false
	}
	remaining := index
	for keyIndex := 0; keyIndex < schema.KeyCount(); keyIndex++ {
		key, keyOK := schema.KeyAt(keyIndex)
		if !keyOK || key.Kind() != RootAllocation {
			continue
		}
		if remaining == 0 {
			return key, true
		}
		remaining--
	}
	return Key{}, false
}

// AllocationRootOrdinal is the exact inverse of AllocationRootAt.
func (schema Schema) AllocationRootOrdinal(key Key) (uint32, bool) {
	if !schema.OwnsKey(key) || key.Kind() != RootAllocation {
		return 0, false
	}
	ordinal := uint32(0)
	for keyIndex := 0; keyIndex < schema.KeyCount(); keyIndex++ {
		candidate, keyOK := schema.KeyAt(keyIndex)
		if !keyOK || candidate.Kind() != RootAllocation {
			continue
		}
		if candidate.slot == key.slot {
			return ordinal, true
		}
		ordinal++
	}
	return 0, false
}

// KeyID is Heap's stable identity for an already owner-issued coordinate.
// It is not a second root representation and it never serializes a Link
// allocation handle.
func (schema Schema) KeyID(key Key) (identity.ContentID, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner {
		return identity.ContentID{}, false
	}
	var payload [32 + 2*8]byte
	id := schema.ContentID()
	copy(payload[:32], id[:])
	binary.BigEndian.PutUint64(payload[32:40], 0x686561702d6b6579) // "heap-key"
	binary.BigEndian.PutUint64(payload[40:48], uint64(key.slot))
	return sha256.Sum256(payload[:]), true
}

// KeyForID redeems one Heap KeyID through the sealed owner inverse. The
// returned coordinate is always issued by this exact Schema; unknown,
// unavailable, and foreign IDs receive no fallback or root scan.
func (schema Schema) KeyForID(id identity.ContentID) (Key, bool) {
	if !schema.valid() || !id.Available() || schema.owner.keyByID == nil {
		return Key{}, false
	}
	slot, ok := schema.owner.keyByID[id]
	if !ok || slot == 0 {
		return Key{}, false
	}
	key := Key{owner: schema.owner, slot: slot}
	canonical, canonicalOK := schema.KeyID(key)
	return key, canonicalOK && canonical == id
}

// KeyForBootID admits one existing detached bootstrap semantic ID into the
// same Heap Key family as allocation roots.  Host coordinates are consumed at
// seal time and cannot be passed through this post-seal API.
func (schema Schema) KeyForBootID(rootID identity.ContentID) (Key, bool) {
	if !schema.valid() {
		return Key{}, false
	}
	id := schema.owner.bootIndex[rootID]
	if id == 0 {
		return Key{}, false
	}
	return Key{owner: schema.owner, slot: id}, true
}

// BootCount and BootIDAt enumerate the detached bootstrap-root directory
// sealed into Heap.  They are the only post-seal bootstrap discovery surface.
func (schema Schema) BootCount() int {
	if !schema.valid() {
		return 0
	}
	count := 0
	for _, row := range schema.owner.roots {
		if row.kind == RootBoot && row.bootID.Available() {
			count++
		}
	}
	return count
}

func (schema Schema) BootIDAt(index int) (identity.ContentID, bool) {
	if !schema.valid() || index < 0 {
		return identity.ContentID{}, false
	}
	for _, row := range schema.owner.roots {
		if row.kind != RootBoot || !row.bootID.Available() {
			continue
		}
		if index == 0 {
			return row.bootID, true
		}
		index--
	}
	return identity.ContentID{}, false
}

// BootFrozen projects Target's canonical whole-object boot header through the
// actor-local Link root.  It is deliberately separate from InitialFrozen,
// which constrains only one exact entry.  Callers receive the Heap header
// value rather than Target vocabulary, so target policy cannot leak into
// recurrent Heap transfer APIs.
func (schema Schema) BootFrozen(key Key) (Frozen, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootBoot {
		return FrozenInvalid, false
	}
	row, ok := schema.owner.rootAt(key.slot)
	if !ok {
		return FrozenInvalid, false
	}
	if row.bootImmutable {
		return FrozenFrozen, true
	}
	return FrozenMutable, true
}

// FieldCount and FieldAt enumerate the dense field range sealed for one
// allocation Key. Fresh and Boot roots have no constructor fields.
func (schema Schema) FieldCount(key Key) int {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootAllocation {
		return 0
	}
	root, ok := schema.owner.rootAt(key.slot)
	if !ok || uint64(root.fieldCount) > uint64(^uint(0)>>1) {
		return 0
	}
	return int(root.fieldCount)
}

func (schema Schema) FieldAt(key Key, index int) (Field, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || index < 0 || key.Kind() != RootAllocation {
		return Field{}, false
	}
	root, ok := schema.owner.rootAt(key.slot)
	if !ok || index >= int(root.fieldCount) || root.fieldStart == 0 {
		return Field{}, false
	}
	return Field{owner: schema.owner, index: root.fieldStart + uint32(index)}, true
}

// ArtifactAllocationForKey returns the exact reusable allocation descriptor
// that was mounted to issue key. It performs only semantic-ID lookup against
// Heap's already authenticated artifact mount; it never reopens Program.
func (schema Schema) ArtifactAllocationForKey(key Key) (heapallocation.Allocation, bool) {
	if !schema.OwnsKey(key) || key.Kind() != RootAllocation || schema.owner.artifacts == nil {
		return heapallocation.Allocation{}, false
	}
	module, programID, allocationID, kind, form, originOK := schema.AllocationOriginForKey(key)
	root, rootOK := schema.owner.rootAt(key.slot)
	if !originOK || !rootOK || root.kind != RootAllocation || root.allocation.module != module || root.allocation.programID != programID || root.allocation.allocationID != allocationID || root.allocation.kind != kind {
		return heapallocation.Allocation{}, false
	}
	mount, mountOK := schema.owner.artifacts[module]
	if !mountOK || !mount.Available() {
		return heapallocation.Allocation{}, false
	}
	if mount.ProgramID != programID || mount.Snapshot.ProgramID() != programID {
		return heapallocation.Allocation{}, false
	}
	artifact := mount.Snapshot
	if artifact == nil || root.allocation.artifactRow == 0 {
		return heapallocation.Allocation{}, false
	}
	program := artifact.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	allocation, allocationOK := heapallocation.AllocationFamily().At(&program.Frozen, catalog, int(root.allocation.artifactRow-1))
	sealed, sealedOK := sealedAllocationForm(allocation.Form())
	if !program.Available() || !catalogOK || !allocationOK || allocation.ID() != allocationID || !sealedOK || sealed != form || allocation.Role() == 0 {
		return heapallocation.Allocation{}, false
	}
	return allocation, true
}

// ArtifactFieldFor returns one exact reusable field descriptor by its stable
// allocation-field identity. No dense field ordinal or authored Term crosses
// this boundary.
func (schema Schema) ArtifactFieldFor(field Field) (heapallocation.Field, bool) {
	if !schema.ownsField(field) {
		return heapallocation.Field{}, false
	}
	row := schema.owner.fields[field.index-1]
	root, rootOK := schema.owner.rootAt(row.root)
	if !rootOK || root.kind != RootAllocation || root.allocation.allocationID != row.allocationID {
		return heapallocation.Field{}, false
	}
	allocation, allocationOK := schema.ArtifactAllocationForKey(Key{owner: schema.owner, slot: row.root})
	if !allocationOK || allocation.ID() != row.allocationID {
		return heapallocation.Field{}, false
	}
	if row.artifactRow == 0 {
		return heapallocation.Field{}, false
	}
	mount, mountOK := schema.owner.artifacts[root.allocation.module]
	if !mountOK || !mount.Available() {
		return heapallocation.Field{}, false
	}
	program := mount.Snapshot.Program()
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	offset, count, spanOK := allocation.FieldSpan()
	index := int(row.artifactRow - 1)
	if !catalogOK || !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return heapallocation.Field{}, false
	}
	candidate, candidateOK := heapallocation.FieldFamily().At(&program.Frozen, catalog, int(offset)+index)
	return candidate, candidateOK && candidate.ID() == row.fieldID
}

// ArtifactValuesForField returns the exact ProgramArtifact Values denominator
// named by a sealed field descriptor. The join is the Values semantic ID, not
// an authored Term or a dense artifact position.
func (schema Schema) ArtifactValuesForField(field Field) (programschema.Program, programschema.Values, bool) {
	if !schema.ownsField(field) {
		return programschema.Program{}, programschema.Values{}, false
	}
	fieldRow, fieldOK := schema.ArtifactFieldFor(field)
	physical := schema.owner.fields[field.index-1]
	root, rootOK := schema.owner.rootAt(physical.root)
	if !fieldOK || !rootOK || root.kind != RootAllocation {
		return programschema.Program{}, programschema.Values{}, false
	}
	mount, mountOK := schema.owner.artifacts[root.allocation.module]
	if !mountOK || !mount.Available() || mount.ProgramID != root.allocation.programID {
		return programschema.Program{}, programschema.Values{}, false
	}
	program := mount.Snapshot.Program()
	valuesID := fieldRow.ValuesID()
	if !program.Available() || !valuesID.Available() || physical.valuesRow == 0 {
		return programschema.Program{}, programschema.Values{}, false
	}
	catalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	values, valuesOK := programschema.ValuesFamily().At(&program.Frozen, catalog, int(physical.valuesRow-1))
	return program, values, catalogOK && valuesOK && values.ID() == valuesID
}

func (schema Schema) SlotForField(field Field) (Slot, bool) {
	if !schema.ownsField(field) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.fields[field.index-1].slot)
}

func (schema Schema) SlotForOpenFieldTail(field Field) (Slot, bool) {
	if !schema.ownsField(field) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.fields[field.index-1].openTail)
}

// IndexAccessCount and IndexAccessAt enumerate Heap's direct sealed candidate
// rows. Candidate order is Reads first, followed by Writes, as supplied by
// Flow AccessGeometry.
func (schema Schema) IndexAccessCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.indexAccesses)
}

func (schema Schema) IndexAccessAt(index int) (IndexAccess, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.indexAccesses) {
		return IndexAccess{}, false
	}
	access := IndexAccess{owner: schema.owner, index: uint32(index + 1)}
	return access, schema.ownsIndexAccess(access)
}

// IndexAccessOccurrence returns Heap's exact mounted reusable occurrence key.
// It is copied while Heap already owns the Program geometry census; callers
// receive no raw Term, Program, Flow, or mount ordinal to reconstruct it.
func (schema Schema) IndexAccessOccurrence(access IndexAccess) (module, occurrence identity.ContentID, read bool, ok bool) {
	if !schema.ownsIndexAccess(access) {
		return identity.ContentID{}, identity.ContentID{}, false, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	return row.module, row.occurrence, row.isRead, row.module.Available() && row.occurrence.Available()
}

func (schema Schema) PayloadForField(field Field) (Payload, bool) {
	if !schema.ownsField(field) {
		return Payload{}, false
	}
	return schema.payload(schema.owner.fields[field.index-1].payload)
}

// IndexAccessGeometry returns the direct typed geometry retained by Heap.
// Read rows have position -1 and no Values receipt; write rows carry their
// artifact-issued Values identity and position.
func (schema Schema) IndexAccessGeometry(access IndexAccess) (IndexGeometry, bool) {
	if !schema.ownsIndexAccess(access) {
		return IndexGeometry{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	return IndexGeometry{Module: row.module, ProgramID: row.programID, BaseValueID: row.baseValueID, KeyValueID: row.keyValueID, ValuesID: row.valuesID, Position: row.position, DynamicKey: row.dynamic, Read: row.isRead}, true
}

// SlotForIndexAccess returns the exact slot sealed on the Heap row.
func (schema Schema) SlotForIndexAccess(access IndexAccess) (Slot, bool) {
	if !schema.ownsIndexAccess(access) {
		return Slot{}, false
	}
	return schema.slot(schema.owner.indexAccesses[access.index-1].slot)
}

// PayloadForIndexAccess returns the exact sealed RHS payload. Read rows have
// no payload.
func (schema Schema) PayloadForIndexAccess(access IndexAccess) (Payload, bool) {
	if !schema.ownsIndexAccess(access) {
		return Payload{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if row.isRead || row.payload == 0 {
		return Payload{}, false
	}
	return schema.payload(row.payload)
}

// RawPayloadTagForIndexAccess returns Heap's sealed payload identity for one
// indexed write. The row already owns this relation; callers must not rebuild
// it by rescanning payload sources or coordinates.
func (schema Schema) RawPayloadTagForIndexAccess(access IndexAccess) (RawPayloadTag, bool) {
	if !schema.ownsIndexAccess(access) {
		return 0, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if row.isRead || row.payload == 0 {
		return 0, false
	}
	return RawPayloadTag(row.payload), true
}

// IndexAccessResultID returns the detached Link-owned Boundary Value identity
// for a read result; callers resolve it directly through Value's ID directory.
func (schema Schema) IndexAccessResultID(access IndexAccess) (identity.ContentID, bool) {
	if !schema.ownsIndexAccess(access) {
		return identity.ContentID{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if !row.isRead {
		return identity.ContentID{}, false
	}
	return row.resultID, row.resultID.Available()
}

func (schema Schema) IndexAccessID(access IndexAccess) (identity.ContentID, bool) {
	if !schema.ownsIndexAccess(access) {
		return identity.ContentID{}, false
	}
	row := schema.owner.indexAccesses[access.index-1]
	var payload [32 + 32 + 32 + 1]byte
	id := schema.ContentID()
	copy(payload[:32], id[:])
	copy(payload[32:64], row.module[:])
	copy(payload[64:96], row.occurrence[:])
	if row.isRead {
		payload[96] = 1
	}
	return sha256.Sum256(payload[:]), true
}

// BootEntry is one typed immutable bootstrap entry projection. It separates
// Target's raw-absence row from a present initial-value payload so callers can
// never reconstruct absence as a forged present payload.
type BootEntry struct {
	owner *schema
	root  uint32
	slot  uint32
}

// BootEntryCount is the finite, Link/Target-derived bootstrap raw-slot
// denominator.  It is deliberately exposed only as typed BootEntry values:
// callers never receive the owner-private root/slot pair used to store it.
func (schema Schema) BootEntryCount() int {
	if !schema.valid() {
		return 0
	}
	return len(schema.owner.bootEntryOrder)
}

// BootEntryAt returns one canonical bootstrap raw-slot relation.  The order
// is an implementation traversal only; BootEntry.ID is its stable semantic
// identity and callers must not retain this ordinal.
func (schema Schema) BootEntryAt(index int) (BootEntry, bool) {
	if !schema.valid() || index < 0 || index >= len(schema.owner.bootEntryOrder) {
		return BootEntry{}, false
	}
	pair := schema.owner.bootEntryOrder[index]
	entry := BootEntry{owner: schema.owner, root: pair.root, slot: pair.slot}
	return entry, entry.valid()
}

func (entry BootEntry) valid() bool {
	if entry.owner == nil || entry.root == 0 || entry.slot == 0 || uint64(entry.root) > entry.owner.rootCount() || int(entry.slot) > len(entry.owner.slots) {
		return false
	}
	_, ok := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	return ok
}

// BootEntry projects one exact Target-owned bootstrap row.
func (schema Schema) BootEntry(key Key, slot Slot) (BootEntry, bool) {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || key.Kind() != RootBoot || !slot.valid() || slot.owner != schema.owner {
		return BootEntry{}, false
	}
	entry := BootEntry{owner: schema.owner, root: key.slot, slot: slot.id}
	return entry, entry.valid()
}

// Key returns the exact actor-local bootstrap root selected by this entry.
func (entry BootEntry) Key() (Key, bool) {
	if !entry.valid() {
		return Key{}, false
	}
	return Key{owner: entry.owner, slot: entry.root}, true
}

// Slot returns the exact canonical key partition selected by this entry.
func (entry BootEntry) Slot() (Slot, bool) {
	if !entry.valid() {
		return Slot{}, false
	}
	return Slot{owner: entry.owner, id: entry.slot}, true
}

// ID is the stable identity of this exact Link/Target bootstrap raw-slot
// projection. root and slot stay private carrier selectors; their canonical
// pair is hashed under the sealed Heap schema instead of being exposed as a
// second key plane.
func (entry BootEntry) ID() (identity.ContentID, bool) {
	if !entry.valid() || !entry.owner.id.Available() {
		return identity.ContentID{}, false
	}
	var payload [32 + 4*8]byte
	copy(payload[:32], entry.owner.id[:])
	words := payload[32:]
	binary.BigEndian.PutUint64(words[0:8], 0x686561702d626f6f) // "heap-boo"
	binary.BigEndian.PutUint64(words[8:16], 1)
	binary.BigEndian.PutUint64(words[16:24], uint64(entry.root))
	binary.BigEndian.PutUint64(words[24:32], uint64(entry.slot))
	return sha256.Sum256(payload[:]), true
}

// Projection returns RawAbsent with no Payload for Target InitialValueNil and
// InitialValueAbsent. Target retains those distinct contract identities; Heap
// deliberately projects both to the one Lua raw-slot absence relation. A
// RawPresent entry returns its exact non-nil initial-value Payload.
func (entry BootEntry) Projection() (RawPresence, Payload, bool) {
	if !entry.valid() {
		return RawInvalid, Payload{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw == RawAbsent {
		return RawAbsent, Payload{}, true
	}
	if row.raw != RawPresent || row.payload == 0 {
		return RawInvalid, Payload{}, false
	}
	return RawPresent, Payload{owner: entry.owner, id: row.payload}, true
}

// Mutability returns the immutable Target policy for this entry. It is not a
// current Heap frozen-state conclusion.
func (entry BootEntry) Mutability() (vocabulary.InitialMutability, bool) {
	if !entry.valid() {
		return vocabulary.InitialMutabilityInvalid, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	return row.mutability, row.mutability == vocabulary.InitialMutable || row.mutability == vocabulary.InitialFrozen
}

// Reference makes one exact Link structural-root/role operand available to a
// correlated containment fact. Bootstrap roots have one stable Exact role;
// allocation roots retain Recent/Summary materialization roles.
func (schema Schema) Reference(key Key, role materialization.Role) (Reference, bool) {
	if !schema.valid() || key.owner != schema.owner || !key.valid() || !schema.admitsReferenceRole(key.slot, role) {
		return Reference{}, false
	}
	return Reference{owner: schema.owner, root: key.slot, role: role}, true
}

// ValueChild resolves a Root-valued present entry to its exact actor-local
// BootRoot image. Raw-absent and primitive rows intentionally have no child.
func (entry BootEntry) ValueChild() (Reference, bool) {
	if !entry.valid() {
		return Reference{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw != RawPresent || row.payload == 0 || row.valueChild == 0 {
		return Reference{}, false
	}
	payload := entry.owner.payloads[row.payload-1]
	if payload.kind != payloadInitial || payload.initial != row.initial || uint64(row.valueChild) > entry.owner.rootCount() {
		return Reference{}, false
	}
	return Reference{owner: entry.owner, root: row.valueChild, role: materialization.Exact}, true
}

// ValueContainment classifies the value-edge of one raw-present Target boot
// payload. The Target initial-value kind is the sole authority: roots retain
// their exact actor-local child, callable payloads retain an opaque edge, and
// scalar payloads prove no reference edge. Raw-absent entries have no value
// containment tuple and therefore fail closed here.
func (entry BootEntry) ValueContainment() (Containment, bool) {
	if !entry.valid() {
		return Containment{}, false
	}
	row := entry.owner.bootEntries[rootSlot{root: entry.root, slot: entry.slot}]
	if row.raw != RawPresent || row.payload == 0 || int(row.payload) > len(entry.owner.payloads) {
		return Containment{}, false
	}
	payload := entry.owner.payloads[row.payload-1]
	if payload.kind != payloadInitial || payload.initial != row.initial {
		return Containment{}, false
	}
	schema := Schema{owner: entry.owner}
	switch row.kind {
	case vocabulary.InitialValueRoot:
		child, childOK := entry.ValueChild()
		if !childOK {
			return Containment{}, false
		}
		return schema.ContainmentExact(child)
	case vocabulary.InitialValueOperation, vocabulary.InitialValueDeniedOperation:
		if row.valueChild != 0 {
			return Containment{}, false
		}
		return schema.ContainmentUnknown()
	case vocabulary.InitialValueBoolean, vocabulary.InitialValueInteger, vocabulary.InitialValueFloat, vocabulary.InitialValueString:
		if row.valueChild != 0 {
			return Containment{}, false
		}
		return schema.ContainmentNone()
	default:
		return Containment{}, false
	}
}

// Admits is Heap's sole coordinate-specific carrier fence. The outer Value
// remains homogeneous across the Factor; this fence proves that each complete
// World is meaningful at the selected structural root and that every present
// tuple retains its Link/Target provenance.
func (schema Schema) Admits(key Key, value Value) bool {
	if !schema.valid() || !key.valid() || key.owner != schema.owner || !schema.owns(value) {
		return false
	}
	// owns has just proved this Value, so top and bottom are read off it
	// rather than re-proved: a valid Value with no World is bottom.
	if value.top || len(value.worlds) == 0 {
		return true
	}
	// schema.owns has proved this Value: every World it carries is valid and
	// owner-bound, and so is every Object and Partition inside them. The
	// coordinate fence below reads that proof instead of re-deriving it at
	// each World, Object, kind, exception, and present.
	for _, world := range value.worlds {
		if !schema.admitsWorldAdmitted(key, world) {
			return false
		}
	}
	return true
}

// admitsWorldAdmitted is the admitted path for one World of a Value its
// caller has already proved.
func (schema Schema) admitsWorldAdmitted(key Key, world World) bool {
	switch key.Kind() {
	case RootAllocation:
		switch world.kind {
		case WorldZero:
			return true
		case WorldOne:
			return schema.admitsObjectAdmitted(key, world.recent)
		case WorldMany:
			return schema.admitsObjectAdmitted(key, world.recent) && schema.admitsObjectAdmitted(key, world.summary)
		default:
			return false
		}
	case RootBoot:
		return world.kind == WorldExact && schema.admitsObjectAdmitted(key, world.exact)
	default:
		return false
	}
}

// admitsObjectAdmitted is the admitted path for one Object of a Value its
// caller has already proved. The Object's validity carries its Partition's,
// so the kind passes, the exception passes, and the slot incidence below all
// read that one proof.
func (schema Schema) admitsObjectAdmitted(key Key, object Object) bool {
	for index := 0; index < legalKeyKindCount; index++ {
		kind, _ := legalKeyKindAt(index)
		if !schema.admitsPartitionStateAdmitted(key, object.partition, kind, nil, object.partition.rest[kind]) {
			return false
		}
	}
	for _, exception := range object.partition.exceptions {
		if !schema.admitsPartitionStateAdmitted(key, object.partition, runtimekind.Invalid, &exception.atom, exception.state) {
			return false
		}
	}
	return true
}

// admitsPartitionStateAdmitted is the admitted path for one coordinate of a
// Partition its caller has already proved complete and owner-bound.
func (schema Schema) admitsPartitionStateAdmitted(key Key, partition Partition, kind runtimekind.Kind, atom *keyAtom, state CellState) bool {
	if !state.valid() || state.owner != schema.owner || atom == nil && !legalKeyKind(kind) ||
		atom != nil && !validExactKeyAtom(schema.owner, *atom) {
		return false
	}
	for _, present := range state.presents {
		if !schema.admitsSlot(key.slot, present.slotID) || !schema.partitionAdmitsSlotAdmitted(partition, kind, atom, present.slotID) ||
			!schema.admitsPayload(key.slot, present.slotID, present.payloadID) || !schema.admitsInitialPresent(key.slot, present) {
			return false
		}
	}
	return true
}

// partitionAdmitsSlotAdmitted verifies only cold source-to-semantic-key
// incidence, on a Partition its caller has already proved complete and
// owner-bound. A dynamic source proves no equality and may inhabit any
// complete partition coordinate. An exact source must inhabit its exact
// exception or a residual coordinate that still contains that atom; source
// occurrences never become a partition identity.
func (schema Schema) partitionAdmitsSlotAdmitted(partition Partition, kind runtimekind.Kind, atom *keyAtom, slot uint32) bool {
	if schema.owner == nil || slot == 0 || int(slot) > len(schema.owner.slots) {
		return false
	}
	row := schema.owner.slots[slot-1]
	if row.kind != SlotExact {
		return true
	}
	exact := keyAtom{kind: keyAtomExact, exactOrdinal: row.exact}
	if !validExactKeyAtom(schema.owner, exact) {
		return false
	}
	if atom != nil {
		return compareKeyAtom(*atom, exact) == 0
	}
	if !legalKeyKind(kind) || !keyAtomRuntimeKinds(schema.owner, exact).Contains(kind) {
		return false
	}
	_, excluded := partition.exceptionIndex(exact)
	return !excluded
}

// admitsInitialPresent preserves Target's immutable bootstrap row law. A
// Root-valued entry has one exact actor-local child; primitives have none.
// Coarsening a semantic cover never drops the tuple's original source slot.
func (schema Schema) admitsInitialPresent(root uint32, present Present) bool {
	if schema.owner == nil || present.payloadID == 0 || int(present.payloadID) > len(schema.owner.payloads) {
		return false
	}
	payload := schema.owner.payloads[present.payloadID-1]
	if payload.kind != payloadInitial {
		return true
	}
	entry, found := schema.owner.bootEntries[rootSlot{root: root, slot: present.slotID}]
	if !found || entry.raw != RawPresent || entry.payload != present.payloadID || entry.initial != payload.initial {
		return false
	}
	expected, expectedOK := (BootEntry{owner: schema.owner, root: root, slot: present.slotID}).ValueContainment()
	return expectedOK && present.valueContainment == expected && present.keyContainment.kind == ContainmentNone
}

func (schema Schema) admitsSlot(root, slot uint32) bool {
	return schema.owner != nil && schema.owner.admitsSlot(root, slot)
}

func (owner *schema) admitsSlot(root, slot uint32) bool {
	if owner == nil || root == 0 || slot == 0 || uint64(root) > owner.rootCount() || uint64(slot) > uint64(len(owner.slotSupport)) {
		return false
	}
	if owner.slotSupport[slot-1].global {
		return true
	}
	_, ok := owner.localSlots[rootSlot{root: root, slot: slot}]
	return ok
}

func (schema Schema) admitsPayload(root, slot, payload uint32) bool {
	return schema.owner != nil && schema.owner.admitsPayload(root, slot, payload)
}

func (owner *schema) admitsPayload(root, slot, payload uint32) bool {
	if owner == nil || root == 0 || slot == 0 || payload == 0 || uint64(payload) > uint64(len(owner.payloadSupport)) {
		return false
	}
	support := owner.payloadSupport[payload-1]
	if _, ok := support.globalSlots[slot]; ok {
		return true
	}
	_, ok := support.local[rootSlot{root: root, slot: slot}]
	return ok
}

// admitsUnknownPayload preserves source provenance after canonical slot
// coarsening. Assignment payloads apply at every root; constructor payloads
// remain confined to the roots at which they were structurally introduced.
func (schema Schema) admitsUnknownPayload(root, payload uint32) bool {
	return schema.owner != nil && schema.owner.admitsUnknownPayload(root, payload)
}

func (owner *schema) admitsUnknownPayload(root, payload uint32) bool {
	if owner == nil || root == 0 || payload == 0 || uint64(root) > owner.rootCount() || int(payload) > len(owner.payloadSupport) {
		return false
	}
	support := owner.payloadSupport[payload-1]
	if len(support.globalSlots) != 0 {
		return true
	}
	_, ok := support.roots[root]
	return ok
}

func (schema Schema) slot(id uint32) (Slot, bool) {
	if id == 0 || int(id) > len(schema.owner.slots) {
		return Slot{}, false
	}
	return Slot{owner: schema.owner, id: id}, true
}

func (schema Schema) payload(id uint32) (Payload, bool) {
	if id == 0 || int(id) > len(schema.owner.payloads) {
		return Payload{}, false
	}
	return Payload{owner: schema.owner, id: id}, true
}

func (schema Schema) ownsField(field Field) bool {
	if !schema.valid() || field.owner != schema.owner || field.index == 0 || int(field.index) > len(schema.owner.fields) {
		return false
	}
	row := schema.owner.fields[field.index-1]
	if row.root == 0 || uint64(row.root) > schema.owner.rootCount() || row.slot == 0 || row.payload == 0 || !row.allocationID.Available() || !row.fieldID.Available() || row.artifactRow == 0 || row.valuesRow == 0 {
		return false
	}
	root, ok := schema.owner.rootAt(row.root)
	if !ok || root.kind != RootAllocation || root.allocation.kind != AllocationTable || root.fieldStart == 0 || field.index < root.fieldStart || uint64(field.index-root.fieldStart) >= uint64(root.fieldCount) {
		return false
	}
	return uint64(row.slot) <= uint64(len(schema.owner.slots)) && uint64(row.payload) <= uint64(len(schema.owner.payloads)) && row.allocationID == root.allocation.allocationID
}

func (schema Schema) ownsIndexAccess(access IndexAccess) bool {
	if !schema.valid() || access.owner != schema.owner || access.index == 0 || int(access.index) > len(schema.owner.indexAccesses) {
		return false
	}
	row := schema.owner.indexAccesses[access.index-1]
	if !row.module.Available() || !row.programID.Available() || !row.baseValueID.Available() || row.dynamic && !row.keyValueID.Available() || row.slot == 0 || uint64(row.slot) > uint64(len(schema.owner.slots)) {
		return false
	}
	if row.isRead {
		return row.resultID.Available() && !row.valuesID.Available() && row.position == -1 && row.payload == 0
	}
	return row.valuesID.Available() && row.position >= 0 && row.payload != 0 && uint64(row.payload) <= uint64(len(schema.owner.payloads))
}
