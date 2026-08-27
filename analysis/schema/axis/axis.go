// Package axis owns the axis surface of the analyzer declaration table: the
// record one axis is declared as, the typed contexts its hooks receive, the
// algebra an axis publishes, and the surface laws the declaration root seals
// it under.
//
// An axis is one coordinate space the solver holds facts over. Naming the
// space is not enough to register it: an entry also carries the algebra that
// space is ordered by, the storage it lives in, the cardinality of its key
// space, its dependency edges, its lifetime, mutability, and concurrency
// discipline, and the columns it publishes for a consumer to read. The engine
// binds the executable algebra; this surface is where the axis is declared,
// sealed, and derived from.
//
// An axis is a writer principal. The entry identity is the principal identity
// and the declaration position is the principal slot, so a lane that writes
// facts is admitted by declaring an axis rather than by adding a member to a
// foreign enum this surface would then have to name.
//
// This surface owns the analyzer's one storage vocabulary. The published value
// a consumer holds names storage by address alone -- a sealing schema identity
// and a dense column slot -- so where facts live, how their key space is
// shaped, and the discipline they are written under are declared here once and
// projected there, never spelled twice.
//
// The surface is blind to every domain. The Link authority record an axis
// binds against is a type parameter supplied by the composition, and an axis's
// own cold fragment, hot axis, and fact carrier are type parameters of its
// declaration, so an authored hook is fully typed and never asserts. Erasure
// exists only in the private cell this package uses to hold declarations of
// different axes in one table.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package axis

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = seal.SurfaceLawFloor + iota
	LawAxisIdentity
	// The ordinal here is retired. An axis is a writer principal, so the
	// principal is declared by declaring the axis and there is no second field
	// to state as present.
	_
	// The ordinal here is retired. One axis is one principal by construction,
	// and two rows carrying one identity is the root's own law, stated by
	// seal.LawEntryUnique over the entry identity this surface derives.
	_
	LawFieldComplete
	LawMetadataComplete
	LawDependencyResolves
	LawSemanticIdentity
	LawSemanticUnique
	// The ordinal here is retired. The bind phase is the root's law: the
	// declaration catalog order is the bind phase order, and the root rejects
	// out-of-order registration under seal.LawSurfacePhase.
	_
	// The ordinal here is retired. Whether the semantic role vocabulary is
	// itself complete is the structural surface's own law, stated over the
	// category the roles are declared in. A role this surface names and that
	// vocabulary does not declare is an unresolved reference, which is stated
	// under LawSemanticIdentity.
	_
	// LawDependencyAcyclic states that the declared edges admit an order. A
	// cycle names no first axis, so a phase that walks the edges could not
	// begin; the table is rejected at seal rather than at the walk.
	LawDependencyAcyclic
	// LawOutputDeclared states that a declared output names both the column it
	// publishes and the principal admitted to write it.
	LawOutputDeclared
	// LawOutputUnique states that one published column has one writer. Two
	// declarations of one output name would leave a consumer reading a column
	// without knowing whose rows it holds.
	LawOutputUnique
	// LawOutputWriterResolves states that an output's writer is a declared
	// axis, so a column cannot be admitted for a principal this table does not
	// know.
	LawOutputWriterResolves
	// LawMemberShape states that every nested member is an admitted member
	// declaration. Members are nested under their owning axis rather than
	// registered as a second SurfaceKindAxis contribution, so this surface is
	// the sole place that owns their shape.
	LawMemberShape
	// LawMemberIdentity states that a member's stable identity is issued by its
	// owner axis and cannot be laundered from a different owner or key.
	LawMemberIdentity
	// LawMemberKind states that a member names one of the finite relation,
	// projection, or reducer vocabularies.
	LawMemberKind
	// LawMemberOwner states that a nested member belongs to the axis row that
	// publishes it.
	LawMemberOwner
	// LawMemberUnique states that an owner cannot publish one member identity
	// twice, even when the declarations arrive through separate contributors.
	LawMemberUnique
	// LawMemberSignature states that a member's declared signature is complete;
	// executable functions and runtime handles are deliberately outside this
	// package.
	LawMemberSignature
	// LawMemberCatalog states that a supplied catalog must be complete. An empty
	// catalog remains the explicit absence value.
	LawMemberCatalog
	// LawCarrierSignature states that the key resolves to a local or imported
	// authority with key capability, while fact resolves to this axis's local
	// ascending authority.
	LawCarrierSignature
	// LawCarrierReference states that an imported carrier points at one issued
	// authority in its target owner's catalog.
	LawCarrierReference
)

// Storage is the closed catalog of places an axis's facts live. It is the
// analyzer's one storage vocabulary: the published value names storage by
// address alone, so every classification of where facts live and under what
// discipline is declared here and consumed there.
type Storage uint8

const (
	StorageInvalid Storage = iota
	// StorageFactor is a Link-bound engine factor cell, written by the rules of
	// the lane this axis is the principal of.
	StorageFactor
	// StorageStatic is an inventory carried by the compiled program and never
	// written during solve.
	StorageStatic
	// StorageEngine is a column the engine publishes itself. No factor cell
	// holds it and no rule writes it, so the axis declares a coordinate space
	// and a writer principal and carries no hot half at all: execution
	// reachability is one, published by the epoch pass that derives it.
	StorageEngine
)

func (storage Storage) Available() bool {
	return storage >= StorageFactor && storage <= StorageEngine
}

