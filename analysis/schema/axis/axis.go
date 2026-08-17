// Package axis owns the axis surface of the analyzer declaration table: the
// record one axis is declared as, the typed contexts its hooks receive, the
// algebra an axis publishes, and the surface laws the declaration root seals
// it under.
//
// An axis is one coordinate space the solver holds facts over. Naming the
// space is not enough to register it: an entry also carries the algebra that
// space is ordered by, the storage it lives in, the principal that writes it,
// the cardinality of its key space, its dependency edges, and its lifetime,
// mutability, and concurrency discipline. The engine binds the executable
// algebra; this surface is where the axis is declared, sealed, and derived
// from.
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
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/lattice"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawAxisIdentity
	LawPrincipalDeclared
	LawPrincipalUnique
	LawFieldComplete
	LawMetadataComplete
	LawDependencyResolves
	LawSemanticIdentity
	LawSemanticUnique
	// The ordinal here is retired. The bind phase is the root's law: the
	// declaration catalog order is the bind phase order, and the root rejects
	// out-of-order registration under schema.LawSurfacePhase.
	_
	// The ordinal here is retired. Whether the closed vocabulary is itself
	// available is that package's own law, stated by vocabulary.Bundle.Available
	// and pinned by its own tests. A vocabulary that is not the canonical one
	// selects nothing, so this surface observes it as an axis with no canonical
	// identity and states that under LawSemanticIdentity.
	_
)

// Storage is the closed catalog of places an axis's facts live. A factor axis
// is a Link-bound engine factor cell; a static axis is an inventory carried by
// the compiled program and never written during solve.
type Storage uint8

const (
	StorageInvalid Storage = iota
	StorageFactor
	StorageStatic
)

