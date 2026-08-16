// Package queryreg owns the query surface of the analyzer declaration table:
// the record one query family is registered as, and the surface laws the
// declaration root seals it under. It is the declaration half of the query
// story only. The detached result contracts in the sibling query package are
// the other half, and they are consumed rather than referenced here: a
// registration names its result codec by identity, never by Go type, so the
// declaration table stays blind to every domain. That is also why the two
// halves are separate packages: the result contracts carry domain types, and a
// declaration surface that imported them would drag the domains into the
// declaration root.
//
// A registration says four things about a family: what it is called, what
// identity its results are frozen under, how partial results compose, and
// which coordinate spaces it reads. The fold contract is the one that carries
// weight - a family declared distributive may be answered from disjoint
// fragments and joined, while a general fold must see its subject whole - so
// the contract is declared per family and never inferred from a codec.
//
// The codec and fold-contract identities name contracts whose own surface does
// not exist yet; they are form-validated here and resolved when that surface
// lands. Form-validating an identity is not resolving it, and this surface
// does not pretend otherwise. Subject axes are different: the axis surface is
// sealed below this one, so a subject is resolved for real.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package queryreg

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
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

// RegistrationSpec is the authored declaration of one query family.
type RegistrationSpec struct {
	// Family is the query's authored identity and its diagnostic name, so a
	// family has exactly one spelling in the analyzer. It derives the entry
	// identity a verdict carries.
	Family schema.Key
	// Codec is the declared identity a result of this family is frozen under.
	// Two families sharing a codec would publish under one identity, so it is
	// unique across the surface.
	Codec identity.ContentID
	// Fold is how partial results of this family compose.
	Fold Fold
	// Contract is the declared identity of the fold contract itself: the proof
	// obligation the Fold above claims. A family declares which contract it
	// meets; it never claims a fold without naming what makes the claim true.
	Contract identity.ContentID
	// Subjects are the axes this family reads, by their authored keys.
	Subjects []schema.Key
}

// Registration is one admitted query family declaration. It is immutable once
// built.
type Registration struct {
	family   schema.Key
	id       schema.EntryID
	codec    identity.ContentID
	fold     Fold
	contract identity.ContentID
	subjects []schema.Key
}

// NewRegistration admits one authored declaration. A rejected spec returns
// false rather than a partially usable registration.
func NewRegistration(spec RegistrationSpec) (*Registration, bool) {
	if !spec.Family.Available() || !spec.Codec.Available() || !spec.Fold.Available() || !spec.Contract.Available() {
		return nil, false
	}
	if len(spec.Subjects) == 0 {
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
		family:   spec.Family,
		id:       schema.NewEntryID(schema.SurfaceKindQuery, spec.Family),
		codec:    spec.Codec,
		fold:     spec.Fold,
		contract: spec.Contract,
		subjects: append([]schema.Key(nil), spec.Subjects...),
	}
	return registration, registration.EntryAvailable() && registration.declarationComplete()
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

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the family it identifies is completely declared is the
// surface's own law, stated by Seal.
func (registration *Registration) EntryAvailable() bool {
	return registration != nil && registration.family.Available() && registration.id.Available()
}

func (registration *Registration) declarationComplete() bool {
	return registration.codec.Available() && registration.fold.Available() &&
		registration.contract.Available() && len(registration.subjects) > 0
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
	// order; stating the query surface's position states the phase.
	if schema.SurfaceKindAxis >= schema.SurfaceKindQuery {
		return failure(schema.EntryID{}, LawAxisPhase, schema.DispositionMalformed)
	}
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
