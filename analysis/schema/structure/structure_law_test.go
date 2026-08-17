package structure

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchEntry is a stand-in row for a sibling surface. The declaration root
// requires every catalog member to be populated, so a structural vocabulary
// law is stated against a complete table rather than a half registered one.
type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

type scratchSurface struct{ kind schema.SurfaceKind }

func (contribution scratchSurface) Kind() schema.SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "scratch"}}
}

func (contribution scratchSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// sealEntries seals one structural inventory into a complete declaration table.
// The catalog is walked rather than listed, so the surfaces the declaration
// root settles on do not change what these laws assert.
func sealEntries(t *testing.T, entries []*Entry) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealContribution(t, NewSurface(entries))
}

// sealContribution seals one arbitrary contribution under this surface's kind,
// so a law about what this surface accepts as a row is stated against the
// public seal path rather than against the unexported entry type alone.
func sealContribution(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindStructure {
			builder.Register(contribution)
			continue
		}
		builder.Register(scratchSurface{kind: kind})
	}
	return builder.Seal()
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok || entry == nil {
		t.Fatalf("structural member %q rejected by construction", spec.Key)
	}
	return entry
}

// canonicalVocabulary is the catalog this surface consolidates: the eight
// structural arms, the three bracket events, the seven body outcomes, the eight
// Lua runtime families, and the ten symbolic expression forms that the analyzer
// today spells once per consumer.
func canonicalVocabulary(t *testing.T) []*Entry {
	t.Helper()
	var entries []*Entry
	add := func(category Category, names ...schema.Key) {
		for index, name := range names {
			entries = append(entries, mustEntry(t, Spec{Key: name, Category: category, Ordinal: uint16(index + 1), Accepted: true}))
		}
	}
	add(CategoryArm, "arm/local", "arm/resume", "arm/select-true", "arm/select-false",
		"arm/tail", "arm/throw", "arm/yield", "arm/cancel")
	add(CategoryEvent, "event/enter", "event/point", "event/exit")
	add(CategoryRuntimeKind, "nil", "boolean", "number", "string",
		"table", "function", "thread", "userdata")
	add(CategoryConstraintForm, "constraint-form/var", "constraint-form/const",
		"constraint-form/binop", "constraint-form/len", "constraint-form/param",
		"constraint-form/ret", "constraint-form/param-len", "constraint-form/ret-len",
		"constraint-form/min", "constraint-form/max")
	outcomes := []struct {
		key      schema.Key
		accepted bool
	}{
		{"outcome/normal", true}, {"outcome/return", true}, {"outcome/throw", true},
		{"outcome/break", false}, {"outcome/goto", false}, {"outcome/yield", true}, {"outcome/cancel", true},
	}
	for index, outcome := range outcomes {
		entries = append(entries, mustEntry(t, Spec{Key: outcome.key, Category: CategoryOutcome, Ordinal: uint16(index + 1), Accepted: outcome.accepted}))
	}
	return entries
}

