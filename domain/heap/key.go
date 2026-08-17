// Package heap owns the one Link-scoped abstract heap relation.
//
// Its Factor key is one exact Link structural aggregate root: either an
// allocation root or an actor-local bootstrap root. Recent/Summary, slots,
// payloads, containment, shape, and frozen state are correlated alternatives
// inside Value; none is a second Factor key or a caller-minted numeric
// identity.
package heap

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// RootKind names one existing structural root family admitted by Heap.  The
// allocation family is sealed directly from Program and Target; only Boot
// retains Link deployment topology.
type RootKind uint8

const (
	RootInvalid RootKind = iota
	RootAllocation
	RootBoot
)

func (kind RootKind) Valid() bool { return kind == RootAllocation || kind == RootBoot }

// Key is Heap's sole owner-issued aggregate coordinate. slot is only a private
// dense physical selector for the generic Factor carrier; callers can observe
// a Program allocation, Target fresh result, or retained Boot root but cannot
// mint or combine source identities.
type Key struct {
	owner *schema
	slot  uint32
}

func (key Key) valid() bool {
	_, ok := key.owner.rootAt(key.slot)
	return ok
}

// Valid reports whether Key was issued by a sealed Heap schema.
func (key Key) Valid() bool { return key.valid() }

// ContentID returns the stable owner-scoped identity of this exact coordinate.
func (key Key) ContentID() (identity.ContentID, bool) {
	if !key.valid() {
		return identity.ContentID{}, false
	}
	return Schema{owner: key.owner}.KeyID(key)
}

// Kind returns the sealed root family selected by this key.
func (key Key) Kind() RootKind {
	if !key.valid() {
		return RootInvalid
	}
	row, ok := key.owner.rootAt(key.slot)
	if !ok {
		return RootInvalid
	}
	return row.kind
}

// AllocationReceipt is Heap's pointer-free mounted allocation identity. The
// module key is the concrete mount receipt, so duplicate mounts of one
// Program remain distinct while allocation identity stays reusable.
type AllocationReceipt struct {
	module       identity.ContentID
	programID    identity.ContentID
	allocationID identity.ContentID
	kind         AllocationKind
	form         flow.AllocationForm
	artifact     *programartifact.Artifact
}

func (kind AllocationKind) Valid() bool { return kind == AllocationTable || kind == AllocationClosure }
func (receipt AllocationReceipt) Available() bool {
	return receipt.module.Available() && receipt.programID.Available() && receipt.allocationID.Available() && receipt.kind.Valid() && receipt.form.Valid() && receipt.artifact != nil && receipt.artifact.Available() && receipt.artifact.CompileKey().ProgramID() == receipt.programID
}
func (receipt AllocationReceipt) Module() identity.ContentID       { return receipt.module }
func (receipt AllocationReceipt) ProgramID() identity.ContentID    { return receipt.programID }
func (receipt AllocationReceipt) AllocationID() identity.ContentID { return receipt.allocationID }
func (receipt AllocationReceipt) Kind() AllocationKind {
	if !receipt.Available() {
		return AllocationInvalid
	}
	return receipt.kind
}
func (receipt AllocationReceipt) Form() flow.AllocationForm {
	if !receipt.Available() {
		return flow.AllocationFormInvalid
	}
	return receipt.form
}

// AllocationReceipt issues an opaque owner receipt for an artifact allocation
// occurrence. The artifact occurrence catalog is the sole Program fact plane.
func (mount ArtifactMount) AllocationReceipt(id identity.ContentID, kind AllocationKind, form flow.AllocationForm) (AllocationReceipt, bool) {
	if !mount.Available() || !id.Available() || !kind.Valid() || !form.Valid() {
		return AllocationReceipt{}, false
	}
	row, ok := mount.artifact.OccurrenceForID(programartifact.OccurrenceAllocation, id)
	if !ok || row.ID() != id || row.Code() != uint64(form) {
		return AllocationReceipt{}, false
	}
	receipt := AllocationReceipt{module: mount.module, programID: mount.programID, allocationID: id, kind: kind, form: form, artifact: mount.artifact}
	return receipt, receipt.Available()
}

