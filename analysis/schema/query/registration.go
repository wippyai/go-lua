// Package query owns the query surface of the analyzer declaration table:
// the record one query family is registered as, and the surface laws the
// declaration root seals it under. It is the declaration half of the query
// story only. The detached result contracts are the other half, and they are
// consumed rather than referenced here: a registration names its result codec
// by identity, never by Go type, so the declaration table stays blind to every
// domain. The result contracts carry domain types, and a declaration surface
// that imported them would drag the domains into the declaration root.
//
// A registration says four things about a family: what it is called, what
// identity its results are frozen under, how partial results compose, and
// which coordinate spaces it reads. The fold contract is the one that carries
// weight - a family declared distributive may be answered from disjoint
// fragments and joined, while a general fold must see its subject whole - so
// the contract is declared per family and never inferred from a codec.
//
// The contributor that opens a slot, installs a fold, and recovers a sealed
// implementation is composition wiring. It is selected by this family's
// declaration key and does not live on the sealed record.
//
// The codec and fold-contract identities are named as semantic roles and
// resolved against the sealed role vocabulary when the family is admitted, so
// a contributor names its identities and never mints one. Subject axes are
// resolved against the axis surface, which is sealed below this one.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package query

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawRegistrationIdentity
	LawAxisPhase
	LawCodecDeclared
	LawCodecUnique
	LawFoldDeclared
	LawSubjectDeclared
	LawSubjectUnique
	LawSubjectResolves
	_
	LawPopulationDeclared
	LawPopulationResolves
	LawProjectionDeclared
	LawProjectionResolves
	LawProjectionFold
)

// Artifact population and projection roles a family declares. They are
// semantic-role keys; construction enumerates families by these instead of
// restating family names.
const (
	PopulationSelectedPoint schema.Key = "semantic/query/population/selected-point"
	// PopulationObservation declares a query family that is only attached as
	// an explicit observation. It is a sealed producer population, not a
	// request for one result row at every selected Program point; construction
	// must therefore keep it out of SelectedQuerySites.
	PopulationObservation schema.Key = "semantic/query/population/observation"
	ProjectionSummary     schema.Key = "semantic/query/projection/summary"
	ProjectionExact       schema.Key = "semantic/query/projection/exact"
)

// Fold is the closed catalog of ways a family's partial results compose.
type Fold uint8

const (
	FoldInvalid Fold = iota
	// FoldDistributive admits answering the family over disjoint fragments of
	// its subject and joining the fragments, because the join of the answers is
	// the answer over the join.
	FoldDistributive
	// FoldGeneral admits no such split: the family is answered over its whole
	// subject or not at all.
	FoldGeneral
)

func (fold Fold) Available() bool { return fold == FoldDistributive || fold == FoldGeneral }

// Subjects is one pass's view of the axis payloads a family reads: the cold
// fragment or the bound axis each of its subject spaces produced, keyed by the
// axis's authored key. The composition holds the axis pass and opens the view;
// a family's own declaration narrows it, so a contributor reaches exactly the
// coordinate spaces its registration is on record as reading.
type Subjects struct{ cells map[schema.Key]axis.Cell }

// NewSubjects opens a subject view over one pass's axis payloads.
func NewSubjects(cells map[schema.Key]axis.Cell) Subjects {
	view := Subjects{cells: make(map[schema.Key]axis.Cell, len(cells))}
	for key, cell := range cells {
		view.cells[key] = cell
	}
	return view
}

// At recovers one subject axis's payload. The cell stays opaque here; the
// contributor that declared the subject recovers it at the axis owner's type.
func (subjects Subjects) At(key schema.Key) (axis.Cell, bool) {
	cell, held := subjects.cells[key]
	return cell, held && cell.Available()
}

// restrict narrows a pass's payloads to exactly one family's declared
// subjects. A declared subject the pass produced no payload for leaves the
// view unavailable rather than partial, so a contributor never runs against a
// space its own axis did not bind.
func (subjects Subjects) restrict(keys []schema.Key) (Subjects, bool) {
	view := Subjects{cells: make(map[schema.Key]axis.Cell, len(keys))}
	for _, key := range keys {
		cell, held := subjects.cells[key]
		if !held || !cell.Available() {
			return Subjects{}, false
		}
		view.cells[key] = cell
	}
	return view, true
}