// TestStructureSurfaceSealsTheCanonicalVocabulary is the baseline and the
// modeling proof at once: the three catalogs the analyzer spells six times
// over are declared once, sealed, and projected back at their declared
// ordinals.
func TestStructureSurfaceSealsTheCanonicalVocabulary(t *testing.T) {
	sealed, failure := sealEntries(t, canonicalVocabulary(t))
	if failure.Available() {
		t.Fatalf("canonical structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	if table.Count(CategoryArm) != 8 || table.Count(CategoryEvent) != 3 || table.Count(CategoryOutcome) != 7 ||
		table.Count(CategoryRuntimeKind) != 8 || table.Count(CategoryConstraintForm) != 10 {
		t.Fatalf("projected sizes: arms=%d events=%d outcomes=%d families=%d forms=%d",
			table.Count(CategoryArm), table.Count(CategoryEvent), table.Count(CategoryOutcome),
			table.Count(CategoryRuntimeKind), table.Count(CategoryConstraintForm))
	}
	for category, name := range map[Category]schema.Key{
		CategoryArm:            "arm/local",
		CategoryEvent:          "event/enter",
		CategoryOutcome:        "outcome/normal",
		CategoryRuntimeKind:    "nil",
		CategoryConstraintForm: "constraint-form/var",
	} {
		entry, ok := table.At(category, 1)
		if !ok || entry.Key() != name || entry.Category() != category || entry.Ordinal() != 1 {
			t.Fatalf("category %d does not begin at its declared first member", category)
		}
	}
	if _, ok := table.At(CategoryEvent, 4); ok {
		t.Fatal("projection answered beyond the declared vocabulary")
	}
	if _, ok := table.At(CategoryInvalid, 1); ok {
		t.Fatal("projection answered for a category outside the catalog")
	}
}

// foreignSurface contributes a row that is not this surface's entry type,
// under this surface's kind, and states this surface's own seal over it.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindStructure }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "foreign"}}
}

func (contribution foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// TestForeignRowIsRejected states the shape law: a structural member is read
// from a row this surface itself built, so a row that identifies one entry and
// is not one of this surface's declarations is rejected rather than counted
// into a vocabulary a consumer switches on.
func TestForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealContribution(t, foreignSurface{})
	if sealed != nil || !failure.Available() {
		t.Fatal("a foreign row was admitted into the structural vocabulary")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindStructure {
		t.Fatalf("contributor=%d want the structural surface", failure.Contributor)
	}
}

// TestStructureIdentityIsThisSurfaceDerivation states that a member carries
// this surface's own derivation of its key.
func TestStructureIdentityIsThisSurfaceDerivation(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[0].id = schema.NewEntryID(schema.SurfaceKindAxis, entries[0].key)
	_, failure := sealEntries(t, entries)
	if failure.Law != LawStructureIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestStructureMemberKeyIsUnique states that two members cannot share one
// authored identity, across categories as well as within one.
func TestStructureMemberKeyIsUnique(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[len(entries)-1].key = entries[0].key
	entries[len(entries)-1].id = schema.NewEntryID(schema.SurfaceKindStructure, entries[0].key)
	_, failure := sealEntries(t, entries)
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate member key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestStructureCategoryIsDeclared states that a member belongs to a
// vocabulary.
func TestStructureCategoryIsDeclared(t *testing.T) {
	for name, category := range map[string]Category{
		"undeclared":      CategoryInvalid,
		"out of catalog":  categoryLimit,
		"beyond the edge": categoryLimit + 1,
	} {
		entries := canonicalVocabulary(t)
		entries[0].category = category
		_, failure := sealEntries(t, entries)
		if failure.Law != LawCategoryDeclared || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("member with a %s category sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestStructureOrdinalIsDeclared states that a member has a position its
// consumers can switch on.
func TestStructureOrdinalIsDeclared(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[0].ordinal = 0
	_, failure := sealEntries(t, entries)
	if failure.Law != LawOrdinalDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("member without an ordinal sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestStructureOrdinalIsUniqueWithinItsCategory states that two members of one
// vocabulary cannot occupy one position.
func TestStructureOrdinalIsUniqueWithinItsCategory(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[1].ordinal = entries[0].ordinal
	_, failure := sealEntries(t, entries)
	if failure.Law != LawOrdinalUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("repeated ordinal sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Entry != entries[0].ID() {
		t.Fatalf("verdict named entry %x, not the prior claimant", failure.Entry)
	}
}

// TestStructureOrdinalsAreDense states the law that makes an exhaustive
// consumer switch provable: a vocabulary numbers its members from one with no
// gap, so a projection over its ordinals reaches every member.
func TestStructureOrdinalsAreDense(t *testing.T) {
	entries := canonicalVocabulary(t)
	for _, entry := range entries {
		if entry.category == CategoryEvent && entry.ordinal == 2 {
			entry.ordinal = 9
		}
	}
	_, failure := sealEntries(t, entries)
	if failure.Law != LawOrdinalDense || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("vocabulary with an ordinal gap sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestStructureCategoriesArePopulated states the totality law: this surface is
// the single declaration of all three vocabularies, so leaving one out is an
// incomplete table rather than a smaller one.
func TestStructureCategoriesArePopulated(t *testing.T) {
	var entries []*Entry
	for _, entry := range canonicalVocabulary(t) {
		if entry.category != CategoryOutcome {
			entries = append(entries, entry)
		}
	}
	_, failure := sealEntries(t, entries)
	if failure.Law != LawCategoryPopulated || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("vocabulary missing a whole category sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestOutcomeVocabularyDeclaresItsAcceptedMembers states the property the
// ingress body projection reads: a body's exits are the points of its accepted
// outcomes, and which outcomes those are is declared here rather than listed at
// the consumer. Break and Goto conclude a body inside its own function, so they
// contribute no transfer exit.
func TestOutcomeVocabularyDeclaresItsAcceptedMembers(t *testing.T) {
	sealed, failure := sealEntries(t, canonicalVocabulary(t))
	if failure.Available() {
		t.Fatalf("canonical structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	for key, accepted := range map[schema.Key]bool{
		"outcome/normal": true, "outcome/return": true, "outcome/throw": true,
		"outcome/break": false, "outcome/goto": false, "outcome/yield": true, "outcome/cancel": true,
	} {
		var member *Entry
		for ordinal := uint16(1); ordinal <= uint16(table.Count(CategoryOutcome)); ordinal++ {
			entry, ok := table.At(CategoryOutcome, ordinal)
			if ok && entry.Key() == key {
				member = entry
			}
		}
		if member == nil {
			t.Fatalf("outcome %q is not declared", key)
		}
		if member.Accepted() != accepted {
			t.Fatalf("outcome %q is accepted=%t, want %t", key, member.Accepted(), accepted)
		}
	}
	for ordinal := uint16(1); ordinal <= uint16(table.Count(CategoryArm)); ordinal++ {
		entry, ok := table.At(CategoryArm, ordinal)
		if !ok || !entry.Accepted() {
			t.Fatalf("arm at ordinal %d is not projected whole", ordinal)
		}
	}
	for ordinal := uint16(1); ordinal <= uint16(table.Count(CategoryEvent)); ordinal++ {
		entry, ok := table.At(CategoryEvent, ordinal)
		if !ok || !entry.Accepted() {
			t.Fatalf("event at ordinal %d is not projected whole", ordinal)
		}
	}
}

// TestVocabulariesProjectedWholeAcceptEveryMember states the first half of the
// admission law: an arm or an event that declares itself rejected is malformed,
// because those vocabularies have no unprojected members to be.
func TestVocabulariesProjectedWholeAcceptEveryMember(t *testing.T) {
	for name, category := range map[string]Category{"arm": CategoryArm, "event": CategoryEvent} {
		entries := canonicalVocabulary(t)
		for _, entry := range entries {
			if entry.category == category && entry.ordinal == 1 {
				entry.accepted = false
			}
		}
		_, failure := sealEntries(t, entries)
		if failure.Law != LawAcceptedDeclared || failure.Disposition != schema.DispositionMalformed {
			t.Fatalf("rejected %s member sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestVocabularyAcceptsAtLeastOneMember states the second half: a vocabulary
// whose every member is rejected projects nothing, so the consumer reading the
// property would silently produce an empty result.
func TestVocabularyAcceptsAtLeastOneMember(t *testing.T) {
	entries := canonicalVocabulary(t)
	for _, entry := range entries {
		if entry.category == CategoryOutcome {
			entry.accepted = false
		}
	}
	_, failure := sealEntries(t, entries)
	if failure.Law != LawAcceptedDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("vocabulary accepting no member sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestTableDigestCoversDeclaredContent is the law the accepted property is read
// against: the digest is the drift guard every derived inventory is checked
// against, so it covers what a member declares and not only which member it is.
// Two vocabularies that name the same members and admit different ones into the
// projection are two vocabularies, and the digest says so.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealEntries(t, canonicalVocabulary(t))
	if failure.Available() {
		t.Fatalf("canonical structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	shifted := canonicalVocabulary(t)
	for _, entry := range shifted {
		if entry.category == CategoryOutcome && entry.key == "outcome/break" {
			entry.accepted = true
		}
	}
	alternative, failure := sealEntries(t, shifted)
	if failure.Available() {
		t.Fatalf("alternative structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == alternative.Digest() {
		t.Fatal("a member's declared admission left the table digest unchanged")
	}
}

// TestNewRejectsIncompleteSpec states the constructor half: a spec that
// violates a law yields no entry at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]Spec{
		"key":                {Category: CategoryArm, Ordinal: 1, Accepted: true},
		"category":           {Key: "arm/local", Ordinal: 1, Accepted: true},
		"catalog":            {Key: "arm/local", Category: categoryLimit, Ordinal: 1, Accepted: true},
		"ordinal":            {Key: "arm/local", Category: CategoryArm, Accepted: true},
		"declared admission": {Key: "arm/local", Category: CategoryArm, Ordinal: 1},
	}
	for name, spec := range cases {
		if entry, ok := New(spec); ok || entry != nil {
			t.Fatalf("spec without a %s admitted", name)
		}
	}
}

// TestTableRejectsAForeignView states that the projection is of this surface's
// own sealed view and of nothing else.
func TestTableRejectsAForeignView(t *testing.T) {
	sealed, failure := sealEntries(t, canonicalVocabulary(t))
	if failure.Available() {
		t.Fatalf("canonical structural vocabulary rejected: law=%d", failure.Law)
	}
	foreign, foreignOK := sealed.Surface(schema.SurfaceKindAxis)
	if !foreignOK {
		t.Fatal("scratch axis surface did not seal")
	}
	if _, projected := NewTable(foreign); projected {
		t.Fatal("a foreign surface view projected as the structural vocabulary")
	}
	if _, projected := NewTable(schema.View{}); projected {
		t.Fatal("an unavailable view projected as the structural vocabulary")
	}
}
