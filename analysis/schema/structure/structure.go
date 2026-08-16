// Package structure owns the structural vocabulary surface of the analyzer
// declaration table: the one place the arm, event, and outcome catalogs are
// declared, and the surface laws the declaration root seals them under.
//
// These three catalogs are today spelled three times over. The arm catalog is
// programartifact.RouteKind and ingress.StructuralArm, related by a private
// mapping function; the event catalog is programartifact.WTOEventKind,
// ingress.EventKind, and the solver schedule's own EventKind; the outcome
// catalog is programartifact.OutcomeKind, of which ingress accepts a subset.
// Each triplet has to agree for the analyzer to be correct, and nothing checks
// that they do: a member added to one spelling and not another is a silent
// hole, and a member reordered in one is a silent mistranslation.
//
// This surface is the single declaration those spellings become projections
// of. A row is a category, a name, and the row's dense ordinal inside its
// category. That is all a closed catalog is: the ordinal is what a consumer
// switches on, the density law is what makes an exhaustive switch exhaustive,
// and the per-category population law is what makes the vocabulary total.
//
// The consumer cutover is not performed here. Every one of the six spellings
// lives in a package this surface may not edit, so replacing them with a
// projection of this table is a following lane; what lands here is the
// declaration those projections will be derived from. Declaring the catalog
// without cutting the consumers over leaves the triplication visible rather
// than hidden, which is the honest intermediate state.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package structure

import "github.com/wippyai/go-lua/analysis/schema"

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawStructureIdentity
	LawCategoryDeclared
	LawOrdinalDeclared
	LawOrdinalUnique
	LawOrdinalDense
	LawCategoryPopulated
)

// Category is the closed catalog of structural vocabularies this surface
// declares.
type Category uint8

const (
	CategoryInvalid Category = iota
	// CategoryArm is the vocabulary of structural edge arms: the ways control
	// leaves one point for another.
	CategoryArm
	// CategoryEvent is the vocabulary of bracket-stream events: the ways a
	// traversal enters, visits, and leaves a region.
	CategoryEvent
	// CategoryOutcome is the vocabulary of body outcomes: the ways a body's
	// execution concludes.
	CategoryOutcome
	categoryLimit
)

func (category Category) Available() bool {
	return category > CategoryInvalid && category < categoryLimit
}

// Spec is the authored declaration of one structural vocabulary member.
type Spec struct {
	// Key is the member's authored identity and its diagnostic name, so a
	// member has exactly one spelling in the analyzer. It derives the entry
	// identity a verdict carries.
	Key schema.Key
	// Category is the vocabulary this member belongs to.
	Category Category
	// Ordinal is the member's position inside its own category, numbered from
	// one. It is the value a consumer's projection switches on, so it is dense
	// and unique within the category: a gap would make an exhaustive switch
	// unprovable, and a repeat would make two members one.
	Ordinal uint16
}

// Entry is one admitted structural vocabulary member. It is immutable once
// built.
type Entry struct {
	key      schema.Key
	id       schema.EntryID
	category Category
	ordinal  uint16
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable entry.
func New(spec Spec) (*Entry, bool) {
	if !spec.Key.Available() || !spec.Category.Available() || spec.Ordinal == 0 {
		return nil, false
	}
	entry := &Entry{
		key:      spec.Key,
		id:       schema.NewEntryID(schema.SurfaceKindStructure, spec.Key),
		category: spec.Category,
		ordinal:  spec.Ordinal,
	}
	return entry, entry.EntryAvailable() && entry.declarationComplete()
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Category() Category { return entry.category }

func (entry *Entry) Ordinal() uint16 { return entry.ordinal }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the vocabulary it belongs to is completely declared is
// the surface's own law, stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

func (entry *Entry) declarationComplete() bool {
	return entry.category.Available() && entry.ordinal != 0
}

// Table is the immutable projection a consumer reads the sealed vocabulary
// through. It is the shape every spelling this surface replaces is derived
// from, so a projection never restates the catalog.
type Table struct {
	members [categoryLimit][]*Entry
}

// NewTable projects one sealed structure view. The density law has already run
// at seal, so the projection is total by construction rather than by check.
func NewTable(view schema.View) (Table, bool) {
	if view.Kind() != schema.SurfaceKindStructure || !view.Available() {
		return Table{}, false
	}
	var table Table
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil || !entry.category.Available() || entry.ordinal == 0 {
			return Table{}, false
		}
		table.members[entry.category] = append(table.members[entry.category], entry)
	}
	for category := CategoryInvalid + 1; category < categoryLimit; category++ {
		members := table.members[category]
		if len(members) == 0 {
			return Table{}, false
		}
		ordered := make([]*Entry, len(members))
		for _, entry := range members {
			if int(entry.ordinal) > len(ordered) || ordered[entry.ordinal-1] != nil {
				return Table{}, false
			}
			ordered[entry.ordinal-1] = entry
		}
		table.members[category] = ordered
	}
	return table, true
}

// Count is the size of one declared vocabulary.
func (table Table) Count(category Category) int {
	if !category.Available() {
		return 0
	}
	return len(table.members[category])
}

// At resolves one member by its category and dense ordinal.
func (table Table) At(category Category, ordinal uint16) (*Entry, bool) {
	if !category.Available() || ordinal == 0 || int(ordinal) > len(table.members[category]) {
		return nil, false
	}
	entry := table.members[category][ordinal-1]
	return entry, entry != nil
}

// surface is the structural vocabulary contribution to the declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of structural vocabulary declarations to
// the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindStructure }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the structural vocabulary's own totality laws over the indexed
// view. A consumer projection switches on the declared ordinals, so the laws
// that make such a switch exhaustive are stated here once rather than assumed
// at every consumer.
func (contribution surface) Seal(view schema.View, _ schema.Sealed) schema.SealFailure {
	var counts [categoryLimit]int
	var claimed [categoryLimit]map[uint16]schema.EntryID
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
		if !entry.key.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindStructure, entry.key) {
			return failure(entry.id, LawStructureIdentity, schema.DispositionMalformed)
		}
		if !entry.category.Available() {
			return failure(entry.id, LawCategoryDeclared, schema.DispositionIncomplete)
		}
		if entry.ordinal == 0 {
			return failure(entry.id, LawOrdinalDeclared, schema.DispositionIncomplete)
		}
		if claimed[entry.category] == nil {
			claimed[entry.category] = make(map[uint16]schema.EntryID, view.Count())
		}
		if prior, duplicate := claimed[entry.category][entry.ordinal]; duplicate {
			return failure(prior, LawOrdinalUnique, schema.DispositionDuplicate)
		}
		claimed[entry.category][entry.ordinal] = entry.id
		counts[entry.category]++
	}
	// A vocabulary is total only if its ordinals are dense from one. A gap
	// makes an exhaustive consumer switch unprovable, so the gap is a verdict
	// here rather than a default arm there.
	for category := CategoryInvalid + 1; category < categoryLimit; category++ {
		if counts[category] == 0 {
			return failure(schema.EntryID{}, LawCategoryPopulated, schema.DispositionIncomplete)
		}
		for ordinal := 1; ordinal <= counts[category]; ordinal++ {
			if _, declared := claimed[category][uint16(ordinal)]; !declared {
				return failure(schema.EntryID{}, LawOrdinalDense, schema.DispositionIncomplete)
			}
		}
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return schema.SurfaceLawFailure(schema.SurfaceKindStructure, entry, law, disposition)
}