// AllocationReceipt returns the compact identity for one Heap allocation key.
func (key Key) AllocationReceipt() (AllocationReceipt, bool) {
	if !key.valid() || key.Kind() != RootAllocation {
		return AllocationReceipt{}, false
	}
	row, ok := key.owner.rootAt(key.slot)
	if !ok || row.kind != RootAllocation || !row.allocation.module.Available() {
		return AllocationReceipt{}, false
	}
	mount, mounted := key.owner.artifacts[row.allocation.module]
	if !mounted {
		return AllocationReceipt{}, false
	}
	receipt := AllocationReceipt{module: row.allocation.module, programID: row.allocation.programID, allocationID: row.allocation.allocationID, kind: row.allocation.kind, form: row.allocation.form, artifact: mount.artifact}
	return receipt, receipt.Available()
}

// FreshResult returns the exact creation occurrence selected by this Key.
// Program aggregates return false. Its target Call coordinate remains opaque:
// Heap never reissues or exposes the raw Program Term.
func (key Key) FreshResultID() (identity.ContentID, int, uint32, uint32, bool) {
	if !key.valid() || key.Kind() != RootAllocation {
		return identity.ContentID{}, 0, 0, 0, false
	}
	root, ok := key.owner.rootAt(key.slot)
	if !ok {
		return identity.ContentID{}, 0, 0, 0, false
	}
	row := root.fresh
	if !row.applicationID.Available() || !row.kinds.Valid() {
		return identity.ContentID{}, 0, 0, 0, false
	}
	return row.applicationID, int(row.outcome), row.result, row.ordinal, true
}

// BootID returns the detached actor-local bootstrap root identity selected by
// this key.  Heap never reissues Host's authority-bearing BootRoot.
func (key Key) BootID() (identity.ContentID, bool) {
	if !key.valid() || key.Kind() != RootBoot {
		return identity.ContentID{}, false
	}
	row, ok := key.owner.rootAt(key.slot)
	if !ok {
		return identity.ContentID{}, false
	}
	return row.bootID, row.bootID.Available()
}

// Slot is an owner-issued cold structural storage provenance. Exact Lua keys
// are shared within the Link-wide arena; dynamic source Values remain only
// topology provenance and never become a Heap equality or partition identity.
// Dynamic source is topology provenance only; its Rule uses a typed kind
// selector, never a string key, raw sentinel, or source-occurrence identity.
type Slot struct {
	owner *schema
	id    uint32
}

// ExactKey is Heap's owner-fenced exact literal coordinate.  It is issued
// only from the sealed Heap exact-key universe; it cannot retain or replay a
// Project Key authority.
type ExactKey struct {
	owner   *schema
	ordinal uint32
}

func (key ExactKey) valid() bool {
	return key.owner != nil && key.ordinal != 0 && int(key.ordinal) <= len(key.owner.exactKeys)
}

// Literal returns the already-normalized scalar exact key.
func (key ExactKey) Literal() (keyspace.LiteralValue, bool) {
	if !key.valid() {
		return keyspace.LiteralValue{}, false
	}
	literal := key.owner.exactKeys[key.ordinal-1].literal
	return literal, literal.Kind != 0
}

func (slot Slot) valid() bool {
	return slot.owner != nil && slot.id != 0 && int(slot.id) <= len(slot.owner.slots)
}

// SlotKind describes the structural source of one symbolic key partition.
type SlotKind uint8

const (
	SlotInvalid SlotKind = iota
	SlotExact
	SlotDynamic
	SlotOpenTail
	SlotUnknown
)

// KeySelectorKind names an operation input, never stored Heap state.  A Rule
// selects either one atomic key, a finite set of atoms, or a runtime-kind
// family.  The Heap carrier stores one complete residual partition instead of
// storing a caller-provided "unknown" cell alongside exact cells.
//
// Numeric owns interval/range reasoning. Heap deliberately has no numeric
// range payload until Numeric issues one typed descriptor: copying numeric
// equality or interval syntax into Heap would create a second authority.
type KeySelectorKind uint8

const (
	KeySelectorInvalid KeySelectorKind = iota
	KeySelectorAtom
	KeySelectorFinite
	KeySelectorKinds
)

type keyAtomKind uint8

const (
	keyAtomInvalid keyAtomKind = iota
	keyAtomExact
	keyAtomReference
)