// Bound reports whether an axis of this storage instantiates a factor binding.
// A bound axis declares a cold fragment, a hot binding, and the algebra of that
// binding; an engine-published axis declares none of the three, because the
// pass that fills its column is not a factor lane.
func (storage Storage) Bound() bool {
	return storage == StorageFactor || storage == StorageStatic
}

// Cardinality is the shape of an axis's key space. A dense axis numbers its
// coordinates over a contiguous ordinal range; a sparse axis materializes only
// the coordinates that occur.
type Cardinality uint8

const (
	CardinalityInvalid Cardinality = iota
	CardinalityDense
	CardinalitySparse
)

func (cardinality Cardinality) Available() bool {
	return cardinality == CardinalityDense || cardinality == CardinalitySparse
}

// Lifetime is the scope an axis's facts are valid for.
type Lifetime uint8

const (
	LifetimeInvalid Lifetime = iota
	// LifetimeLink is one Link binding: the axis is bound with the shared
	// engine binding and dies with it.
	LifetimeLink
	// LifetimeProgram is one compiled program: the axis is materialized with
	// the artifact and outlives any single binding.
	LifetimeProgram
	// LifetimeProcess is the sealed process-global declaration itself.
	LifetimeProcess
)

func (lifetime Lifetime) Available() bool {
	return lifetime >= LifetimeLink && lifetime <= LifetimeProcess
}

// Mutability is whether an axis's facts change after publication.
type Mutability uint8

const (
	MutabilityInvalid Mutability = iota
	// MutabilitySolve admits monotone rule writes until the fixpoint closes.
	MutabilitySolve
	// MutabilityFrozen admits no write after the axis is published.
	MutabilityFrozen
)

func (mutability Mutability) Available() bool {
	return mutability == MutabilitySolve || mutability == MutabilityFrozen
}

// Concurrency is the sharing discipline of an axis's storage.
type Concurrency uint8

const (
	ConcurrencyInvalid Concurrency = iota
	// ConcurrencySingleWriter is written by one solver at a time and is not
	// shared with a concurrent reader.
	ConcurrencySingleWriter
	// ConcurrencyShared is immutable once published and safe for concurrent
	// readers.
	ConcurrencyShared
)

func (concurrency Concurrency) Available() bool {
	return concurrency == ConcurrencySingleWriter || concurrency == ConcurrencyShared
}

// Rank is one widening or narrowing measure of an axis, projected onto the
// axis's ordinal key space. Width is the number of components; a rank with
// Width zero is declared absent.
type Rank[V any] struct {
	Width int
	At    func(key uint64, value V, component int) uint64
}

func (rank Rank[V]) Available() bool { return rank.Width > 0 && rank.At != nil }

// Absent reports the one admissible undeclared shape.
func (rank Rank[V]) Absent() bool { return rank.Width == 0 && rank.At == nil }

// Algebra is the axis's published fact algebra: the lattice its facts are
// ordered by, the default fact, the end of its key space, the coordinate
// admission predicate, the fact fingerprint, and the widening and narrowing
// ranks. Keys are the axis's ordinal key space; the carrier coordinate a
// binding instantiates the engine with stays inside the axis's own owner.
//
// The engine remains the authority on whether an algebra is admissible for a
// factor binding. This record is the declaration surface's view of the same
// algebra, so a derived inventory reads it without asking a domain.
type Algebra[V any] struct {
	KeyEnd      uint64
	Lattice     lattice.Lattice[V]
	Default     V
	AdmitAt     func(key uint64, value V) bool
	Fingerprint func(value V) uint64
	Widen       Rank[V]
	Narrow      Rank[V]
}

func (algebra Algebra[V]) Available() bool {
	if algebra.AdmitAt == nil || algebra.Fingerprint == nil {
		return false
	}
	if algebra.Lattice.Bottom == nil || algebra.Lattice.Top == nil || algebra.Lattice.Equal == nil ||
		algebra.Lattice.LessOrEq == nil || algebra.Lattice.Join == nil || algebra.Lattice.Widen == nil {
		return false
	}
	return algebra.Widen.Available() && (algebra.Narrow.Available() || algebra.Narrow.Absent())
}

// CarrierRank is one widening or narrowing measure on the owner's carrier
// coordinate. Adopt projects it onto the axis ordinal key space.
type CarrierRank[K ~uint32 | ~uint64, V any] struct {
	Width int
	At    func(key K, value V, component int) uint64
}

// CarrierAlgebra is the owner-typed factor algebra Adopt projects onto
// Algebra. It is declared here so the axis surface never names an engine type.
type CarrierAlgebra[K ~uint32 | ~uint64, V any] struct {
	KeyEnd      uint64
	Lattice     lattice.Lattice[V]
	Default     V
	AdmitAt     func(key K, value V) bool
	Fingerprint func(value V) uint64
	Widen       CarrierRank[K, V]
	Narrow      CarrierRank[K, V]
}

// Adopt projects one carrier-typed factor algebra onto the axis surface.
func Adopt[K ~uint32 | ~uint64, V any](spec CarrierAlgebra[K, V]) (Algebra[V], bool) {
	algebra := Algebra[V]{
		KeyEnd:      spec.KeyEnd,
		Lattice:     spec.Lattice,
		Default:     spec.Default,
		Fingerprint: spec.Fingerprint,
		Widen:       adoptRank(spec.Widen, spec.KeyEnd),
		Narrow:      adoptRank(spec.Narrow, spec.KeyEnd),
	}
	if spec.AdmitAt != nil {
		admit := spec.AdmitAt
		keyEnd := spec.KeyEnd
		algebra.AdmitAt = func(key uint64, value V) bool {
			carrier, ok := carrierKey[K](key, keyEnd)
			return ok && admit(carrier, value)
		}
	}
	return algebra, algebra.Available()
}

