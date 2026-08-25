// Package structure owns the structural vocabulary surface of the analyzer
// declaration table: the one place the arm, event, outcome, runtime family,
// constraint form, and diagnostic publication catalogs are declared, and the
// surface laws the declaration root seals them under.
//
// Three of these catalogs are projected across package boundaries. The arm
// catalog has one semantic owner in Flow's BoundaryArmKind; ingress and engine
// expose scalar projections of that owner. The event catalog is
// the canonical Program WTO-event ordinal,
// ingress.EventKind, and the solver schedule's own EventKind; the outcome
// catalog is cold.OutcomeKind, of which ingress accepts a subset.
// Event and outcome projections have to agree for the analyzer to be correct,
// while every arm projection must preserve Flow's ordinal. A member added to
// one spelling and not another is a silent hole, and a member reordered in one
// is a silent mistranslation.
//
// The runtime family catalog is spelled twice: the runtimekind domain owns the
// families as ordinals, and the standard library owns the same eight members
// again as the name domain of type(v). The constraint form catalog is spelled
// twice as well: the expression grammar owns the shapes, and the module
// manifest codec owns the same ten members again as the wire kinds it encodes
// and decodes.
//
// This surface is the single declaration those spellings become projections
// of. A row is a category, a name, a rendered spelling, and the row's dense
// ordinal inside its category. That is all a closed catalog is: the ordinal is
// what a consumer switches on, the spelling is what a consumer renders, the
// density law is what makes an exhaustive switch exhaustive, and the
// per-category population law is what makes the vocabulary total.
//
// The ingress boundary reads this table: its arm, event, and outcome
// projections are lookups of the declared ordinals. The artifact and engine
// projections consume neutral identities from this package. Those ordinals are
// compiled from the Program at load and reach no byte stream of their own - the
// serialized authorities are the program's source, flow, static, and imports
// sections - so the canonical identity remains a schema concern rather than a
// domain-composition concern.
//
// The diagnostic publication catalogs - the observation populations a row is
// measured over, the families publication is gated by, and the severities a row
// defaults to - are declared here for the same reason, and their members are
// named by reference from the diagnostic surface rather than re-bounded there.
// A family is therefore added by declaring a row, which is what makes a
// diagnostic under a family the analyzer has never published before spellable.
//
// The semantic role catalog is declared here for a different reason. Its
// members are not a spelling anything else restates; they are the identities
// every surface binds under, and each was a field of one closed struct no
// domain could extend. Declared here, a domain contributes its own roles, and
// the spelling law over the one category proves every role in the analyzer
// distinct from every other - across surfaces, which no surface's own law can
// reach.
//
// A category is hosted, not owned: several contributors may add rows to one
// category, and Collect is what numbers the aggregate densely so the hosting
// costs the consumer nothing.
//
// Nothing registers itself: declarations are values, handed to the table at
// composition.
package structure