// keyAtom is intentionally private.  It retains only Link- or
// Schema-issued identities, never decoded strings, copied literals, or a
// caller-minted equality token.
type keyAtom struct {
	kind         keyAtomKind
	exactOrdinal uint32
	root         uint32
	role         materialization.Role
}

// KeySelector is an immutable normalized selection operand. Dynamic source
// occurrences are deliberately not members: an occurrence can produce
// different keys on different visits. Rules instead use an exact Link key, an
// exact Reference, a future Numeric-issued descriptor, or a kind selector.
//
// `kinds` and `atoms` are mutually exclusive. A kind selector has no
// caller-provided exclusions: exclusions belong solely to stored Partition
// exceptions and are derived from them.
type KeySelector struct {
	owner *schema
	kinds runtimekind.Set
	atoms []keyAtom
}

func (selector KeySelector) valid() bool {
	if selector.owner == nil {
		return false
	}
	if selector.kinds != 0 {
		return len(selector.atoms) == 0 && selector.kinds.Valid() && selector.kinds&^runtimekind.NonNil == 0
	}
	return len(selector.atoms) != 0 && validExactKeyAtoms(selector.owner, selector.atoms)
}

// Valid reports whether this is an owner-issued Heap key selector.
func (selector KeySelector) Valid() bool { return selector.valid() }

// Kind reports the selector's operation precision without exposing storage.
func (selector KeySelector) Kind() KeySelectorKind {
	if !selector.valid() {
		return KeySelectorInvalid
	}
	if selector.kinds != 0 {
		return KeySelectorKinds
	}
	if len(selector.atoms) == 1 {
		return KeySelectorAtom
	}
	return KeySelectorFinite
}

// ExactCount/ExactAt enumerate normalized exact Lua keys in this selector.
// Link has already quotiented 1 and 1.0 and rejected nil/NaN, so Heap does
// not repeat either rule.
func (selector KeySelector) ExactCount() int {
	if !selector.valid() || selector.kinds != 0 {
		return 0
	}
	count := 0
	for _, atom := range selector.atoms {
		if atom.kind == keyAtomExact {
			count++
		}
	}
	return count
}

func (selector KeySelector) ExactAt(index int) (ExactKey, bool) {
	if !selector.valid() || selector.kinds != 0 || index < 0 {
		return ExactKey{}, false
	}
	for _, atom := range selector.atoms {
		if atom.kind != keyAtomExact {
			continue
		}
		if index == 0 {
			return ExactKey{owner: selector.owner, ordinal: atom.exactOrdinal}, true
		}
		index--
	}
	return ExactKey{}, false
}

// ReferenceCount/ReferenceAt enumerate exact object-identity key subjects.
// They are distinct from containment references only in role: here they
// classify a table key, while CellState containment classifies an edge.
func (selector KeySelector) ReferenceCount() int {
	if !selector.valid() || selector.kinds != 0 {
		return 0
	}
	count := 0
	for _, atom := range selector.atoms {
		if atom.kind == keyAtomReference {
			count++
		}
	}
	return count
}

func (selector KeySelector) ReferenceAt(index int) (Reference, bool) {
	if !selector.valid() || selector.kinds != 0 || index < 0 {
		return Reference{}, false
	}
	for _, atom := range selector.atoms {
		if atom.kind != keyAtomReference {
			continue
		}
		if index == 0 {
			return Reference{owner: selector.owner, root: atom.root, role: atom.role}, true
		}
		index--
	}
	return Reference{}, false
}

// RuntimeKinds reports the complete lawful kind set of this selector. Exact
// selectors derive their kind from sealed Link/Heap identity; a kind selector
// returns its canonical runtime-kind mask.
func (selector KeySelector) RuntimeKinds() runtimekind.Set {
	if !selector.valid() {
		return 0
	}
	if selector.kinds != 0 {
		return selector.kinds
	}
	var kinds runtimekind.Set
	for _, atom := range selector.atoms {
		kinds |= keyAtomRuntimeKinds(selector.owner, atom)
	}
	return kinds
}