func adoptRank[K ~uint32 | ~uint64, V any](measure CarrierRank[K, V], keyEnd uint64) Rank[V] {
	if measure.At == nil {
		return Rank[V]{Width: measure.Width}
	}
	at := measure.At
	return Rank[V]{Width: measure.Width, At: func(key uint64, value V, component int) uint64 {
		carrier, ok := carrierKey[K](key, keyEnd)
		if !ok {
			return 0
		}
		return at(carrier, value, component)
	}}
}

// carrierKey admits one ordinal key into the carrier coordinate type. A key
// outside the declared key space, or one the carrier cannot represent, is
// rejected rather than truncated.
func carrierKey[K ~uint32 | ~uint64](key, keyEnd uint64) (K, bool) {
	if key >= keyEnd {
		return 0, false
	}
	carrier := K(key)
	if uint64(carrier) != key {
		return 0, false
	}
	return carrier, true
}

// Declaration is the cold context an axis's Declare hook receives. An axis is
// the producer of a factor principal, so it declares against the schema
// builder and the semantic roles it declared alone.
//
// Roles resolves exactly the roles this axis declared: its own identity and
// the additional roles named on its Spec. A hook that reaches for a role the
// axis never declared resolves nothing, so an identity an axis consumes is an
// identity it is on record as consuming.
type Declaration struct {
	Roles vocabulary.Roles
}

// Mounting is the Link context an axis's Mount hook receives. Inputs is the
// composition's own Link input record, which carries the neutral sealed
// artifact view and every peer authority already mounted in this phase. The
// mount phase runs after the cold declaration and before any binding, so no
// engine binding is reachable here.
type Mounting[A any] struct{ Inputs A }

// MountEntry is one axis's typed mount declaration, erased in the authority it
// seals and in its own rejection evidence but still typed in the composition's
// Link input record. The zero value is an axis that declares no mount: its
// authority is supplied to the composition by some other owner.
type MountEntry[A any] struct {
	run func(A) (Cell, Cell, bool)
}

func (entry MountEntry[A]) Available() bool { return entry.run != nil }

// NewMount admits one authored mount hook. M is the Link authority this axis
// seals from the mounted artifacts; R is the domain's own rejection evidence,
// which travels back to the composition erased and is recovered at the domain's
// type by Payload. A successful mount returns the domain's zero rejection.
func NewMount[A, M, R any](hook func(Mounting[A]) (M, R, bool)) MountEntry[A] {
	if hook == nil {
		return MountEntry[A]{}
	}
	return MountEntry[A]{run: func(inputs A) (Cell, Cell, bool) {
		authority, rejection, ok := hook(Mounting[A]{Inputs: inputs})
		if !ok {
			return Cell{}, Cell{value: rejection}, false
		}
		return Cell{value: authority}, Cell{}, true
	}}
}

// Binding is the hot binding context. Fragment is the cold fragment this
// axis's Declare hook produced; Inputs is the composition's own Link input
// record. Axes bind before rules, so no rule authority is reachable here.
type Binding[A, F any] struct {
	Fragment F
	Inputs   A
}

// Signature is the bounded cold carrier signature of an axis. Key and Fact
// are nominal member carriers, so executable or domain-owned handles cannot
// enter the declaration stream by structural coincidence.
type Signature struct {
	Key  carrier.Key
	Fact carrier.Key
}

func (signature Signature) Available() bool {
	return signature.Key.Available() && signature.Fact.Available()
}

// Spec is the authored declaration of one axis. A is the composition's Link
// input record. The owning domain keeps its algebra, owner, and contributor;
// what it hands over here is the sealed declaration.
type Spec[A any] struct {
	// Key is the axis's authored identity and its diagnostic name, so an axis
	// has exactly one spelling in the analyzer. It derives the entry identity
	// a verdict carries.
	Key schema.Key
	// The writer principal is not a field. An axis is the principal that writes
	// it, so the entry identity this key derives is the principal identity and
	// the declaration position is the principal slot.
	Storage     Storage
	Cardinality Cardinality
	Lifetime    Lifetime
	Mutability  Mutability
	Concurrency Concurrency
	// Dependencies are the axes this axis's declaration depends on, by their
	// authored keys. An edge must resolve to a declared axis and may not be a
	// self-edge.
	Dependencies []schema.Key
	// Frame is this axis's published half: the columns its facts are read out
	// of and the principal admitted to write each of them.
	Frame Frame
	// Catalog is the owner-issued declaration catalog for this axis. Its zero
	// value is the explicit absence value; once members are supplied, the
	// catalog must be complete.
	Catalog member.Catalog
	// Signature is the bounded cold carrier signature shared by this axis's
	// members. A zero signature is admitted only while Catalog is empty.
	Signature Signature
	// Semantic is this axis's canonical identity: the semantic role row it is
	// declared under. The row's declared spelling derives the identity the
	// engine binds this axis's factor with, so the coordinate space and the
	// identity it is bound under are one declaration.
	Semantic schema.Key
	// Roles are the further semantic roles this axis's Declare hook resolves.
	// They are content, so the identity set an axis is declared against is part
	// of the table digest, and the hook reaches no identity this list omits.
	Roles []schema.Key
	// Mount seals this axis's own Link authority from the neutral mounted
	// artifact view. It is optional: an axis that declares no mount has its
	// authority supplied to the composition by some other owner.
	Mount MountEntry[A]
}