// Declaration is the cold context one family's Declare hook receives: the
// two identities the family is declared under, and the payloads of its
// declared subject axes' cold pass. The schema builder is composition
// wiring and is not carried here.
type Declaration struct {
	// Semantic is the identity the query slot is declared under, and Freezer
	// the identity its results are published and opened under. Both are
	// resolved from the roles the family named, so a contributor installs the
	// identities its registration carries and cannot open a slot under another.
	Semantic identity.SemanticKey
	Freezer  identity.SemanticKey
	Subjects Subjects
}

// Binding is the hot context one family's Bind hook receives: the cold
// fragment its own Declare hook produced, and the payloads of its declared
// subject axes' hot pass. The schema binding is composition wiring.
type Binding[F any] struct {
	Fragment F
	Subjects Subjects
}

// Sealed is the sealed context one family's Recover hook receives. It runs
// after the binding seals. The schema binding is composition wiring.
type Sealed[F any] struct {
	Fragment F
}

// Spec is the authored declaration of one query family. The owning domain
// keeps its fold, observation type, and contributor; what it hands over here
// is the sealed declaration.
type Spec struct {
	// Family is the query's authored identity and its diagnostic name, so a
	// family has exactly one spelling in the analyzer. It derives the entry
	// identity a verdict carries.
	Family schema.Key
	// Semantic is the role naming the identity this family's query slot is
	// declared under.
	Semantic schema.Key
	// Codec is the role naming the identity a result of this family is frozen
	// under. Two families sharing a codec would publish under one identity, so
	// the resolved identity is unique across the surface.
	Codec schema.Key
	// Fold is how partial results of this family compose.
	Fold Fold
	// Contract is the role naming the fold contract itself: the proof
	// obligation the Fold above claims. A family declares which contract it
	// meets; it never claims a fold without naming what makes the claim true.
	Contract schema.Key
	// Subjects are the axes this family reads, by their authored keys.
	Subjects []schema.Key
	// Population is the Artifact geometry this family is asked at.
	Population schema.Key
	// Projection is the read shape construction uses to attach this family.
	Projection schema.Key
}

// Cell is the opaque per-family payload the table carries between passes. It is
// produced and consumed only by the typed thunks one Spec instantiated, so an
// authored hook never sees it and never asserts.
type Cell struct{ value any }

func (cell Cell) Available() bool { return cell.value != nil }

// Payload recovers one cell's value at its declared type. It is how a family's
// own consumer reaches its fragment or sealed implementation; the table itself
// never needs it.
func Payload[T any](cell Cell) (T, bool) {
	value, ok := cell.value.(T)
	return value, ok
}

// Registration is one admitted query family declaration, erased in its cold
// fragment and sealed implementation but carrying the typed contributor that
// produces both. It is immutable once built.
type Registration struct {
	family     schema.Key
	id         schema.EntryID
	codec      identity.ContentID
	fold       Fold
	contract   identity.ContentID
	subjects   []schema.Key
	population schema.Key
	projection schema.Key

	semantic identity.SemanticKey
	freezer  identity.SemanticKey
}

// New admits one authored declaration. The identities the family is
// published under are resolved from the sealed role vocabulary here, so the
// declaration and the slot the contributor opens name one contract. A
// rejected spec returns false rather than a partially usable registration.
func New(spec Spec, roles vocabulary.Roles) (*Registration, bool) {
	if !spec.Family.Available() || !spec.Fold.Available() || len(spec.Subjects) == 0 ||
		!spec.Population.Available() || !spec.Projection.Available() {
		return nil, false
	}
	semantic, semanticOK := roles.Key(spec.Semantic)
	codec, codecOK := roles.Key(spec.Codec)
	contract, contractOK := roles.Key(spec.Contract)
	_, populationOK := roles.Key(spec.Population)
	_, projectionOK := roles.Key(spec.Projection)
	if !semanticOK || !codecOK || !contractOK || !populationOK || !projectionOK {
		return nil, false
	}
	if !projectionAgrees(spec.Fold, spec.Projection) {
		return nil, false
	}
	seen := make(map[schema.Key]bool, len(spec.Subjects))
	for _, subject := range spec.Subjects {
		if !subject.Available() || seen[subject] {
			return nil, false
		}
		seen[subject] = true
	}
	registration := &Registration{
		family:     spec.Family,
		id:         schema.NewEntryID(schema.SurfaceKindQuery, spec.Family),
		codec:      identity.ContentID(codec.Digest()),
		fold:       spec.Fold,
		contract:   identity.ContentID(contract.Digest()),
		subjects:   append([]schema.Key(nil), spec.Subjects...),
		population: spec.Population,
		projection: spec.Projection,
		semantic:   semantic,
		freezer:    codec,
	}
	return registration, registration.EntryAvailable() && registration.declarationComplete()
}