// Origin exposes the one existing source coordinate of a partition. Dynamic
// slots retain the globally unique opaque Boundary Value identity; exact
// slots retain the Link canonical key arena identity. Heap never exposes a
// raw authored Term or a redundant mount qualifier.
func (slot Slot) Origin() (kind SlotKind, exact ExactKey, value identity.ContentID, ok bool) {
	if !slot.valid() {
		return SlotInvalid, ExactKey{}, identity.ContentID{}, false
	}
	row := slot.owner.slots[slot.id-1]
	if row.kind == SlotExact {
		return row.kind, ExactKey{owner: slot.owner, ordinal: row.exact}, identity.ContentID{}, row.exact != 0
	}
	return row.kind, ExactKey{}, row.dynamic, row.dynamic.Available()
}

// Payload is one exact Values-pack scalar-selection source. It names neither
// an inferred runtime value nor a Heap object; Value/identity owns the former
// and Heap relates the sealed source selection to a present tuple.
type Payload struct {
	owner *schema
	id    uint32
}

func (payload Payload) valid() bool {
	return payload.owner != nil && payload.id != 0 && int(payload.id) <= len(payload.owner.payloads)
}

// Source returns the mounted artifact Values identity and its scalar
// adjustment. The ID is a Program-issued semantic receipt, not a raw Term.
func (payload Payload) Source() (identity.ContentID, identity.ContentID, int, bool) {
	if !payload.valid() {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	row := payload.owner.payloads[payload.id-1]
	if row.kind != payloadValues {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.module, row.valuesID, int(row.index), row.module.Available() && row.valuesID.Available()
}

// InitialValue returns the sealed Target bootstrap source for a boot payload.
// Program Values payloads return false; their source remains available through
// Source. Neither projection is a recurrent runtime value.
func (payload Payload) InitialValue() (target.InitialValue, bool) {
	if !payload.valid() {
		return 0, false
	}
	row := payload.owner.payloads[payload.id-1]
	return row.initial, row.kind == payloadInitial && row.initial != 0
}

// Reference is one exact structural Heap root together with the canonical
// nominal materialization role. It is a Heap relation operand, not an alias
// Factor or a concrete runtime object ID.
type Reference struct {
	owner *schema
	root  uint32
	role  materialization.Role
}

func (reference Reference) valid() bool {
	_, ok := reference.owner.rootAt(reference.root)
	return ok && reference.role.Valid()
}

// Kind reports the sealed family of the referenced structural root.
func (reference Reference) Kind() RootKind {
	if !reference.valid() {
		return RootInvalid
	}
	row, ok := reference.owner.rootAt(reference.root)
	if !ok {
		return RootInvalid
	}
	return row.kind
}

// Key projects the exact owning Heap coordinate and materialization role.
func (reference Reference) Key() (Key, materialization.Role, bool) {
	if !reference.valid() {
		return Key{}, materialization.Invalid, false
	}
	return Key{owner: reference.owner, slot: reference.root}, reference.role, true
}

// BootID projects a detached bootstrap-family reference.
func (reference Reference) BootID() (identity.ContentID, materialization.Role, bool) {
	if !reference.valid() || reference.Kind() != RootBoot {
		return identity.ContentID{}, materialization.Invalid, false
	}
	row, ok := reference.owner.rootAt(reference.root)
	if !ok {
		return identity.ContentID{}, materialization.Invalid, false
	}
	return row.bootID, reference.role, row.bootID.Available()
}

// ContainmentKind distinguishes a proved absence from exact and opaque
// containment edges. The zero kind is deliberately invalid.
type ContainmentKind uint8

const (
	ContainmentInvalid ContainmentKind = iota
	ContainmentNone
	ContainmentExact
	ContainmentUnknown
)

// Containment is one owner-fenced child-edge fact carried by a raw-present
// tuple. None proves that no reference edge exists, Exact names one reference,
// and Unknown preserves an edge whose target is not tracked structurally.
type Containment struct {
	owner *schema
	kind  ContainmentKind
	root  uint32
	role  materialization.Role
}

func (containment Containment) valid() bool {
	if containment.owner == nil {
		return false
	}
	switch containment.kind {
	case ContainmentNone, ContainmentUnknown:
		return containment.root == 0 && containment.role == materialization.Invalid
	case ContainmentExact:
		return containment.root != 0 && containment.owner.admitsReferenceRole(containment.root, containment.role)
	default:
		return false
	}
}

// Valid reports whether containment is a canonical, owner-issued fact.
func (containment Containment) Valid() bool { return containment.valid() }

// Kind reports the containment fact family.
func (containment Containment) Kind() ContainmentKind {
	if !containment.valid() {
		return ContainmentInvalid
	}
	return containment.kind
}

// Reference projects an Exact containment edge. None and Unknown do not make
// an exact-reference claim.
func (containment Containment) Reference() (Reference, bool) {
	if !containment.valid() || containment.kind != ContainmentExact {
		return Reference{}, false
	}
	return Reference{owner: containment.owner, root: containment.root, role: containment.role}, true
}

// ContainmentNone issues a proof that the tuple has no reference edge.
func (schema Schema) ContainmentNone() (Containment, bool) {
	if !schema.valid() {
		return Containment{}, false
	}
	return Containment{owner: schema.owner, kind: ContainmentNone}, true
}

// ContainmentUnknown issues an opaque reference edge that must not be treated
// as absence or projected as an exact structural reference.
func (schema Schema) ContainmentUnknown() (Containment, bool) {
	if !schema.valid() {
		return Containment{}, false
	}
	return Containment{owner: schema.owner, kind: ContainmentUnknown}, true
}

// ContainmentExact issues an exact edge for an owner-matched reference.
func (schema Schema) ContainmentExact(reference Reference) (Containment, bool) {
	if !schema.valid() || !reference.valid() || reference.owner != schema.owner {
		return Containment{}, false
	}
	return Containment{owner: schema.owner, kind: ContainmentExact, root: reference.root, role: reference.role}, true
}

// MetatableRoute is one typed existing bootstrap primitive-base to metatable
// route. It is a Heap relation operand; it neither selects dispatch nor stores
// mutable attachment state.
type MetatableRoute struct {
	owner *schema
	id    uint32
}

func (route MetatableRoute) valid() bool {
	return route.owner != nil && route.id != 0 && int(route.id) <= len(route.owner.metatableRoutes)
}

// PrimitiveBase returns the Target primitive family of an immutable bootstrap
// route.
func (route MetatableRoute) PrimitiveBase() (target.InitialValueKind, bool) {
	if !route.valid() {
		return target.InitialValueInvalid, false
	}
	row := route.owner.metatableRoutes[route.id-1]
	if row.primitive == target.InitialValueInvalid {
		return target.InitialValueInvalid, false
	}
	return row.primitive, true
}

// Metatable returns the exact contained metatable root reference. Bootstrap
// rows always use Exact; mutable root attachments retain the caller-declared
// nominal role in their Heap relation instead.
func (route MetatableRoute) Metatable() (Reference, bool) {
	if !route.valid() {
		return Reference{}, false
	}
	row := route.owner.metatableRoutes[route.id-1]
	if row.metatable == 0 {
		return Reference{}, false
	}
	return Reference{owner: route.owner, root: row.metatable, role: row.role}, true
}

// MutationLicence is an opaque proof operand. A nonzero licence can only be
// issued by the Heap owner after the exact root, selected Recent, direct/raw
// route, uniqueness, must-existing, and no-unmodelled-mutation obligations
// have all been discharged. The carrier never infers any of those from an
// allocation count. Until an owning Rule provides such an issuer, every write
// receives the zero licence and is weak by construction.
type MutationLicence struct {
	owner *schema
	root  uint32
	role  materialization.Role
	key   KeySelector
	bits  uint8
}

const (
	mutationMustExist uint8 = 1 << iota
	mutationUnique
	mutationNoUnmodelled
	mutationDirectRaw
)

const mutationComplete = mutationMustExist | mutationUnique | mutationNoUnmodelled | mutationDirectRaw

func (licence MutationLicence) validForObject(owner *schema, root uint32) bool {
	return owner != nil && licence.owner == owner && licence.root == root && licence.role == materialization.Recent && licence.bits == mutationComplete
}

func (licence MutationLicence) validForCell(owner *schema, root uint32, key KeySelector) bool {
	return licence.validForObject(owner, root) && key.valid() && licence.key.valid() &&
		equalKeySelector(licence.key, key) && key.exactSelection()
}