// Cell is the opaque per-axis payload the table carries between passes. It is
// produced and consumed only by the typed thunks one Spec instantiated, so an
// authored hook never sees it and never asserts.
type Cell struct {
	value any
}

func (cell Cell) Available() bool { return cell.value != nil }

// Payload recovers one cell's value at its declared type. It is how an axis's
// own owner reaches its fragment or bound axis; the table itself never needs
// it.
func Payload[T any](cell Cell) (T, bool) {
	value, ok := cell.value.(T)
	return value, ok
}

// Template is one admitted axis declaration, erased in its fragment, hot axis,
// and fact carrier but still typed in the composition's Link input record. It
// is immutable once built.
type Template[A any] struct {
	key schema.Key
	id  schema.EntryID

	storage      Storage
	cardinality  Cardinality
	lifetime     Lifetime
	mutability   Mutability
	concurrency  Concurrency
	dependencies []schema.Key
	outputs      []Output
	catalog      member.Catalog
	signature    Signature

	semantic schema.Key
	roles    []schema.Key
	mount    MountEntry[A]
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable template. Contributor wiring is composition-owned.
func New[A any](spec Spec[A]) (*Template[A], bool) {
	if !specAdmissible(spec) {
		return nil, false
	}
	owner := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: spec.Key}
	catalog, catalogOK := spec.Catalog.Issue(owner)
	if !catalogOK {
		return nil, false
	}
	if spec.Signature != (Signature{}) && !signatureResolves(catalog, owner, spec.Signature) {
		return nil, false
	}
	outputs, outputsOK := issueOutputs(spec.Frame.Outputs, owner, catalog)
	if !outputsOK {
		return nil, false
	}
	template := &Template[A]{
		key:          spec.Key,
		id:           schema.NewEntryID(schema.SurfaceKindAxis, spec.Key),
		storage:      spec.Storage,
		cardinality:  spec.Cardinality,
		lifetime:     spec.Lifetime,
		mutability:   spec.Mutability,
		concurrency:  spec.Concurrency,
		dependencies: append([]schema.Key(nil), spec.Dependencies...),
		outputs:      outputs,
		catalog:      catalog,
		signature:    spec.Signature,
		semantic:     spec.Semantic,
		roles:        append([]schema.Key(nil), spec.Roles...),
		mount:        spec.Mount,
	}
	return template, template.EntryAvailable() && template.metadataComplete() && template.fieldsComplete()
}

func signatureResolves(catalog member.Catalog, owner schema.EntryReference, signature Signature) bool {
	if !signature.Available() {
		return false
	}
	keyBinding, keyResolved := catalog.ResolveCarrier(owner, signature.Key)
	if !keyResolved {
		return false
	}
	// The fact carrier is the axis's own ascending algebra. A foreign fact
	// import would let an axis publish under an authority it does not own.
	factAuthority, factOK := catalog.ResolveLocalCarrier(owner, signature.Fact)
	factIssued, factIssuedOK := carrier.Issue(owner, carrier.Authority{Carrier: signature.Fact, Capability: factAuthority.Capability})
	if !factOK || !factAuthority.ID().Available() || !factIssuedOK || factAuthority.ID() != factIssued.ID() || factAuthority.Capability != carrier.Ascending {
		return false
	}
	if keyBinding.Ref.Owner == owner {
		keyAuthority, local := catalog.ResolveLocalCarrier(owner, signature.Key)
		keyIssued, keyIssuedOK := carrier.Issue(owner, carrier.Authority{Carrier: signature.Key, Capability: keyAuthority.Capability})
		if !local || !keyAuthority.ID().Available() || !keyIssuedOK || keyAuthority.ID() != keyIssued.ID() ||
			(keyAuthority.Capability != carrier.Equatable && keyAuthority.Capability != carrier.Ascending) {
			return false
		}
	}
	return true
}

// issueOutputs performs the same one-way owner issuance for frame columns as
// member.Catalog.Issue performs for nested members. Keeping output identities
// on the row lets the axis seal detect a column/member collision without a
// side registry or a second name-derived identity path.
func issueOutputs(outputs []Output, owner schema.EntryReference, catalog member.Catalog) ([]Output, bool) {
	issued := append([]Output(nil), outputs...)
	for index := range issued {
		if issued[index].ID().Available() {
			return nil, false
		}
		if catalogHasMemberKey(catalog, issued[index].Key) {
			return nil, false
		}
		id := member.IssueID(owner, issued[index].Key)
		if !id.Available() {
			return nil, false
		}
		for prior := 0; prior < index; prior++ {
			if issued[prior].ID() == id {
				return nil, false
			}
		}
		issued[index].id = id
	}
	return issued, true
}

func catalogHasMemberKey(catalog member.Catalog, key schema.Key) bool {
	if _, ok := catalog.Relation(key); ok {
		return true
	}
	if _, ok := catalog.Projection(key); ok {
		return true
	}
	if _, ok := catalog.Reducer(key); ok {
		return true
	}
	if _, ok := catalog.CarryTransform(key); ok {
		return true
	}
	if _, ok := catalog.Selection(key); ok {
		return true
	}
	return false
}