import (
	"github.com/wippyai/go-lua/analysis/schema"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = seal.SurfaceLawFloor + iota
	LawStructureIdentity
	LawCategoryDeclared
	LawOrdinalDeclared
	LawOrdinalUnique
	LawOrdinalDense
	LawCategoryPopulated
	LawAcceptedDeclared
	LawSpellingDeclared
	LawSpellingUnique
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
	// CategoryRuntimeKind is the vocabulary of observable Lua runtime
	// families: the values type() distinguishes.
	CategoryRuntimeKind
	// CategoryConstraintForm is the vocabulary of symbolic integer expression
	// shapes: the closed grammar the constraint domain builds terms from. Its
	// ordinals are the grammar's own numbering, and its members are the wire
	// catalog of the module manifest's expression codec. They are not a
	// serialized graph ABI: a term crosses the manifest boundary under the
	// codec's own wire spelling, and this vocabulary is what that spelling is
	// pinned to.
	CategoryConstraintForm
	// CategoryDiagnosticObservation is the vocabulary of reusable semantic
	// observation populations an artifact issues and a diagnostic row is
	// measured over. Its canonical identities and ordinals are declared by this
	// package and contributed to the sealed table by composition.
	CategoryDiagnosticObservation
	// CategoryDiagnosticFamily is the vocabulary of publication families: the
	// gates a consumer enables findings by at the query boundary. A published
	// code's first segment is the declared spelling of the family it names, so
	// adding a family is adding a row here.
	CategoryDiagnosticFamily
	// CategoryDiagnosticSeverity is the vocabulary of severities a diagnostic
	// row defaults to and a policy refines within. Its ordinals are the severity
	// enum's own numbering, and its spellings are what a rendered finding is
	// labelled with.
	CategoryDiagnosticSeverity
	// CategoryOccurrenceKind is the vocabulary of compiled occurrence families:
	// the row classifications a program artifact carries and a rule subscribes
	// to. Its ordinals are the artifact's own occurrence numbering.
	CategoryOccurrenceKind
	// CategorySemanticRole is the vocabulary of global semantic roles: the
	// identities the engine binds factors, rules, queries, activations, and
	// contract payloads under. Its members are named by reference from every
	// surface that carries an identity, so the spelling law over this one
	// category is what proves no two of them name one role - a distinctness
	// no per-surface law could state, because the surfaces are sealed apart.
	// It is resolved by key alone: no foreign spelling numbers it.
	CategorySemanticRole
	// CategoryNativeNumericRepresentation is the vocabulary of proved numeric
	// carriers a native publication column names. Its ordinals are
	// programschema.NumericRepresentation's own numbering.
	CategoryNativeNumericRepresentation
	// CategoryNativeScalarRepresentation is the vocabulary of proved exact
	// scalar carriers a native publication column names. Its ordinals are
	// keyspace.LiteralKind's own numbering with Lua nil appended.
	CategoryNativeScalarRepresentation
	// CategoryNativeArithmeticOperator is the vocabulary of primitive binary
	// arithmetic operators a native publication column names. Its ordinals are
	// flowkind.BinaryOp's own numbering, whose arithmetic members occupy the
	// first contiguous segment of that vocabulary.
	CategoryNativeArithmeticOperator
	// CategoryNativeUnaryOperator is the vocabulary of Lua unary operators a
	// native publication column names. Its ordinals are flowkind.UnaryOp's own
	// numbering.
	CategoryNativeUnaryOperator
	// CategoryNativeDivisorProperty is the vocabulary of divisor proofs a
	// native publication column names.
	CategoryNativeDivisorProperty
	// CategoryNativeTruthinessClass is the vocabulary of branch-condition truth
	// verdicts a native publication column names.
	CategoryNativeTruthinessClass
	// CategoryNativeBranchPartition is the vocabulary of branch geometries a
	// native publication column names.
	CategoryNativeBranchPartition
	// CategoryNativeBranchArm is the vocabulary of conditional arms a proved
	// partition names dead.
	CategoryNativeBranchArm
	// CategoryNativeNumericOverflow is the vocabulary of arithmetic overflow
	// disciplines a native publication column names. Its ordinals are
	// value.NumericOverflow's own numbering.
	CategoryNativeNumericOverflow
	// CategoryConformanceVerdict is the vocabulary of assignment conformance
	// answers: what a measured value's runtime families say about the type its
	// target declares. Its ordinals are the conformance judgment's own
	// numbering, and a diagnostic row keyed by them declares one presentation
	// per answer, so one published code renders the finding the verdict names
	// rather than one message for every answer at once.
	CategoryConformanceVerdict
	// CategoryPublicationRowClass is the vocabulary of classes a written row of
	// a detached query answer is published at. It is the row-state vocabulary
	// every published plane layout ranks its state byte against, and it is one
	// catalog rather than one per family: a family that publishes rows carrying
	// presence alone names the single class its written rows are at, and two
	// such families name the same class rather than each declaring a private
	// spelling of it.
	CategoryPublicationRowClass
	// CategoryNativeSendSafety is the vocabulary of proved allocation-level
	// send strategies. Its ordinals are domain/sendsafety.Verdict's numbering.
	CategoryNativeSendSafety
	// CategoryReductionOutcome is the vocabulary of fold dispositions: what one
	// reduction concluded, as opposed to the fact it computed. Its ordinals are
	// ReductionOutcome's own numbering, offset by one so the enum's zero value
	// is a declared member rather than an absent row. It is one catalog rather
	// than one per execution plane: the engine's execution substrate, the
	// activation relation, and the Delta path name the same five members, and a
	// fold that spelled a sixth would be encoding a disposition its consumers
	// cannot read.
	CategoryReductionOutcome
	// The four publication descriptor vocabularies. They are what a Target
	// authored about one effect occurrence - what it does, and what it does to
	// escape, mutability and lifetime - and they are catalogs here rather than
	// private bytes beside a codec for the same reason every other published
	// vocabulary is: a consumer resolves a member by name against the seal's
	// rank, so a producer cannot renumber a disposition without the layout
	// digest refusing the payload it used to write.
	//
	// Each one's ordinals ARE its enum's own numbering. The enum's zero value
	// is Invalid, which is the absence of a disposition rather than one of
	// them, so it is not a member and never occupies a rank; the member column
	// states absence with its own presence byte.
	CategoryPublicationEffectKind
	CategoryPublicationEscape
	CategoryPublicationMutability
	CategoryPublicationLifetime
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
	// Spelling is the member's rendered name. A consumer that renders this
	// vocabulary reads it here instead of holding a switch from ordinal to
	// string, and a consumer that compares a member against authored text
	// compares against this declaration rather than deriving a name from the
	// key by taking it apart.
	Spelling string
	// Accepted is the member's admission into the projection its vocabulary
	// feeds. An outcome that concludes a body inside its own function contributes
	// no transfer exit, so the outcome vocabulary declares which of its members a
	// consumer projects. An arm and an event are projected whole, so every member
	// of those vocabularies is accepted.
	Accepted bool
}

