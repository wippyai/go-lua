// Package denominator owns the denominator surface of the analyzer
// declaration table: the record one closed world is declared as, and the
// surface laws the declaration root seals it under.
//
// A denominator is the declared universe a totality claim is made against. A
// claim that something holds for every member, or that a member is absent, is
// only as good as the set it quantifies over, and that set has to be named,
// owned, and closed at a stated point. This surface is where it is named: one
// identity, the surface entry that owns it, the identity of the universe it
// describes, and the phase at which that universe stops admitting members.
//
// Nothing is resolved that cannot be. The owner is resolved against the
// already-sealed surface it names, so a denominator cannot be owned by a row
// that does not exist. The universe is a declared content identity whose own
// describing surface does not exist yet; it is form-validated here and
// resolved when that surface lands. Form-validating an identity is not
// resolving it, and this surface does not pretend otherwise.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package denominator

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawDenominatorIdentity
	LawOwnerDeclared
	LawOwnerPhase
	LawOwnerResolves
	LawUniverseDeclared
	LawUniverseUnique
	LawPhaseDeclared
)

// Phase is the point at which a denominator's universe stops admitting
// members. It is the closed catalog of closure points the analyzer has.
type Phase uint8

const (
	PhaseInvalid Phase = iota
	// PhaseSeal closes the universe when the declaration table seals: its
	// members are declarations, and no later input can add one.
	PhaseSeal
	// PhasePublication closes the universe when the owning surface's facts are
	// published: its members are derived, and the fixpoint that derives them
	// must be closed before the set is total.
	PhasePublication
)

func (phase Phase) Available() bool { return phase == PhaseSeal || phase == PhasePublication }

// Owner is the surface entry a denominator's universe belongs to. It is a
// reference into the same declaration table, so it names a surface and one
// entry's authored key inside it.
type Owner struct {
	Surface schema.SurfaceKind
	Entry   schema.Key
}

func (owner Owner) Available() bool { return owner.Surface.Available() && owner.Entry.Available() }

// Spec is the authored declaration of one denominator.
type Spec struct {
	// Key is the denominator's authored identity and its diagnostic name, so a
	// closed world has exactly one spelling in the analyzer. It derives the
	// entry identity a verdict carries.
	Key schema.Key
	// Owner is the surface entry whose facts this universe quantifies over.
	Owner Owner
	// Universe is the declared identity of the set description itself. Two
	// denominators over one description are one closed world under two names,
	// so this identity is unique across the surface.
	Universe identity.ContentID
	// Phase is when the universe closes.
	Phase Phase
}

// Entry is one admitted denominator declaration. It is immutable once built.
type Entry struct {
	key      schema.Key
	id       schema.EntryID
	owner    Owner
	universe identity.ContentID
	phase    Phase
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable entry.
func New(spec Spec) (*Entry, bool) {
	if !spec.Key.Available() || !spec.Owner.Available() || !spec.Universe.Available() || !spec.Phase.Available() {
		return nil, false
	}
	// A denominator resolves its owner against a surface sealed below it, so an
	// owner at or above this surface names a table that does not exist yet.
	if spec.Owner.Surface >= schema.SurfaceKindDenominator {
		return nil, false
	}
	entry := &Entry{
		key:      spec.Key,
		id:       schema.NewEntryID(schema.SurfaceKindDenominator, spec.Key),
		owner:    spec.Owner,
		universe: spec.Universe,
		phase:    spec.Phase,
	}
	return entry, entry.EntryAvailable() && entry.declarationComplete()
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Owner() Owner { return entry.owner }

func (entry *Entry) Universe() identity.ContentID { return entry.universe }

func (entry *Entry) Phase() Phase { return entry.phase }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the closed world it identifies is completely declared is
// the surface's own law, stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

func (entry *Entry) declarationComplete() bool {
	return entry.owner.Available() && entry.universe.Available() && entry.phase.Available()
}

// surface is the denominator contribution to the analyzer declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of denominator declarations to the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindDenominator }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the denominator surface's own laws over the indexed view. Every
// owner is resolved against the surface it names, in the same table this
// surface is being sealed into.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	universes := make(map[identity.ContentID]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return failure(schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of this entry's key, so an entry cannot travel under another
		// surface's identity.
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindDenominator, entry.key) {
			return failure(entry.id, LawDenominatorIdentity, schema.DispositionMalformed)
		}
		if !entry.owner.Available() {
			return failure(entry.id, LawOwnerDeclared, schema.DispositionIncomplete)
		}
		// The catalog order is the reference order. An owner at or above this
		// surface has not been sealed yet, so the reference cannot be resolved
		// and is not admitted as though it had been.
		if entry.owner.Surface >= schema.SurfaceKindDenominator {
			return failure(entry.id, LawOwnerPhase, schema.DispositionMalformed)
		}
		owning, owningOK := sealed.Surface(entry.owner.Surface)
		if !owningOK {
			return failure(entry.id, LawOwnerPhase, schema.DispositionIncomplete)
		}
		if _, declared := owning.ByID(schema.NewEntryID(entry.owner.Surface, entry.owner.Entry)); !declared {
			return failure(entry.id, LawOwnerResolves, schema.DispositionIncomplete)
		}
		if !entry.universe.Available() {
			return failure(entry.id, LawUniverseDeclared, schema.DispositionIncomplete)
		}
		// Two denominators over one universe description are one closed world
		// under two names, and a totality claim would then depend on which name
		// it was made under.
		if prior, duplicate := universes[entry.universe]; duplicate {
			return failure(prior, LawUniverseUnique, schema.DispositionDuplicate)
		}
		universes[entry.universe] = entry.id
		if !entry.phase.Available() {
			return failure(entry.id, LawPhaseDeclared, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindDenominator, entry, law, disposition)
}