func specAdmissible[A any](spec Spec[A]) bool {
	if !spec.Key.Available() {
		return false
	}
	if !spec.Storage.Available() || !spec.Cardinality.Available() || !spec.Lifetime.Available() ||
		!spec.Mutability.Available() || !spec.Concurrency.Available() {
		return false
	}
	if !spec.Semantic.Available() {
		return false
	}
	for _, role := range spec.Roles {
		if !role.Available() || role == spec.Semantic {
			return false
		}
	}
	for _, dependency := range spec.Dependencies {
		if !dependency.Available() || dependency == spec.Key {
			return false
		}
	}
	for _, output := range spec.Frame.Outputs {
		if !output.Available() {
			return false
		}
	}
	if spec.Signature != (Signature{}) && !spec.Signature.Available() {
		return false
	}
	if spec.Catalog.HasMembers() && (!spec.Catalog.Complete() || !spec.Signature.Available()) {
		return false
	}
	return true
}

func (template *Template[A]) Key() schema.Key { return template.key }

func (template *Template[A]) ID() schema.EntryID { return template.id }

func (template *Template[A]) Storage() Storage { return template.storage }

func (template *Template[A]) Cardinality() Cardinality { return template.cardinality }

func (template *Template[A]) Lifetime() Lifetime { return template.lifetime }

func (template *Template[A]) Mutability() Mutability { return template.mutability }

func (template *Template[A]) Concurrency() Concurrency { return template.concurrency }

// Signature returns this axis's bounded cold carrier signature.
func (template *Template[A]) Signature() Signature {
	if template == nil {
		return Signature{}
	}
	return template.signature
}

func (template *Template[A]) DependencyCount() int { return len(template.dependencies) }

// OutputCount is the number of published columns this axis declares.
func (template *Template[A]) OutputCount() int {
	if template == nil {
		return 0
	}
	return len(template.outputs)
}

// OutputAt returns one declared output at its declaration position. The
// position is the order the composition assigns column slots in.
func (template *Template[A]) OutputAt(index int) (Output, bool) {
	if template == nil || index < 0 || index >= len(template.outputs) {
		return Output{}, false
	}
	return template.outputs[index], true
}

// HasMembers reports whether this axis contains any member or carrier
// declaration. A zero catalog is the explicit absence value.
func (template *Template[A]) HasMembers() bool {
	return template != nil && template.catalog.HasMembers()
}

// MemberCount is the number of nested declarations in catalog order: all
// relations, followed by projections, followed by reducers.
func (template *Template[A]) MemberCount() int {
	if template == nil {
		return 0
	}
	return template.catalog.MemberCount()
}

// Catalog returns an independent copy of the owner-issued declaration catalog.
func (template *Template[A]) Catalog() member.Catalog {
	if template == nil {
		return member.Catalog{}
	}
	return template.catalog.Clone()
}

// CarrierAuthority exposes the one narrow cross-surface lookup the axis can
// provide to a consumer validating an imported carrier.  The catalog remains
// the owner of the authority and returns a value copy, never mutable storage.
func (template *Template[A]) CarrierAuthority(key carrier.Key) (carrier.Authority, bool) {
	if template == nil {
		return carrier.Authority{}, false
	}
	return template.catalog.CarrierAuthority(key)
}

// RelationOrdinal resolves a relation's dense catalog ordinal.
func (template *Template[A]) RelationOrdinal(key schema.Key) (uint32, bool) {
	if template == nil {
		return 0, false
	}
	return template.catalog.RelationOrdinal(key)
}

// ProjectionOrdinal resolves a projection's dense catalog ordinal.
func (template *Template[A]) ProjectionOrdinal(key schema.Key) (uint32, bool) {
	if template == nil {
		return 0, false
	}
	return template.catalog.ProjectionOrdinal(key)
}

// ReducerOrdinal resolves a reducer's dense catalog ordinal.
func (template *Template[A]) ReducerOrdinal(key schema.Key) (uint32, bool) {
	if template == nil {
		return 0, false
	}
	return template.catalog.ReducerOrdinal(key)
}

// Coverage is what a published column of this axis concludes about a key it
// holds no row for. It is derived from the declared cardinality, so a publisher
// reads it here rather than deciding the question again.
func (template *Template[A]) Coverage() Coverage {
	if template == nil {
		return CoverageInvalid
	}
	return CoverageFor(template.cardinality)
}

func (template *Template[A]) DependencyAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(template.dependencies) {
		return "", false
	}
	return template.dependencies[index], true
}

// Semantic is the semantic role row this axis is declared under. A consumer
// resolves the identity through the sealed vocabulary rather than deriving it
// from this key, so the declaration and the derivation stay one step apart.
func (template *Template[A]) Semantic() schema.Key {
	if template == nil {
		return ""
	}
	return template.semantic
}

// RoleCount is the number of further semantic roles this axis declared.
func (template *Template[A]) RoleCount() int {
	if template == nil {
		return 0
	}
	return len(template.roles)
}

// RoleAt returns one further declared semantic role at its declaration
// position.
func (template *Template[A]) RoleAt(index int) (schema.Key, bool) {
	if template == nil || index < 0 || index >= len(template.roles) {
		return "", false
	}
	return template.roles[index], true
}