// RestrictSubjects narrows a pass's payloads to exactly one family's declared
// subjects. A declared subject the pass produced no payload for leaves the
// view unavailable rather than partial.
func RestrictSubjects(subjects Subjects, keys []schema.Key) (Subjects, bool) {
	return subjects.restrict(keys)
}

// NewCell holds one typed contributor payload in an opaque cell.
func NewCell(value any) Cell {
	if value == nil {
		return Cell{}
	}
	return Cell{value: value}
}

// Semantic is the identity the family's query slot is declared under.
func (registration *Registration) Semantic() identity.SemanticKey {
	if registration == nil {
		return identity.SemanticKey{}
	}
	return registration.semantic
}

// Freezer is the identity the family's results are published under.
func (registration *Registration) Freezer() identity.SemanticKey {
	if registration == nil {
		return identity.SemanticKey{}
	}
	return registration.freezer
}

// Subjects returns the family's declared subject axes in declaration order.
func (registration *Registration) Subjects() []schema.Key {
	if registration == nil {
		return nil
	}
	return append([]schema.Key(nil), registration.subjects...)
}

func (registration *Registration) Key() schema.Key { return registration.family }

func (registration *Registration) ID() schema.EntryID { return registration.id }

func (registration *Registration) Codec() identity.ContentID { return registration.codec }

func (registration *Registration) Fold() Fold { return registration.fold }

func (registration *Registration) Contract() identity.ContentID { return registration.contract }

func (registration *Registration) SubjectCount() int { return len(registration.subjects) }

func (registration *Registration) SubjectAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(registration.subjects) {
		return "", false
	}
	return registration.subjects[index], true
}

func (registration *Registration) Population() schema.Key { return registration.population }

func (registration *Registration) Projection() schema.Key { return registration.projection }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the family it identifies is completely declared is the
// surface's own law, stated by Seal.
func (registration *Registration) EntryAvailable() bool {
	return registration != nil && registration.family.Available() && registration.id.Available()
}

// EntryContent writes this family's declared data: the codec its results are
// frozen under, how its partial results compose, the contract that discharges
// that fold claim, and the axes it reads, in declaration order. The fold class
// is what admits answering a family from disjoint fragments, so a family that
// changes it is a different family and the table digest says so.
//
// The contributor is behaviour and not content: what a consumer is owed is the
// declaration above, and the identities in it are what a contributor is held to.
func (registration *Registration) EntryContent(content *framing.Writer) error {
	if err := content.Bytes(registration.codec[:]); err != nil {
		return err
	}
	if err := content.Uint(uint64(registration.fold)); err != nil {
		return err
	}
	if err := content.Bytes(registration.contract[:]); err != nil {
		return err
	}
	if err := content.Count(uint64(len(registration.subjects))); err != nil {
		return err
	}
	for _, subject := range registration.subjects {
		if err := content.String(string(subject)); err != nil {
			return err
		}
	}
	if err := content.String(string(registration.population)); err != nil {
		return err
	}
	return content.String(string(registration.projection))
}

func (registration *Registration) declarationComplete() bool {
	return registration.codec.Available() && registration.fold.Available() &&
		registration.contract.Available() && len(registration.subjects) > 0 &&
		registration.population.Available() && registration.projection.Available()
}

func projectionAgrees(fold Fold, projection schema.Key) bool {
	switch fold {
	case FoldDistributive:
		return projection == ProjectionSummary
	case FoldGeneral:
		return projection == ProjectionExact
	default:
		return false
	}
}