// Entry is one admitted structural vocabulary member. It is immutable once
// built.
type Entry struct {
	key      schema.Key
	id       schema.EntryID
	category Category
	ordinal  uint16
	spelling string
	accepted bool
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable entry.
func New(spec Spec) (*Entry, bool) {
	if !spec.Key.Available() || !spec.Category.Available() || spec.Ordinal == 0 || spec.Spelling == "" || (spec.Category != CategoryOutcome && !spec.Accepted) {
		return nil, false
	}
	entry := &Entry{
		key:      spec.Key,
		id:       schema.NewEntryID(schema.SurfaceKindStructure, spec.Key),
		category: spec.Category,
		ordinal:  spec.Ordinal,
		spelling: spec.Spelling,
		accepted: spec.Accepted,
	}
	return entry, entry.EntryAvailable() && entry.declarationComplete()
}

// Collect aggregates the contributed declarations of this surface into one
// admitted inventory, numbering each category by its aggregation position.
//
// A category is hosted rather than owned: the arm catalog arrives from the
// analyzer's own list, the runtime families from the domain that owns them, the
// expression forms from the grammar that owns those, and a category a later
// domain adds rows to arrives the same way. Numbering the aggregate here is
// what keeps the ordinals dense across contributors, which is the property an
// exhaustive consumer switch rests on.
//
// A contributor that authors an ordinal is naming the position a foreign
// spelling is pinned to, so the authored value must be the position the
// aggregation places the row at; a contributor that leaves it zero is declaring
// a category no foreign spelling numbers, and the position is the ordinal.
// Either way the result is dense from one, and a contribution that disagrees
// with its own position is rejected here rather than sealed into a catalog with
// a hole in it.
func Collect(contributions ...[]Spec) ([]*Entry, bool) {
	var counts [categoryLimit]uint16
	var entries []*Entry
	for _, contribution := range contributions {
		for _, spec := range contribution {
			if !spec.Category.Available() {
				return nil, false
			}
			counts[spec.Category]++
			position := counts[spec.Category]
			if spec.Ordinal == 0 {
				spec.Ordinal = position
			} else if spec.Ordinal != position {
				return nil, false
			}
			entry, ok := New(spec)
			if !ok {
				return nil, false
			}
			entries = append(entries, entry)
		}
	}
	return entries, len(entries) > 0
}

// Resolve resolves one reference into the structural vocabulary against the
// table it is being sealed into, and states the category the referring surface
// expects the member to belong to.
//
// Every surface above this one names members of this vocabulary the same way,
// so the resolution and the two ways it fails are stated here once: a name this
// vocabulary does not declare is incomplete, and a name it declares in another
// category is a member of the wrong vocabulary and so malformed. The referring
// surface raises the verdict under its own law, because what the member means
// is that surface's declaration.
func Resolve(sealed seal.Sealed, key schema.Key, category Category) (*Entry, schema.Disposition) {
	if !category.Available() {
		return nil, schema.DispositionMalformed
	}
	row, disposition := sealed.Resolve(schema.SurfaceKindStructure, key)
	if disposition != schema.DispositionAccepted {
		return nil, disposition
	}
	entry, ok := row.(*Entry)
	if !ok || entry == nil || entry.category != category {
		return nil, schema.DispositionMalformed
	}
	return entry, schema.DispositionAccepted
}

func (entry *Entry) Key() schema.Key { return entry.key }

func (entry *Entry) ID() schema.EntryID { return entry.id }

func (entry *Entry) Category() Category { return entry.category }

// Member resolves one vocabulary member by its category and ordinal against a
// table this surface has already sealed into. It is the ordinal counterpart of
// Resolve: a surface above this one that names a member by the ordinal its
// owner assigned reaches the declaration through this rather than projecting a
// whole table of its own, which a surface sealing mid-catalog cannot do.
func Member(sealed seal.Sealed, category Category, ordinal uint16) (*Entry, schema.Disposition) {
	if !category.Available() || ordinal == 0 {
		return nil, schema.DispositionMalformed
	}
	view, registered := sealed.Surface(schema.SurfaceKindStructure)
	if !registered || !view.Available() {
		return nil, schema.DispositionIncomplete
	}
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil || entry.category != category || entry.ordinal != ordinal {
			continue
		}
		return entry, schema.DispositionAccepted
	}
	return nil, schema.DispositionIncomplete
}