// declaredRoles is the whole role set this axis is declared against: its own
// identity first, then the roles its hook resolves.
func (template *Template[A]) declaredRoles() []schema.Key {
	if template == nil {
		return nil
	}
	return append([]schema.Key{template.semantic}, template.roles...)
}

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the axis it identifies is completely declared is the
// surface's own law, stated by Seal, so an incomplete axis is reported as the
// incomplete axis it is rather than as an unidentifiable row.
func (template *Template[A]) EntryAvailable() bool {
	return template != nil && template.key.Available() && template.id.Available()
}

// EntryContent writes this axis's declarative half: the semantic roles it is
// declared against, the storage its facts live in, the shape of its key space,
// and its lifetime, mutability, and concurrency disciplines, followed by its
// dependency edges and then its published outputs, each in declaration order. A
// derived inventory reads a coordinate space from exactly these, so an axis that
// changes one of them is a different space and the table digest says so.
//
// The writer principal is not written. The axis is the principal, so the entry
// identity the root already folds is that principal's identity, and writing it
// again here would write one value twice.
//
// The frame is content: which columns an axis publishes and which principal is
// admitted to write each of them is what a consumer addresses and what a write
// capability is minted against, so a catalog that publishes a different column
// set, or admits a different writer for one column, is a different catalog. The
// column slot a publication assigns is not written, because it is the output's
// position in this very order.
//
// The declared roles are content: the role this axis is identified by, and the
// further roles its hook resolves. The engine binds this axis's factor under
// the first and the hook consumes the rest, so two catalogs whose axes name
// different roles declare different coordinate spaces and are declared against
// different identity sets. The rows are written by the key they are declared
// under rather than by the identity that key resolves to, because the
// resolution is the vocabulary surface's derivation from its own declared
// spelling and is already folded there.
//
// The typed hooks are not content. A hook is a function value with no canonical
// bytes, and the algebra one publishes is produced at bind against a live
// binding rather than declared here, so neither is written. The mount hook is a
// hook by the same reading: which owner seals an axis's Link authority is
// wiring, and the coordinate space that authority is a binding of is already
// stated by the identity and metadata above. What the hooks are declared
// against is covered by those and by the surface's own admission laws.
func (template *Template[A]) EntryContent(content *framing.Writer) error {
	if err := content.String(string(template.semantic)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(template.roles))); err != nil {
		return err
	}
	for _, role := range template.roles {
		if err := content.String(string(role)); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(template.storage)); err != nil {
		return err
	}
	if err := content.Uint(uint64(template.cardinality)); err != nil {
		return err
	}
	if err := content.Uint(uint64(template.lifetime)); err != nil {
		return err
	}
	if err := content.Uint(uint64(template.mutability)); err != nil {
		return err
	}
	if err := content.Uint(uint64(template.concurrency)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(template.dependencies))); err != nil {
		return err
	}
	for _, dependency := range template.dependencies {
		if err := content.String(string(dependency)); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(template.outputs))); err != nil {
		return err
	}
	for _, output := range template.outputs {
		if err := content.String(string(output.Key)); err != nil {
			return err
		}
		outputID := identity.ContentID(output.ID())
		if err := content.Bytes(outputID[:]); err != nil {
			return err
		}
		if err := content.String(string(output.Writer)); err != nil {
			return err
		}
	}
	if !template.catalog.HasMembers() {
		return nil
	}
	// The member signature remains a migration suffix. Frame output identities
	// are part of the base stream because outputs share the owner namespace;
	// only an axis that actually declares members moves to the signature and
	// catalog suffix.
	if err := content.String(string(template.signature.Key)); err != nil {
		return err
	}
	if err := content.String(string(template.signature.Fact)); err != nil {
		return err
	}
	return template.catalog.WriteContent(content)
}

// References exposes the ordered cross-surface declarations carried by
// reducer inputs. The member key is local to its owner and is not converted
// into a second root EntryReference; the containing axis is the root entry
// that owns this snapshot.
func (template *Template[A]) References() schema.EntryReferences {
	if template == nil {
		return nil
	}
	return template.catalog.References()
}

func (template *Template[A]) metadataComplete() bool {
	return template.storage.Available() && template.cardinality.Available() &&
		template.lifetime.Available() && template.mutability.Available() && template.concurrency.Available()
}

// fieldsComplete states the hook set the declared storage requires. A bound
// axis instantiates a factor binding and declares the whole cold and hot half;
// an engine-published axis declares none of it, because the pass that fills its
// column is not a factor lane and there is nothing here to instantiate.
func (template *Template[A]) fieldsComplete() bool {
	return template.semantic.Available()
}

// DeclaredRoles is the whole role set this axis is declared against: its own
// identity first, then the roles its contributor resolves.
func (template *Template[A]) DeclaredRoles() []schema.Key {
	return template.declaredRoles()
}

// NewCell holds one typed contributor payload in an opaque cell.
func NewCell(value any) Cell {
	if value == nil {
		return Cell{}
	}
	return Cell{value: value}
}

// NewBoundCell holds one bound axis.
func NewBoundCell(value any) Cell {
	if value == nil {
		return Cell{}
	}
	return Cell{value: value}
}

// MountDeclared reports whether this axis seals its own Link authority.
func (template *Template[A]) MountDeclared() bool {
	return template != nil && template.mount.Available()
}

