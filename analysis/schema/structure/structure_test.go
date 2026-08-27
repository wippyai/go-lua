package structure

import (
	"testing"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
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

func (contribution scratchSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// sealEntries seals one structural inventory into a complete declaration table.
// The catalog is walked rather than listed, so the surfaces the declaration
// root settles on do not change what these laws assert.
func sealEntries(t *testing.T, entries []*Entry) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	return sealContribution(t, NewSurface(entries))
}

// sealContribution seals one arbitrary contribution under this surface's kind,
// so a law about what this surface accepts as a row is stated against the
// public seal path rather than against the unexported entry type alone.
func sealContribution(t *testing.T, contribution seal.Surface) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	builder := seal.NewBuilder()
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
// today spells once per consumer, together with the three publication
// vocabularies the diagnostic surface names members of and the five occurrence
// geometry vocabularies the rule surface declares its subscriptions against.
func canonicalVocabulary(t *testing.T) []*Entry {
	t.Helper()
	return canonicalSpecs(t, canonicalContributions())
}

// member is one authored row of the fixture catalog: the key it is identified
// by, the name it renders as, and its admission into the projection.
type member struct {
	key      schema.Key
	spelling string
	accepted bool
}

// canonicalContributions is the fixture catalog as the contributions it is
// hosted from, one per category, in the order the surface numbers them.
func canonicalContributions() [][]Spec {
	arms := []member{
		{"arm/local", "local", true}, {"arm/resume", "resume", true},
		{"arm/select-true", "select-true", true}, {"arm/select-false", "select-false", true},
		{"arm/tail", "tail", true}, {"arm/throw", "throw", true},
		{"arm/yield", "yield", true}, {"arm/cancel", "cancel", true},
	}
	events := []member{
		{"event/enter", "enter", true}, {"event/point", "point", true}, {"event/exit", "exit", true},
	}
	families := []member{
		{"nil", "nil", true}, {"boolean", "boolean", true}, {"number", "number", true},
		{"string", "string", true}, {"table", "table", true}, {"function", "function", true},
		{"thread", "thread", true}, {"userdata", "userdata", true},
	}
	forms := []member{
		{"constraint-form/var", "var", true}, {"constraint-form/const", "const", true},
		{"constraint-form/binop", "binop", true}, {"constraint-form/len", "len", true},
		{"constraint-form/param", "param", true}, {"constraint-form/ret", "ret", true},
		{"constraint-form/param-len", "param-len", true}, {"constraint-form/ret-len", "ret-len", true},
		{"constraint-form/min", "min", true}, {"constraint-form/max", "max", true},
	}
	populations := []member{
		{"observation/branch-condition", "branch-condition", true},
		{"observation/type-reference-unresolved", "type-reference-unresolved", true},
		{"observation/value-reference-unresolved", "value-reference-unresolved", true},
		{"observation/type-conformance", "type-conformance", true},
	}
	publications := []member{
		{"family/advice", "advice", true}, {"family/type", "type", true},
		{"family/value", "value", true}, {"family/lint", "lint", true},
	}
	severities := []member{
		{"severity/error", "error", true}, {"severity/warning", "warning", true},
		{"severity/hint", "hint", true},
	}
	occurrences := []member{
		{"occurrence/value-source", "value-source", true}, {"occurrence/call", "call", true},
	}
	semanticRoles := []member{
		{"semantic/factor/probe", "factor/probe", true},
		{"semantic/rule/probe/source", "rule/probe/source", true},
	}
	verdicts := []member{
		{"conformance-verdict/abstains", "abstains", true}, {"conformance-verdict/conforms", "conforms", true},
		{"conformance-verdict/violates", "violates", true}, {"conformance-verdict/may-be-nil", "may be nil", true},
		{"conformance-verdict/member-absent", "member absent", true}, {"conformance-verdict/unproven", "unproven", true},
	}
	outcomes := []member{
		{"outcome/normal", "normal", true}, {"outcome/return", "return", true},
		{"outcome/throw", "throw", true}, {"outcome/break", "break", false},
		{"outcome/goto", "goto", false}, {"outcome/yield", "yield", true},
		{"outcome/cancel", "cancel", true},
	}
	authored := func(category Category, members []member) []Spec {
		specs := make([]Spec, 0, len(members))
		for index, declared := range members {
			specs = append(specs, Spec{
				Key:      declared.key,
				Category: category,
				Ordinal:  uint16(index + 1),
				Spelling: declared.spelling,
				Accepted: declared.accepted,
			})
		}
		return specs
	}
	return [][]Spec{
		authored(CategoryArm, arms),
		authored(CategoryEvent, events),
		authored(CategoryRuntimeKind, families),
		authored(CategoryConstraintForm, forms),
		authored(CategoryOutcome, outcomes),
		authored(CategoryDiagnosticObservation, populations),
		authored(CategoryDiagnosticFamily, publications),
		authored(CategoryDiagnosticSeverity, severities),
		authored(CategoryOccurrenceKind, occurrences),
		authored(CategorySemanticRole, semanticRoles),
		authored(CategoryConformanceVerdict, verdicts),
		NativePublicationSpecs(),
		PublicationPlaneSpecs(),
		PublicationEffectSpecs(),
		ReductionOutcomeSpecs(),
		RelationGeometrySpecs(),
	}
}

func canonicalSpecs(t *testing.T, contributions [][]Spec) []*Entry {
	t.Helper()
	var entries []*Entry
	for _, contribution := range contributions {
		for _, spec := range contribution {
			entries = append(entries, mustEntry(t, spec))
		}
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
		table.Count(CategoryRuntimeKind) != 8 || table.Count(CategoryConstraintForm) != 10 ||
		table.Count(CategoryDiagnosticObservation) != 4 || table.Count(CategoryDiagnosticFamily) != 4 ||
		table.Count(CategoryDiagnosticSeverity) != 3 {
		t.Fatalf("projected sizes: arms=%d events=%d outcomes=%d families=%d forms=%d populations=%d publications=%d severities=%d",
			table.Count(CategoryArm), table.Count(CategoryEvent), table.Count(CategoryOutcome),
			table.Count(CategoryRuntimeKind), table.Count(CategoryConstraintForm),
			table.Count(CategoryDiagnosticObservation), table.Count(CategoryDiagnosticFamily),
			table.Count(CategoryDiagnosticSeverity))
	}
	for category, name := range map[Category]schema.Key{
		CategoryArm:                   "arm/local",
		CategoryEvent:                 "event/enter",
		CategoryOutcome:               "outcome/normal",
		CategoryRuntimeKind:           "nil",
		CategoryConstraintForm:        "constraint-form/var",
		CategoryDiagnosticObservation: "observation/branch-condition",
		CategoryDiagnosticFamily:      "family/advice",
		CategoryDiagnosticSeverity:    "severity/error",
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

func (contribution foreignSurface) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
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
	// Mutate an ordinary row: changing a geometry key would first invalidate
	// its owner-issued carrier, correctly tripping the entry-shape fence before
	// this independent catalog-uniqueness law can run.
	entries[1].key = entries[0].key
	entries[1].id = schema.NewEntryID(schema.SurfaceKindStructure, entries[0].key)
	_, failure := sealEntries(t, entries)
	if failure.Law != seal.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
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

// TestSpellingIsDeclared states the law a renderer rests on: a member carries
// the name it renders as, so a consumer never has to derive one from the key or
// keep a switch from ordinal to string of its own.
func TestSpellingIsDeclared(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[0].spelling = ""
	_, failure := sealEntries(t, entries)
	if failure.Law != LawSpellingDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("member without a spelling sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestSpellingIsUniqueWithinItsCategory states that a rendered name identifies
// one member of its vocabulary. Two members answering to one name would make
// the rendered catalog smaller than the declared one.
func TestSpellingIsUniqueWithinItsCategory(t *testing.T) {
	entries := canonicalVocabulary(t)
	entries[1].spelling = entries[0].spelling
	_, failure := sealEntries(t, entries)
	if failure.Law != LawSpellingUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("repeated spelling sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Entry != entries[0].ID() {
		t.Fatalf("verdict named entry %x, not the prior claimant", failure.Entry)
	}
}

// TestSpellingIsUniqueOnlyWithinItsCategory is the other half of that law: two
// vocabularies are separate catalogs, so a name one of them uses is free in the
// other. An arm and an outcome both spell "throw", and both are correct.
func TestSpellingIsUniqueOnlyWithinItsCategory(t *testing.T) {
	if _, failure := sealEntries(t, canonicalVocabulary(t)); failure.Available() {
		t.Fatalf("a name shared across two vocabularies was rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestSpellingIsDeclaredContent states that the rendered name is part of what
// the entry declares, so a renamed member is a different declaration and the
// table digest says so.
func TestSpellingIsDeclaredContent(t *testing.T) {
	declared, failure := sealEntries(t, canonicalVocabulary(t))
	if failure.Available() {
		t.Fatalf("canonical structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	renamed := canonicalVocabulary(t)
	renamed[0].spelling = "renamed"
	alternative, failure := sealEntries(t, renamed)
	if failure.Available() {
		t.Fatalf("renamed structural vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == alternative.Digest() {
		t.Fatal("a member's declared spelling left the table digest unchanged")
	}
}

// TestCollectNumbersOneCategoryAcrossContributors is the hosting law. A
// category is not owned by one contributor: several may add rows to it, and
// what makes the aggregate a catalog rather than a pile is that Collect numbers
// it densely from one in aggregation order.
func TestCollectNumbersOneCategoryAcrossContributors(t *testing.T) {
	first := []Spec{
		{Key: "probe/one", Category: CategoryArm, Spelling: "one", Accepted: true},
		{Key: "probe/two", Category: CategoryArm, Spelling: "two", Accepted: true},
	}
	second := []Spec{
		{Key: "probe/three", Category: CategoryArm, Spelling: "three", Accepted: true},
	}
	entries, ok := Collect(first, second)
	if !ok || len(entries) != 3 {
		t.Fatalf("two contributions to one category collected %d rows, ok=%t", len(entries), ok)
	}
	for index, entry := range entries {
		if entry.Category() != CategoryArm || entry.Ordinal() != uint16(index+1) {
			t.Fatalf("row %d of the aggregate is category %d ordinal %d", index, entry.Category(), entry.Ordinal())
		}
	}
}

// TestCollectHoldsAnAuthoredOrdinalToItsPosition states what authoring an
// ordinal means. A contributor authors one to name the position a foreign
// spelling is pinned to, so a value that is not the position the aggregation
// places the row at is a contribution that disagrees with its own catalog.
func TestCollectHoldsAnAuthoredOrdinalToItsPosition(t *testing.T) {
	if entries, ok := Collect(canonicalContributions()...); !ok || len(entries) == 0 {
		t.Fatalf("the canonical contributions did not collect: ok=%t rows=%d", ok, len(entries))
	}
	displaced := canonicalContributions()
	displaced[0][1].Ordinal = 5
	if entries, ok := Collect(displaced...); ok || entries != nil {
		t.Fatal("a contribution authoring an ordinal off its position collected")
	}
}

// TestCollectRejectsAnIncompleteContribution states that Collect admits rows
// under the same construction laws a directly authored row is admitted under: a
// contributed row without a spelling, a category, or a key is no row at all.
func TestCollectRejectsAnIncompleteContribution(t *testing.T) {
	for name, spec := range map[string]Spec{
		"spelling": {Key: "probe/one", Category: CategoryArm, Accepted: true},
		"key":      {Category: CategoryArm, Spelling: "one", Accepted: true},
		"category": {Key: "probe/one", Spelling: "one", Accepted: true},
	} {
		if entries, ok := Collect([]Spec{spec}); ok || entries != nil {
			t.Fatalf("a contribution without a %s collected", name)
		}
	}
	if entries, ok := Collect(); ok || entries != nil {
		t.Fatal("an empty aggregation collected an inventory")
	}
}

// TestCollectedDuplicateSpellingIsRejectedAtSeal is the hosting law's other
// edge: two contributors are free to number one category together, and they are
// not free to give one rendered name to two of its members.
func TestCollectedDuplicateSpellingIsRejectedAtSeal(t *testing.T) {
	contributions := canonicalContributions()
	contributions = append(contributions, []Spec{
		{Key: "probe/duplicate", Category: CategoryArm, Spelling: "local", Accepted: true},
	})
	entries, ok := Collect(contributions...)
	if !ok {
		t.Fatal("a contribution repeating a declared spelling was rejected before seal, where the law is not stated")
	}
	_, failure := sealEntries(t, entries)
	if failure.Law != LawSpellingUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("repeated spelling across contributors sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestTwoContributorsCannotDeclareOneSemanticRole is the law that replaced the
// closed semantic vocabulary's own distinctness check. Every identity the
// analyzer binds under is a member of one category here, so the spelling law
// over that category proves every role in the table distinct from every other -
// across surfaces, which no surface's own law can reach, because the surfaces
// are sealed one at a time.
func TestTwoContributorsCannotDeclareOneSemanticRole(t *testing.T) {
	contributions := append(canonicalContributions(), []Spec{
		{Key: "semantic/probe/second-claimant", Category: CategorySemanticRole, Spelling: "factor/probe", Accepted: true},
	})
	entries, ok := Collect(contributions...)
	if !ok {
		t.Fatal("a contribution repeating a declared role was rejected before seal, where the law is not stated")
	}
	_, failure := sealEntries(t, entries)
	if failure.Law != LawSpellingUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("two contributors declaring one semantic role sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
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
		"key":                {Category: CategoryArm, Ordinal: 1, Spelling: "local", Accepted: true},
		"category":           {Key: "arm/local", Ordinal: 1, Spelling: "local", Accepted: true},
		"catalog":            {Key: "arm/local", Category: categoryLimit, Ordinal: 1, Spelling: "local", Accepted: true},
		"ordinal":            {Key: "arm/local", Category: CategoryArm, Spelling: "local", Accepted: true},
		"spelling":           {Key: "arm/local", Category: CategoryArm, Ordinal: 1, Accepted: true},
		"declared admission": {Key: "arm/local", Category: CategoryArm, Ordinal: 1, Spelling: "local"},
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
	if _, projected := NewTable(seal.View{}); projected {
		t.Fatal("an unavailable view projected as the structural vocabulary")
	}
}