func (entry *Entry) Ordinal() uint16 { return entry.ordinal }

// Spelling is the member's declared rendered name. A consumer renders this
// instead of switching from the ordinal to a string of its own.
func (entry *Entry) Spelling() string {
	if entry == nil {
		return ""
	}
	return entry.spelling
}

// Accepted is the member's declared admission into the projection its
// vocabulary feeds. A consumer reads it instead of keeping its own list of the
// members it projects.
func (entry *Entry) Accepted() bool { return entry != nil && entry.accepted }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the vocabulary it belongs to is completely declared is
// the surface's own law, stated by Seal.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.key.Available() && entry.id.Available()
}

// EntryContent writes this member's declared data: the vocabulary it belongs
// to, its dense ordinal inside that vocabulary, its declared admission into the
// projection the vocabulary feeds, and the name it renders as. A consumer
// projects a member from these four, so a member that changes any of them is a
// different declaration and the table digest says so.
func (entry *Entry) EntryContent(content *framing.Writer) error {
	if err := content.Uint(uint64(entry.category)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.ordinal)); err != nil {
		return err
	}
	if err := content.Bool(entry.accepted); err != nil {
		return err
	}
	if err := content.String(entry.spelling); err != nil {
		return err
	}
	return nil
}

func (entry *Entry) declarationComplete() bool {
	return entry.category.Available() && entry.ordinal != 0 && entry.spelling != "" && (entry.category == CategoryOutcome || entry.accepted)
}

// Table is the immutable projection a consumer reads the sealed vocabulary
// through. It is the shape every spelling this surface replaces is derived
// from, so a projection never restates the catalog.
type Table struct {
	members   [categoryLimit][]*Entry
	spellings [categoryLimit]map[string]uint16
}