// Mount seals this axis's Link authority from the composition's input record.
// It returns the sealed authority cell, and on rejection the domain's own
// erased evidence. An axis that declares no mount returns an absent cell and
// admits: mounting nothing is not a failure to mount.
func (template *Template[A]) Mount(inputs A) (Cell, Cell, bool) {
	if template == nil {
		return Cell{}, Cell{}, false
	}
	if !template.mount.Available() {
		return Cell{}, Cell{}, true
	}
	authority, rejection, ok := template.mount.run(inputs)
	if !ok || !authority.Available() {
		return Cell{}, rejection, false
	}
	return authority, Cell{}, true
}

// surface is the axis contribution to the analyzer declaration root.
type surface[A any] struct{ templates []*Template[A] }

// NewSurface hands one ordered set of axis declarations to the table.
func NewSurface[A any](templates []*Template[A]) seal.Surface {
	return surface[A]{templates: templates}
}

func (contribution surface[A]) Kind() schema.SurfaceKind { return schema.SurfaceKindAxis }

func (contribution surface[A]) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.templates))
	for index, template := range contribution.templates {
		entries[index] = template
	}
	return entries
}

// Seal states the axis surface's own laws over the indexed view. The
// structural vocabulary is sealed below this surface, so the semantic roles an
// axis names are resolved against it here.
func (contribution surface[A]) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
	keys := make(map[schema.Key]schema.EntryID, view.Count())
	semantics := make(map[schema.Key]schema.EntryID, view.Count())
	templates := make([]*Template[A], 0, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		template, templateOK := entry.(*Template[A])
		if !entryOK || !templateOK || template == nil {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		templates = append(templates, template)
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !template.key.Available() || template.id != schema.NewEntryID(schema.SurfaceKindAxis, template.key) {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawAxisIdentity, schema.DispositionMalformed)
		}
		id := template.id
		keys[template.key] = id
		if !template.metadataComplete() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawMetadataComplete, schema.DispositionIncomplete)
		}
		if !template.fieldsComplete() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawFieldComplete, schema.DispositionIncomplete)
		}
		// An empty catalog is the explicit absence value. Once an axis publishes
		// members, the bounded catalog must be complete and closed.
		if template.signature != (Signature{}) && !template.signature.Available() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawMemberSignature, schema.DispositionIncomplete)
		}
		if template.catalog.HasMembers() && !template.signature.Available() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawMemberSignature, schema.DispositionIncomplete)
		}
		if template.catalog.HasMembers() && !template.catalog.Complete() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawMemberCatalog, schema.DispositionMalformed)
		}
		if template.catalog.HasMembers() {
			owner := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: template.key}
			if !template.catalog.Issued(owner) {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawMemberIdentity, schema.DispositionMalformed)
			}
			if template.signature != (Signature{}) && !signatureResolves(template.catalog, owner, template.signature) {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawCarrierSignature, schema.DispositionIncomplete)
			}
		}
		// Every role an axis names is a declared member of the semantic role
		// vocabulary. The vocabulary raises the two ways the name fails - one it
		// does not declare, and one it declares in another category - and this
		// surface raises them as its own verdict, because what the role means
		// here is this declaration.
		for _, role := range template.declaredRoles() {
			if _, disposition := structure.Resolve(sealed, role, structure.CategorySemanticRole); disposition != schema.DispositionAccepted {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, id, LawSemanticIdentity, disposition)
			}
		}
		// One role is one axis. Two axes declared under one role would be one
		// coordinate space the engine binds twice, so the repeat is a verdict
		// here rather than a binding whichever axis reaches it first wins.
		if prior, duplicate := semantics[template.semantic]; duplicate {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, prior, LawSemanticUnique, schema.DispositionDuplicate)
		}
		semantics[template.semantic] = id
	}
	for _, template := range templates {
		owner := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: template.key}
		key := carrier.Key("")
		if template.signature != (Signature{}) {
			key = template.signature.Key
		}
		if failure := validateCarrierImports(template.catalog, owner, templates, sealed, key); failure.Available() {
			failure.Entry = template.id
			return failure
		}
	}
	// One published column has one writer. The pair a publication is admitted
	// under is exactly the pair sealed here, so a second declaration of one
	// output name is rejected at the table rather than resolved by whichever
	// writer reaches the column first.
	outputs := make(map[schema.Key]schema.EntryID, len(templates))
	for _, template := range templates {
		for _, output := range template.outputs {
			if !output.Available() {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawOutputDeclared, schema.DispositionIncomplete)
			}
			owner := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: template.key}
			if !output.ID().Available() || output.ID() != member.IssueID(owner, output.Key) {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawMemberIdentity, schema.DispositionMalformed)
			}
			if prior, duplicate := outputs[output.Key]; duplicate {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, prior, LawOutputUnique, schema.DispositionDuplicate)
			}
			outputs[output.Key] = template.id
		}
	}
	// A frame column and a nested member share the same owner-qualified
	// namespace. Rejecting an authored-key collision here keeps a consumer from
	// resolving one spelling to two different row kinds.
	for _, template := range templates {
		for _, output := range template.outputs {
			if catalogHasMemberKey(template.catalog, output.Key) {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawOutputUnique, schema.DispositionDuplicate)
			}
		}
	}
	// A writer is a declared axis, because an axis is a writer principal. A
	// column admitted for a principal this table does not declare would be a
	// capability over a writer no seal ever states a law about.
	for _, template := range templates {
		for _, output := range template.outputs {
			if _, declared := keys[output.Writer]; !declared {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawOutputWriterResolves, schema.DispositionIncomplete)
			}
		}
	}
	// Dependency edges resolve against the sealed inventory, so an axis cannot
	// declare an edge to an axis that is not in this table.
	for _, template := range templates {
		for _, dependency := range template.dependencies {
			if dependency == template.key {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawDependencyResolves, schema.DispositionMalformed)
			}
			if _, declared := keys[dependency]; !declared {
				return seal.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawDependencyResolves, schema.DispositionIncomplete)
			}
		}
	}
	// The edges must admit an order, because a phase that seals one axis over
	// another's authority walks them. The first axis on an unresolvable cycle
	// carries the verdict.
	if blamed, cyclic := firstCyclicEntry(templates); cyclic {
		return seal.SurfaceLawFailure(schema.SurfaceKindAxis, blamed, LawDependencyAcyclic, schema.DispositionMalformed)
	}
	return schema.SealFailure{}
}