// surface is the query contribution to the analyzer declaration root.
type surface struct{ registrations []*Registration }

// NewSurface hands one ordered set of query family declarations to the table.
func NewSurface(registrations []*Registration) schema.Surface {
	return surface{registrations: registrations}
}

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindQuery }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.registrations))
	for index, registration := range contribution.registrations {
		entries[index] = registration
	}
	return entries
}

// Seal states the query surface's own laws over the indexed view. Subject axes
// are resolved against the already-sealed axis surface, so a family that reads
// a coordinate space which does not exist is rejected here rather than
// discovered at answer time.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	// A family resolves its subjects against the axis inventory, so the axis
	// surface must be sealed below it. The catalog order is the bind phase
	// order, and asking the sealed projection for the axis surface is what
	// states that phase: a projection reaches the surfaces below the one
	// currently sealing and none at or above it, so an axis surface that is
	// absent from it is an axis surface this family may not resolve against.
	axes, axesOK := sealed.Surface(schema.SurfaceKindAxis)
	if !axesOK {
		return failure(schema.EntryID{}, LawAxisPhase, schema.DispositionIncomplete)
	}
	codecs := make(map[identity.ContentID]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		registration, registrationOK := row.(*Registration)
		if !rowOK || !registrationOK || registration == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !registration.family.Available() || registration.id != schema.NewEntryID(schema.SurfaceKindQuery, registration.family) {
			return failure(registration.id, LawRegistrationIdentity, schema.DispositionMalformed)
		}
		if !registration.codec.Available() {
			return failure(registration.id, LawCodecDeclared, schema.DispositionIncomplete)
		}
		if prior, duplicate := codecs[registration.codec]; duplicate {
			return failure(prior, LawCodecUnique, schema.DispositionDuplicate)
		}
		codecs[registration.codec] = registration.id
		// A fold claim without the contract that discharges it is a claim
		// about nothing, so the two are declared together or neither is.
		if !registration.fold.Available() || !registration.contract.Available() {
			return failure(registration.id, LawFoldDeclared, schema.DispositionIncomplete)
		}
		if len(registration.subjects) == 0 {
			return failure(registration.id, LawSubjectDeclared, schema.DispositionIncomplete)
		}
		subjects := make(map[schema.Key]bool, len(registration.subjects))
		for _, subject := range registration.subjects {
			if !subject.Available() {
				return failure(registration.id, LawSubjectDeclared, schema.DispositionIncomplete)
			}
			if subjects[subject] {
				return failure(registration.id, LawSubjectUnique, schema.DispositionDuplicate)
			}
			subjects[subject] = true
			if !axisDeclared(axes, subject) {
				return failure(registration.id, LawSubjectResolves, schema.DispositionIncomplete)
			}
		}
		if !registration.population.Available() {
			return failure(registration.id, LawPopulationDeclared, schema.DispositionIncomplete)
		}
		if _, disposition := sealed.Resolve(schema.SurfaceKindStructure, registration.population); disposition != schema.DispositionAccepted {
			return failure(registration.id, LawPopulationResolves, disposition)
		}
		if !registration.projection.Available() {
			return failure(registration.id, LawProjectionDeclared, schema.DispositionIncomplete)
		}
		if _, disposition := sealed.Resolve(schema.SurfaceKindStructure, registration.projection); disposition != schema.DispositionAccepted {
			return failure(registration.id, LawProjectionResolves, disposition)
		}
		if !projectionAgrees(registration.fold, registration.projection) {
			return failure(registration.id, LawProjectionFold, schema.DispositionMalformed)
		}
	}
	return schema.SealFailure{}
}

// axisDeclared resolves one authored axis key against the sealed axis surface.
// The query surface never sees an axis's own record: it derives the axis
// surface's identity for the key it was handed and asks the sealed view, so a
// reference is resolved against the same table it is being sealed into.
func axisDeclared(axes schema.View, key schema.Key) bool {
	if !key.Available() {
		return false
	}
	id := schema.NewEntryID(schema.SurfaceKindAxis, key)
	if !id.Available() {
		return false
	}
	_, declared := axes.ByID(id)
	return declared
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindQuery, entry, law, disposition)
}