func (storage Storage) Available() bool {
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

// Adopt projects one engine factor algebra onto the axis surface. It is the
// single conversion between the carrier-typed hot spec an owner binds and the
// ordinal-keyed view an axis entry publishes, so the two cannot drift and the
// owner's carrier coordinate never needs a name outside its own package.
func Adopt[K ~uint32 | ~uint64, V any](spec engine.HotFactorSpec[K, V]) (Algebra[V], bool) {
	algebra := Algebra[V]{
		KeyEnd:      spec.KeyEnd,
		Lattice:     spec.Lattice,
		Default:     spec.Default,
		Fingerprint: spec.Fingerprint,
		Widen:       adoptRank(spec.WidenRank, spec.KeyEnd),
		Narrow:      adoptRank(spec.NarrowRank, spec.KeyEnd),
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

func adoptRank[K ~uint32 | ~uint64, V any](measure engine.Measure[K, V], keyEnd uint64) Rank[V] {
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
// builder and the canonical vocabulary alone.
type Declaration struct {
	Builder *engine.SchemaBuilder
	Bundle  vocabulary.Bundle
}

// Binding is the hot binding context. Fragment is the cold fragment this
// axis's Declare hook produced; Inputs is the composition's own Link input
// record. Axes bind before rules, so no rule authority is reachable here.
type Binding[A, F any] struct {
	Binding  *engine.SchemaBinding
	Fragment F
	Inputs   A
}

// Spec is the authored declaration of one axis. A is the composition's Link
// input record; F, H, and V are this axis's own cold fragment, hot axis, and
// fact carrier. The owning domain keeps its algebra and its owner; what it
// hands over here is the wiring and the published algebra view.
type Spec[A, F, H, V any] struct {
	// Key is the axis's authored identity and its diagnostic name, so an axis
	// has exactly one spelling in the analyzer. It derives the entry identity
	// a verdict carries.
	Key schema.Key
	// Principal is the writer principal of this axis: the factor lane whose
	// rules write it. It is read from the artifact format rather than restated,
	// and it is unique per axis.
	Principal   programartifact.RuleOutputKind
	Storage     Storage
	Cardinality Cardinality
	Lifetime    Lifetime
	Mutability  Mutability
	Concurrency Concurrency
	// Dependencies are the axes this axis's declaration depends on, by their
	// authored keys. An edge must resolve to a declared axis and may not be a
	// self-edge.
	Dependencies []schema.Key
	// Semantic selects the axis identity from the canonical vocabulary.
	Semantic func(vocabulary.Bundle) identity.SemanticKey
	// Declare records the axis's cold Schema shape and returns its fragment.
	Declare func(Declaration) (F, bool)
	// Bind instantiates the axis's typed factor binding and returns the hot
	// axis. It is the one place the carrier coordinate is instantiated.
	Bind func(Binding[A, F]) (H, bool)
	// Algebra publishes the bound axis's algebra on the surface's ordinal key
	// space. It runs immediately after Bind, against the same authority.
	Algebra func(H) (Algebra[V], bool)
}

// Cell is the opaque per-axis payload the table carries between passes. It is
// produced and consumed only by the typed thunks one Spec instantiated, so an
// authored hook never sees it and never asserts.
type Cell struct {
	value   any
	algebra any
}

func (cell Cell) Available() bool { return cell.value != nil }

// AlgebraAvailable reports whether this cell carries a published algebra. Only
// a hot cell does.
func (cell Cell) AlgebraAvailable() bool { return cell.algebra != nil }

// Payload recovers one cell's value at its declared type. It is how an axis's
// own owner reaches its fragment or bound axis; the table itself never needs
// it.
func Payload[T any](cell Cell) (T, bool) {
	value, ok := cell.value.(T)
	return value, ok
}

// AlgebraOf recovers one bound axis's published algebra at its declared fact
// type.
func AlgebraOf[V any](cell Cell) (Algebra[V], bool) {
	algebra, ok := cell.algebra.(Algebra[V])
	return algebra, ok && algebra.Available()
}

// Template is one admitted axis declaration, erased in its fragment, hot axis,
// and fact carrier but still typed in the composition's Link input record. It
// is immutable once built.
type Template[A any] struct {
	key       schema.Key
	id        schema.EntryID
	principal programartifact.RuleOutputKind

	storage      Storage
	cardinality  Cardinality
	lifetime     Lifetime
	mutability   Mutability
	concurrency  Concurrency
	dependencies []schema.Key

	semantic func(vocabulary.Bundle) identity.SemanticKey
	declare  func(Declaration) (Cell, bool)
	bind     func(*engine.SchemaBinding, A, Cell) (Cell, bool)
}

// New admits one authored declaration and instantiates its typed hooks. A
// rejected spec returns false rather than a partially usable template.
func New[A, F, H, V any](spec Spec[A, F, H, V]) (*Template[A], bool) {
	if !specAdmissible(spec) {
		return nil, false
	}
	template := &Template[A]{
		key:          spec.Key,
		id:           schema.NewEntryID(schema.SurfaceKindAxis, spec.Key),
		principal:    spec.Principal,
		storage:      spec.Storage,
		cardinality:  spec.Cardinality,
		lifetime:     spec.Lifetime,
		mutability:   spec.Mutability,
		concurrency:  spec.Concurrency,
		dependencies: append([]schema.Key(nil), spec.Dependencies...),
		semantic:     spec.Semantic,
	}
	template.declare = func(context Declaration) (Cell, bool) {
		fragment, ok := spec.Declare(context)
		if !ok {
			return Cell{}, false
		}
		return Cell{value: fragment}, true
	}
	// The bind thunk is the axis's typed instantiation: it performs the axis's
	// own factor binding and publishes the algebra of that exact binding in one
	// step, so a bound axis without a published algebra cannot exist.
	template.bind = func(binding *engine.SchemaBinding, inputs A, holder Cell) (Cell, bool) {
		fragment, fragmentOK := holder.value.(F)
		if !fragmentOK {
			return Cell{}, false
		}
		bound, ok := spec.Bind(Binding[A, F]{Binding: binding, Fragment: fragment, Inputs: inputs})
		if !ok {
			return Cell{}, false
		}
		algebra, algebraOK := spec.Algebra(bound)
		if !algebraOK || !algebra.Available() {
			return Cell{}, false
		}
		return Cell{value: bound, algebra: algebra}, true
	}
	return template, template.EntryAvailable() && template.metadataComplete() && template.fieldsComplete()
}

func specAdmissible[A, F, H, V any](spec Spec[A, F, H, V]) bool {
	if !spec.Key.Available() || spec.Principal == programartifact.RuleOutputInvalid {
		return false
	}
	if !spec.Storage.Available() || !spec.Cardinality.Available() || !spec.Lifetime.Available() ||
		!spec.Mutability.Available() || !spec.Concurrency.Available() {
		return false
	}
	if spec.Semantic == nil || spec.Declare == nil || spec.Bind == nil || spec.Algebra == nil {
		return false
	}
	for _, dependency := range spec.Dependencies {
		if !dependency.Available() || dependency == spec.Key {
			return false
		}
	}
	return true
}

func (template *Template[A]) Key() schema.Key { return template.key }

func (template *Template[A]) ID() schema.EntryID { return template.id }

// Principal is the factor lane whose rules write this axis.
func (template *Template[A]) Principal() programartifact.RuleOutputKind { return template.principal }

func (template *Template[A]) Storage() Storage { return template.storage }

func (template *Template[A]) Cardinality() Cardinality { return template.cardinality }

func (template *Template[A]) Lifetime() Lifetime { return template.lifetime }

func (template *Template[A]) Mutability() Mutability { return template.mutability }

func (template *Template[A]) Concurrency() Concurrency { return template.concurrency }

func (template *Template[A]) DependencyCount() int { return len(template.dependencies) }

func (template *Template[A]) DependencyAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(template.dependencies) {
		return "", false
	}
	return template.dependencies[index], true
}

// Semantic resolves this axis's canonical identity in one vocabulary bundle.
func (template *Template[A]) Semantic(bundle vocabulary.Bundle) identity.SemanticKey {
	if template == nil || template.semantic == nil {
		return identity.SemanticKey{}
	}
	return template.semantic(bundle)
}

// semanticIdentity resolves this axis's identity in the canonical vocabulary.
// It is the one evaluation both the content fold and this surface's admission
// laws read, so the identity a catalog is digested under and the identity it is
// sealed under cannot differ.
//
// It is total: an axis that declares no selector, and one whose selector names
// nothing in the canonical vocabulary, both resolve to the absent identity. The
// surface's own LawFieldComplete and LawSemanticIdentity state those cases, so
// the content stream stays writable for every row the root hands it.
func (template *Template[A]) semanticIdentity() identity.SemanticKey {
	if template == nil || template.semantic == nil {
		return identity.SemanticKey{}
	}
	bundle, canonical := vocabulary.New()
	if !canonical {
		return identity.SemanticKey{}
	}
	return template.semantic(bundle)
}

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the axis it identifies is completely declared is the
// surface's own law, stated by Seal, so an incomplete axis is reported as the
// incomplete axis it is rather than as an unidentifiable row.
func (template *Template[A]) EntryAvailable() bool {
	return template != nil && template.key.Available() && template.id.Available()
}

// EntryContent writes this axis's declarative half: the canonical identity it
// is bound under, the principal that writes it, the storage its facts live in,
// the shape of its key space, and its lifetime, mutability, and concurrency
// disciplines, followed by its dependency edges in declaration order. A derived
// inventory reads a coordinate space from exactly these, so an axis that changes
// one of them is a different space and the table digest says so.
//
// The semantic identity is content. The selector is a hook, but what it selects
// is a role of the closed vocabulary, and the engine binds this axis's factor
// under that role, so two catalogs whose axes select different roles declare
// different coordinate spaces. The identity is written as its canonical bytes -
// the digest and the interpretation version - rather than as the role's
// spelling, so no authored text reaches the stream.
//
// The typed hooks are not content. A hook is a function value with no canonical
// bytes, and the algebra one publishes is produced at bind against a live
// binding rather than declared here, so neither is written. What the hooks are
// declared against is covered by the identity and metadata above and by the
// surface's own admission laws.
func (template *Template[A]) EntryContent(content *framing.Writer) error {
	semantic := template.semanticIdentity()
	digest := semantic.Digest()
	if err := content.Bytes(digest[:]); err != nil {
		return err
	}
	if err := content.Uint(semantic.Version()); err != nil {
		return err
	}
	if err := content.Uint(uint64(template.principal)); err != nil {
		return err
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
	return nil
}

func (template *Template[A]) metadataComplete() bool {
	return template.storage.Available() && template.cardinality.Available() &&
		template.lifetime.Available() && template.mutability.Available() && template.concurrency.Available()
}

func (template *Template[A]) fieldsComplete() bool {
	return template.semantic != nil && template.declare != nil && template.bind != nil
}

// Declare records this axis's cold shape.
func (template *Template[A]) Declare(context Declaration) (Cell, bool) {
	if template == nil || template.declare == nil || context.Builder == nil {
		return Cell{}, false
	}
	holder, ok := template.declare(context)
	return holder, ok && holder.Available()
}

// Bind instantiates this axis's factor binding and publishes its algebra.
func (template *Template[A]) Bind(binding *engine.SchemaBinding, inputs A, fragment Cell) (Cell, bool) {
	if template == nil || template.bind == nil || binding == nil || !fragment.Available() {
		return Cell{}, false
	}
	holder, ok := template.bind(binding, inputs, fragment)
	return holder, ok && holder.Available() && holder.AlgebraAvailable()
}

// surface is the axis contribution to the analyzer declaration root.
type surface[A any] struct{ templates []*Template[A] }

// NewSurface hands one ordered set of axis declarations to the table.
func NewSurface[A any](templates []*Template[A]) schema.Surface {
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

// Seal states the axis surface's own laws over the indexed view. Axes are the
// first surface in the catalog, so no sealed sibling is reachable here.
func (contribution surface[A]) Seal(view schema.View, _ schema.Sealed) schema.SealFailure {
	keys := make(map[schema.Key]schema.EntryID, view.Count())
	principals := make(map[programartifact.RuleOutputKind]schema.EntryID, view.Count())
	semantics := make(map[identity.SemanticKey]schema.EntryID, view.Count())
	templates := make([]*Template[A], 0, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		template, templateOK := entry.(*Template[A])
		if !entryOK || !templateOK || template == nil {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		templates = append(templates, template)
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !template.key.Available() || template.id != schema.NewEntryID(schema.SurfaceKindAxis, template.key) {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawAxisIdentity, schema.DispositionMalformed)
		}
		keys[template.key] = template.id
		if template.principal == programartifact.RuleOutputInvalid {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawPrincipalDeclared, schema.DispositionIncomplete)
		}
		if prior, duplicate := principals[template.principal]; duplicate {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, prior, LawPrincipalUnique, schema.DispositionDuplicate)
		}
		principals[template.principal] = template.id
		if !template.metadataComplete() {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawMetadataComplete, schema.DispositionIncomplete)
		}
		if !template.fieldsComplete() {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawFieldComplete, schema.DispositionIncomplete)
		}
		semantic := template.semanticIdentity()
		if !semantic.Available() {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawSemanticIdentity, schema.DispositionMalformed)
		}
		if prior, duplicate := semantics[semantic]; duplicate {
			return schema.SurfaceLawFailure(schema.SurfaceKindAxis, prior, LawSemanticUnique, schema.DispositionDuplicate)
		}
		semantics[semantic] = template.id
	}
	// Dependency edges resolve against the sealed inventory, so an axis cannot
	// declare an edge to an axis that is not in this table.
	for _, template := range templates {
		for _, dependency := range template.dependencies {
			if dependency == template.key {
				return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawDependencyResolves, schema.DispositionMalformed)
			}
			if _, declared := keys[dependency]; !declared {
				return schema.SurfaceLawFailure(schema.SurfaceKindAxis, template.id, LawDependencyResolves, schema.DispositionIncomplete)
			}
		}
	}
	return schema.SealFailure{}
}