// validateCarrierImports checks the target side of each imported binding. A
// foreign reference is accepted only when the target entry exposes an issued
// local authority for that carrier. Unknown target surfaces are left to their
// own surface's authority law; this keeps the axis surface generic over the
// Program/issuance carrier vocabulary.
func validateCarrierImports[A any](catalog member.Catalog, owner schema.EntryReference, templates []*Template[A], sealed seal.Sealed, signatureKey carrier.Key) schema.SealFailure {
	for _, binding := range catalog.CarrierRefs {
		if !binding.Available() || binding.Ref.Owner == owner {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawCarrierReference, schema.DispositionMalformed)
		}
		var target schema.Entry
		var disposition schema.Disposition
		if binding.Ref.Owner.Surface == schema.SurfaceKindAxis {
			for _, candidate := range templates {
				if candidate != nil && candidate.key == binding.Ref.Owner.Key {
					target = candidate
					break
				}
			}
			if target == nil {
				disposition = schema.DispositionIncomplete
			}
		} else {
			// A later surface cannot be inspected through this phase-fenced
			// resolver. The root's post-surface reference law still proves that
			// the owner entry exists; its carrier authority is resolved by the
			// cold relation compiler once the complete schema is available.
			if !sealed.Registered(binding.Ref.Owner.Surface) {
				continue
			}
			target, disposition = sealed.ResolveReference(binding.Ref.Owner)
		}
		if target == nil {
			if disposition == schema.DispositionAccepted {
				disposition = schema.DispositionIncomplete
			}
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawCarrierReference, disposition)
		}
		provider, knowsAuthorities := target.(interface {
			CarrierAuthority(carrier.Key) (carrier.Authority, bool)
		})
		if !knowsAuthorities {
			// A lower surface may own a different carrier vocabulary and validate
			// its authority in its own seal. Axis does not guess that shape.
			continue
		}
		authority, authorityOK := provider.CarrierAuthority(binding.Ref.Carrier)
		if !authorityOK {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawCarrierReference, schema.DispositionIncomplete)
		}
		expected, expectedOK := carrier.Issue(binding.Ref.Owner, carrier.Authority{Carrier: binding.Ref.Carrier, Capability: authority.Capability})
		if !authority.ID().Available() || !expectedOK || authority.ID() != expected.ID() {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawCarrierReference, schema.DispositionMalformed)
		}
		if binding.Use == signatureKey && authority.Capability != carrier.Equatable && authority.Capability != carrier.Ascending {
			return seal.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawCarrierSignature, schema.DispositionMalformed)
		}
	}
	return schema.SealFailure{}
}

// firstCyclicEntry reports the first declaration that no dependency order can
// place. It is the seal's own reading of the same edges DependencyOrder walks,
// so a table that seals is a table that orders.
func firstCyclicEntry[A any](templates []*Template[A]) (schema.EntryID, bool) {
	ordered, ok := DependencyOrder(templates)
	if ok {
		return schema.EntryID{}, false
	}
	placed := make(map[schema.Key]struct{}, len(ordered))
	for _, template := range ordered {
		placed[template.key] = struct{}{}
	}
	for _, template := range templates {
		if _, done := placed[template.key]; !done {
			return template.id, true
		}
	}
	return schema.EntryID{}, true
}

// DependencyOrder derives the order a dependency-respecting phase walks one
// axis inventory in: every axis follows the axes it declared an edge to, and
// axes that no edge separates keep their catalog order. It is the surface's
// own derivation because the surface owns the edges; a phase consumes the
// order rather than restating what an edge means.
//
// A cycle admits no order and is reported as such. The already-placed prefix
// travels with the rejection so a caller can name the axes the cycle blocked.
func DependencyOrder[A any](templates []*Template[A]) ([]*Template[A], bool) {
	positions := make(map[schema.Key]int, len(templates))
	for index, template := range templates {
		if template == nil || !template.key.Available() {
			return nil, false
		}
		if _, duplicate := positions[template.key]; duplicate {
			return nil, false
		}
		positions[template.key] = index
	}
	ordered := make([]*Template[A], 0, len(templates))
	placed := make(map[schema.Key]struct{}, len(templates))
	for len(ordered) < len(templates) {
		progressed := false
		for _, template := range templates {
			if _, done := placed[template.key]; done {
				continue
			}
			ready := true
			for _, dependency := range template.dependencies {
				position, resolved := positions[dependency]
				if !resolved {
					return ordered, false
				}
				if _, done := placed[templates[position].key]; !done {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			ordered = append(ordered, template)
			placed[template.key] = struct{}{}
			progressed = true
		}
		if !progressed {
			return ordered, false
		}
	}
	return ordered, true
}