// NewTable projects one sealed structure view. The density law has already run
// at seal, so the projection is total by construction rather than by check.
func NewTable(view seal.View) (Table, bool) {
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
		spellings := make(map[string]uint16, len(members))
		for _, entry := range members {
			if int(entry.ordinal) > len(ordered) || ordered[entry.ordinal-1] != nil {
				return Table{}, false
			}
			ordered[entry.ordinal-1] = entry
			if _, duplicate := spellings[entry.spelling]; duplicate {
				return Table{}, false
			}
			spellings[entry.spelling] = entry.ordinal
		}
		table.members[category] = ordered
		table.spellings[category] = spellings
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

// Spelling resolves one member by its category and its declared spelling, the
// reverse of At. It is a lookup index built once at seal, not a declared
// column: it carries no information EntryContent does not already write, so
// it enters no digest and a foreign switch on a rendered spelling reads the
// sealed vocabulary through it instead of restating the spellings itself.
func (table Table) Spelling(category Category, spelling string) (uint16, bool) {
	if !category.Available() {
		return 0, false
	}
	ordinal, ok := table.spellings[category][spelling]
	return ordinal, ok
}

// Vocabulary is one sealed category's member identities in declared rank
// order. It is the projection a closed catalog is consumed as when the
// consumer needs the whole vocabulary rather than one member: a wire layout
// ranks its bytes against it, so the ranks a payload carries are the sealed
// declaration's own positions and no consumer restates the catalog to obtain
// them.
func (table Table) Vocabulary(category Category) ([]schema.Key, bool) {
	if !category.Available() {
		return nil, false
	}
	members := table.members[category]
	if len(members) == 0 {
		return nil, false
	}
	keys := make([]schema.Key, len(members))
	for index, entry := range members {
		if entry == nil || !entry.key.Available() {
			return nil, false
		}
		keys[index] = entry.key
	}
	return keys, true
}

// surface is the structural vocabulary contribution to the declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of structural vocabulary declarations to
// the table.
func NewSurface(entries []*Entry) seal.Surface { return surface{entries: entries} }

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
func (contribution surface) Seal(view seal.View, _ seal.Sealed) schema.SealFailure {
	var counts [categoryLimit]int
	var accepted [categoryLimit]int
	var claimed [categoryLimit]map[uint16]schema.EntryID
	var spelled [categoryLimit]map[string]schema.EntryID
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
		// A consumer that renders a member reads the declared spelling, and a
		// consumer that compares a member against authored text compares against
		// it. An undeclared spelling would send both back to a switch of their
		// own, so it is a verdict here rather than an empty string there.
		if entry.spelling == "" {
			return failure(entry.id, LawSpellingDeclared, schema.DispositionIncomplete)
		}
		// Only the outcome vocabulary distinguishes the members its consumers
		// project: an arm and an event are projected whole. A rejected member of
		// those vocabularies is a malformed declaration rather than a smaller
		// catalog, so it is a verdict here rather than a member a consumer
		// quietly never sees.
		if entry.category != CategoryOutcome && !entry.accepted {
			return failure(entry.id, LawAcceptedDeclared, schema.DispositionMalformed)
		}
		if entry.accepted {
			accepted[entry.category]++
		}
		if claimed[entry.category] == nil {
			claimed[entry.category] = make(map[uint16]schema.EntryID, view.Count())
		}
		if prior, duplicate := claimed[entry.category][entry.ordinal]; duplicate {
			return failure(prior, LawOrdinalUnique, schema.DispositionDuplicate)
		}
		claimed[entry.category][entry.ordinal] = entry.id
		if spelled[entry.category] == nil {
			spelled[entry.category] = make(map[string]schema.EntryID, view.Count())
		}
		// A spelling names one member of its vocabulary. Two members rendering
		// the same name would make the rendered catalog smaller than the declared
		// one, so a repeat is a verdict here rather than an ambiguity a renderer
		// resolves by position.
		if prior, duplicate := spelled[entry.category][entry.spelling]; duplicate {
			return failure(prior, LawSpellingUnique, schema.DispositionDuplicate)
		}
		spelled[entry.category][entry.spelling] = entry.id
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
		// A vocabulary every member of which is rejected projects nothing, so the
		// consumer reading the property would silently produce an empty result.
		if accepted[category] == 0 {
			return failure(schema.EntryID{}, LawAcceptedDeclared, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

func failure(entry schema.EntryID, law schema.LawID, disposition schema.Disposition) schema.SealFailure {
	return seal.SurfaceLawFailure(schema.SurfaceKindStructure, entry, law, disposition)
}
