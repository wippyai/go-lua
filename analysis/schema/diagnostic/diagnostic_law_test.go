package diagnostic

import (
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The declared identities a scratch row is named against. The probe family is
// declared and used by nothing else: a family the analyzer has never published
// before is exactly one row of the structural vocabulary, and these laws state
// that a code under it is spellable.
const (
	scratchFamilyAdvice = schema.Key("family/advice")
	scratchFamilyType   = schema.Key("family/type")
	scratchFamilyValue  = schema.Key("family/value")
	scratchFamilyLint   = schema.Key("family/lint")
	scratchFamilyProbe  = schema.Key("family/probe")

	scratchObservationBranch = schema.Key("observation/branch-condition")
	scratchObservationType   = schema.Key("observation/type-reference-unresolved")
	scratchObservationValue  = schema.Key("observation/value-reference-unresolved")
)

// scratchVocabularySurface carries the declared vocabulary a diagnostic row
// resolves its family and observation population against. It states no law of
// its own: the structural surface's totality laws belong to the surface that
// declares the catalog, and what these laws are about is what the diagnostic
// surface does with a member of it.
type scratchVocabularySurface struct{ entries []*structure.Entry }

func (contribution scratchVocabularySurface) Kind() schema.SurfaceKind {
	return schema.SurfaceKindStructure
}

func (contribution scratchVocabularySurface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

func (contribution scratchVocabularySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func scratchVocabulary(t *testing.T) schema.Surface {
	t.Helper()
	specs := []structure.Spec{
		{Key: scratchObservationBranch, Category: structure.CategoryDiagnosticObservation, Ordinal: 1, Spelling: "branch-condition", Accepted: true},
		{Key: scratchObservationType, Category: structure.CategoryDiagnosticObservation, Ordinal: 2, Spelling: "type-reference-unresolved", Accepted: true},
		{Key: scratchObservationValue, Category: structure.CategoryDiagnosticObservation, Ordinal: 3, Spelling: "value-reference-unresolved", Accepted: true},
		{Key: scratchFamilyAdvice, Category: structure.CategoryDiagnosticFamily, Ordinal: 1, Spelling: "advice", Accepted: true},
		{Key: scratchFamilyType, Category: structure.CategoryDiagnosticFamily, Ordinal: 2, Spelling: "type", Accepted: true},
		{Key: scratchFamilyValue, Category: structure.CategoryDiagnosticFamily, Ordinal: 3, Spelling: "value", Accepted: true},
		{Key: scratchFamilyLint, Category: structure.CategoryDiagnosticFamily, Ordinal: 4, Spelling: "lint", Accepted: true},
		{Key: scratchFamilyProbe, Category: structure.CategoryDiagnosticFamily, Ordinal: 5, Spelling: "probe", Accepted: true},
	}
	entries := make([]*structure.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := structure.New(spec)
		if !ok {
			t.Fatalf("scratch vocabulary member %q rejected by construction", spec.Key)
		}
		entries = append(entries, entry)
	}
	return scratchVocabularySurface{entries: entries}
}

// member names one declared vocabulary member from a scratch row.
func member(key schema.Key) Reference {
	return Reference{Surface: schema.SurfaceKindStructure, Key: key}
}

// scratchSiblingSurface stands in for one already-catalogued surface. The
// declaration root requires every catalog member to be registered, so a
// diagnostic law is stated against a complete table rather than a half
// registered one, and a diagnostic reference resolves against a real sealed
// sibling view.
type scratchSiblingSurface struct {
	kind schema.SurfaceKind
	keys []schema.Key
}

type scratchSiblingEntry struct{ key schema.Key }

func (entry scratchSiblingEntry) Key() schema.Key { return entry.key }

func (entry scratchSiblingEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchSiblingEntry) EntryContent(*framing.Writer) error { return nil }

func (surface scratchSiblingSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface scratchSiblingSurface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(surface.keys))
	for index, key := range surface.keys {
		entries[index] = scratchSiblingEntry{key: key}
	}
	return entries
}

func (surface scratchSiblingSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// scratchSpec is one complete diagnostic declaration. Each law test starts
// from this record and damages exactly the field the law is about.
func scratchSpec(code Code, family schema.Key) Spec {
	return Spec{
		Code:            code,
		Family:          member(family),
		DefaultSeverity: SeverityHint,
		Lane:            LaneBranch,
		Observation:     member(scratchObservationBranch),
		Fact:            Reference{Surface: schema.SurfaceKindAxis, Key: "value"},
		Requirements:    RequiresSubject,
		Message:         "unknown value {subject}",
		Help:            "Declare the value",
		Evidence: []Evidence{
			{Anchor: AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unspecified", Detail: "no value named {subject} is declared"},
		},
		Labels: []Label{{Anchor: AnchorPrimary, Text: "unknown value"}},
		Render: []Section{SectionSummary, SectionLocation, SectionSource, SectionEvidence, SectionHelp},
	}
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok || entry == nil {
		t.Fatalf("scratch diagnostic %q rejected by construction", spec.Code)
	}
	return entry
}

// sealSurfaces seals one diagnostic inventory into a complete declaration
// table. The catalog is walked rather than listed, so the surfaces the
// declaration root settles on do not change what these laws assert, and the
// two surfaces a diagnostic reference resolves against carry real inventories.
func sealSurfaces(t *testing.T, entries []*Entry, axes []schema.Key) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealContribution(t, NewSurface(entries), axes)
}

// sealContribution seals one arbitrary contribution under this surface's kind,
// so a law about what this surface accepts as a row is stated against the
// public seal path rather than against the unexported entry type alone.
func sealContribution(t *testing.T, contribution schema.Surface, axes []schema.Key) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindDiagnostic:
			builder.Register(contribution)
		case schema.SurfaceKindStructure:
			builder.Register(scratchVocabulary(t))
		case schema.SurfaceKindAxis:
			builder.Register(scratchSiblingSurface{kind: kind, keys: axes})
		case schema.SurfaceKindRule:
			builder.Register(scratchSiblingSurface{kind: kind, keys: []schema.Key{"value-source"}})
		default:
			builder.Register(scratchSiblingSurface{kind: kind, keys: []schema.Key{"scratch"}})
		}
	}
	return builder.Seal()
}

func sealEntries(t *testing.T, entries []*Entry) schema.SealFailure {
	t.Helper()
	_, failure := sealSurfaces(t, entries, []schema.Key{"value", "heap"})
	return failure
}

// TestDiagnosticSurfaceSealsCompleteInventory is the baseline: a complete
// declaration is admitted, indexed, and sealed with no verdict.
func TestDiagnosticSurfaceSealsCompleteInventory(t *testing.T) {
	entries := []*Entry{
		mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice)),
		mustEntry(t, scratchSpec("value.reference.unresolved", scratchFamilyValue)),
	}
	if failure := sealEntries(t, entries); failure.Available() {
		t.Fatalf("complete diagnostic inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// foreignSurface contributes a row that is not this surface's entry type,
// under this surface's kind, and states this surface's own seal over it.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindDiagnostic }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchSiblingEntry{key: "advice.foreign"}}
}

func (contribution foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// TestForeignRowIsRejected states the shape law: a diagnostic is read from a
// row this surface itself built, so a row that identifies one entry and is not
// one of this surface's declarations is rejected rather than published as a
// code with no declared presentation.
func TestForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealContribution(t, foreignSurface{}, []schema.Key{"value", "heap"})
	if sealed != nil || !failure.Available() {
		t.Fatal("a foreign row was admitted into the diagnostic surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindDiagnostic {
		t.Fatalf("contributor=%d want the diagnostic surface", failure.Contributor)
	}
}

// TestDiagnosticCodeIsThisSurfaceDerivation states that a diagnostic carries
// this surface's own derivation of its code, so a row cannot travel under
// another surface's identity.
func TestDiagnosticCodeIsThisSurfaceDerivation(t *testing.T) {
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	entry.id = schema.NewEntryID(schema.SurfaceKindRule, schema.Key(entry.code))
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawCodeIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticCodeIsUnique states that two rows cannot publish one code. The
// entry identity is derived from the code, so the root rejects the duplicate.
func TestDiagnosticCodeIsUnique(t *testing.T) {
	first := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	second := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	failure := sealEntries(t, []*Entry{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate diagnostic code sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticFamilyIsDeclared states that a row names the family it is
// published under and that the published code carries that same family, so
// publication gating by family cannot disagree with the code a reader sees.
func TestDiagnosticFamilyIsDeclared(t *testing.T) {
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	entry.family = member(scratchFamilyLint)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawFamilyDeclared || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("code published outside its declared family sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if _, ok := New(scratchSpec("advice.always_true_guard", "")); ok {
		t.Fatal("row without a declared family admitted")
	}
	if _, ok := New(scratchSpec("always_true_guard", scratchFamilyAdvice)); ok {
		t.Fatal("code without a family segment admitted")
	}
}

// TestDiagnosticFamilyIsDeclaredNotEnumerated is the addition law of the
// family vocabulary: publishing under a family the analyzer has never published
// before is declaring one row on the structural surface, and nothing else. The
// probe family is declared and used by no analyzer row, so the code under it
// exercises the whole path - admission, resolution, and the spelling law - on a
// family no Go type names.
func TestDiagnosticFamilyIsDeclaredNotEnumerated(t *testing.T) {
	declared := mustEntry(t, scratchSpec("probe.example", scratchFamilyProbe))
	if failure := sealEntries(t, []*Entry{declared}); failure.Available() {
		t.Fatalf("code under a declared family rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// A family no row declares resolves to nothing. The code is well formed and
	// the reference is well formed; what is missing is the declaration, so the
	// verdict is incomplete rather than malformed.
	undeclared := mustEntry(t, scratchSpec("probe.example", scratchFamilyProbe))
	undeclared.family = member("family/undeclared")
	failure := sealEntries(t, []*Entry{undeclared})
	if failure.Law != LawFamilyDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("code under an undeclared family sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticReferencesResolveDownward states what a named identity may be:
// a member of the vocabulary the referring field is about, on the surface that
// declares it. A name that resolves to a member of another vocabulary is the
// wrong catalog, and a name that reaches a surface sealed at or above this one
// reaches a table that does not exist yet; both are malformed rather than
// resolutions a consumer would have to re-check.
func TestDiagnosticReferencesResolveDownward(t *testing.T) {
	crossed := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	crossed.family = member(scratchObservationBranch)
	failure := sealEntries(t, []*Entry{crossed})
	if failure.Law != LawFamilyDeclared || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("family naming an observation population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	crossed = mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	crossed.observation = member(scratchFamilyAdvice)
	failure = sealEntries(t, []*Entry{crossed})
	if failure.Law != LawObservationDeclared || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("observation naming a family sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	upward := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	upward.family = Reference{Surface: schema.SurfaceKindQuery, Key: scratchFamilyAdvice}
	failure = sealEntries(t, []*Entry{upward})
	if failure.Law != LawFamilyDeclared || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("family named on another surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticObservationIsDeclaredOnce states that the population a row is
// measured over is bounded by the declaration and by nothing else: a name the
// observation vocabulary does not carry is unresolved here, so there is one
// place a population is added and one place it is checked.
func TestDiagnosticObservationIsDeclaredOnce(t *testing.T) {
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	entry.observation = member("observation/undeclared")
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawObservationDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("row measured over an undeclared population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	for _, population := range []schema.Key{scratchObservationBranch, scratchObservationType, scratchObservationValue} {
		measured := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
		measured.observation = member(population)
		if failure := sealEntries(t, []*Entry{measured}); failure.Available() {
			t.Fatalf("row measured over %q rejected: law=%d disposition=%s", population, failure.Law, failure.Disposition)
		}
	}
}

// TestDiagnosticTierFollowsDefaultSeverity states the advisory-tier law: the
// publication tier is derived from the declared default severity and is never
// declared beside it, so the two cannot drift.
func TestDiagnosticTierFollowsDefaultSeverity(t *testing.T) {
	advisory := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	if advisory.Tier() != TierAdvisory {
		t.Fatalf("hint default severity published tier %d", advisory.Tier())
	}
	spec := scratchSpec("value.reference.unresolved", scratchFamilyValue)
	spec.DefaultSeverity = SeverityError
	if mustEntry(t, spec).Tier() != TierError {
		t.Fatal("error default severity did not publish the error tier")
	}
	damaged := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	damaged.defaultSeverity = SeverityInvalid
	failure := sealEntries(t, []*Entry{damaged})
	if failure.Law != LawTierValid || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("row without a default severity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticTemplatesParseAtSeal states that every authored template is
// non-empty and resolves its placeholders against the closed placeholder
// vocabulary. A template is parsed once, here, and never at render time.
func TestDiagnosticTemplatesParseAtSeal(t *testing.T) {
	for _, damaged := range []Text{"", "unknown value {", "unknown value }", "unknown value {nobody}", "unknown value {subject"} {
		spec := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
		spec.Message = damaged
		if _, ok := New(spec); ok {
			t.Fatalf("template %q admitted", damaged)
		}
		spec = scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
		spec.Evidence[0].Detail = damaged
		if _, ok := New(spec); ok {
			t.Fatalf("evidence template %q admitted", damaged)
		}
	}
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	message := entry.Message()
	if message.Text() != "unknown value {subject}" {
		t.Fatalf("parsed line lost its authored text: %q", message.Text())
	}
	if message.Count() != 2 {
		t.Fatalf("parsed message has %d segments", message.Count())
	}
	literal, literalOK := message.At(0)
	placeholder, placeholderOK := message.At(1)
	if !literalOK || !placeholderOK || literal.Literal != "unknown value " || literal.Placeholder != PlaceholderInvalid || placeholder.Placeholder != PlaceholderSubject || placeholder.Literal != "" {
		t.Fatal("parsed message did not project the authored template")
	}
}

// TestDiagnosticRequirementsCoverPlaceholders states both halves of the
// payload law: a template may not read a payload field the row does not
// require, and a row may not require a payload field no template reads.
func TestDiagnosticRequirementsCoverPlaceholders(t *testing.T) {
	unread := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	unread.requirements = RequiresSubject | RequiresTarget
	failure := sealEntries(t, []*Entry{unread})
	if failure.Law != LawRequirementCovered || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("unread requirement sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	unrequired := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	unrequired.requirements = RequiresInvalid
	failure = sealEntries(t, []*Entry{unrequired})
	if failure.Law != LawRequirementCovered || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("template reading an unrequired payload sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticProofAnchorRequiresProofLocation states that a row anchoring
// evidence or a label at its proof site requires the proof location, so the
// anchor a reader is shown is always one the payload carries.
func TestDiagnosticProofAnchorRequiresProofLocation(t *testing.T) {
	spec := scratchSpec("advice.redundant_claim", scratchFamilyAdvice)
	spec.Evidence[0].Anchor = AnchorProof
	entry := mustEntry(t, spec)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawRequirementCovered || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("proof-anchored evidence without a proof location sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec.Requirements = RequiresSubject | RequiresProofLocation
	if failure = sealEntries(t, []*Entry{mustEntry(t, spec)}); failure.Available() {
		t.Fatalf("proof-anchored evidence with a proof location rejected: law=%d", failure.Law)
	}
}

// TestDiagnosticLaneDeclaresItsSubjects states that a row names the lane its
// subjects arrive on, and that the observation population it is measured over
// is declared exactly when that lane consumes one.
func TestDiagnosticLaneDeclaresItsSubjects(t *testing.T) {
	spec := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
	spec.Observation = Reference{}
	if _, ok := New(spec); ok {
		t.Fatal("producing lane without an observation population admitted")
	}
	spec = scratchSpec("lint.unused.local", scratchFamilyLint)
	spec.Lane = LaneDeclared
	spec.Fact = Reference{}
	if _, ok := New(spec); ok {
		t.Fatal("declared lane carrying an observation population admitted")
	}
	spec.Observation = Reference{}
	entry := mustEntry(t, spec)
	if entry.Collectable() {
		t.Fatal("declared lane reported an installed producer")
	}
	if failure := sealEntries(t, []*Entry{entry}); failure.Available() {
		t.Fatalf("declared row rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticStaticObservationIsUnique states that one static observation
// population feeds at most one code, so a static row can be dispatched from
// the sealed table without a second hand-kept mapping.
func TestDiagnosticStaticObservationIsUnique(t *testing.T) {
	first := scratchSpec("type.reference.unresolved", scratchFamilyType)
	first.Lane, first.Observation, first.Fact = LaneStatic, member(scratchObservationType), Reference{}
	second := scratchSpec("value.reference.unresolved", scratchFamilyValue)
	second.Lane, second.Observation, second.Fact = LaneStatic, member(scratchObservationType), Reference{}
	failure := sealEntries(t, []*Entry{mustEntry(t, first), mustEntry(t, second)})
	if failure.Law != LawObservationUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared static observation population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticFactReferenceResolves states the cross-surface law: a row that
// names the declaration its subjects are decided by resolves that name against
// the same sealed table, and a solver-observed row must name one.
func TestDiagnosticFactReferenceResolves(t *testing.T) {
	absent := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	absent.fact = Reference{Surface: schema.SurfaceKindAxis, Key: "absent"}
	failure := sealEntries(t, []*Entry{absent})
	if failure.Law != LawFactResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved fact reference sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	ruled := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	ruled.fact = Reference{Surface: schema.SurfaceKindRule, Key: "value-source"}
	if failure = sealEntries(t, []*Entry{ruled}); failure.Available() {
		t.Fatalf("resolved rule reference rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// A reference upward names a table that is not sealed yet; the catalog
	// order is the dependency order, so it is malformed rather than deferred.
	upward := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	upward.fact = Reference{Surface: schema.SurfaceKindDiagnostic, Key: "advice.always_true_guard"}
	failure = sealEntries(t, []*Entry{upward})
	if failure.Law != LawFactResolves || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("upward fact reference sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	rootless := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
	rootless.Fact = Reference{}
	if _, ok := New(rootless); ok {
		t.Fatal("solver-observed row without a fact reference admitted")
	}
}

// TestDiagnosticRenderPlanIsComplete states that a row declares the sections
// it publishes, in one order, without repeating a section.
func TestDiagnosticRenderPlanIsComplete(t *testing.T) {
	spec := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
	spec.Render = nil
	if _, ok := New(spec); ok {
		t.Fatal("row without a render plan admitted")
	}
	spec.Render = []Section{SectionSummary, SectionSummary}
	if _, ok := New(spec); ok {
		t.Fatal("row repeating a render section admitted")
	}
	spec.Render = []Section{SectionSummary, SectionInvalid}
	if _, ok := New(spec); ok {
		t.Fatal("row with an undeclared render section admitted")
	}
	// A row that publishes evidence must render it; declaring evidence no
	// reader ever sees is an incomplete row, not a silent omission.
	spec = scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
	spec.Render = []Section{SectionSummary, SectionLocation}
	entry := mustEntry(t, spec)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawRenderComplete || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unrendered evidence sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestNewRejectsIncompleteSpec states the constructor half of completeness: a
// spec missing any required field yields no entry at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*Spec){
		"code":   func(spec *Spec) { spec.Code = "" },
		"family": func(spec *Spec) { spec.Family = Reference{} },
		"family-surface": func(spec *Spec) {
			spec.Family = Reference{Key: scratchFamilyAdvice}
		},
		"family-key": func(spec *Spec) {
			spec.Family = Reference{Surface: schema.SurfaceKindStructure}
		},
		"observation-surface": func(spec *Spec) {
			spec.Observation = Reference{Key: scratchObservationBranch}
		},
		"observation-key": func(spec *Spec) {
			spec.Observation = Reference{Surface: schema.SurfaceKindStructure}
		},
		"severity": func(spec *Spec) { spec.DefaultSeverity = SeverityInvalid },
		"lane":     func(spec *Spec) { spec.Lane = LaneInvalid },
		"message":  func(spec *Spec) { spec.Message = "" },
		"help":     func(spec *Spec) { spec.Help = "" },
		"render":   func(spec *Spec) { spec.Render = nil },
		"evidence-kind": func(spec *Spec) {
			spec.Evidence[0].Kind = ""
		},
		"evidence-trust": func(spec *Spec) {
			spec.Evidence[0].Trust = ""
		},
		"evidence-reason": func(spec *Spec) {
			spec.Evidence[0].Reason = ""
		},
		"evidence-anchor": func(spec *Spec) {
			spec.Evidence[0].Anchor = AnchorInvalid
		},
		"label-anchor": func(spec *Spec) {
			spec.Labels[0].Anchor = AnchorInvalid
		},
		"label-text": func(spec *Spec) {
			spec.Labels[0].Text = ""
		},
		"fact-surface": func(spec *Spec) {
			spec.Fact = Reference{Key: "value"}
		},
		"fact-key": func(spec *Spec) {
			spec.Fact = Reference{Surface: schema.SurfaceKindAxis}
		},
	}
	for missing, damage := range cases {
		spec := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
		damage(&spec)
		if entry, ok := New(spec); ok || entry != nil {
			t.Fatalf("spec without %s admitted", missing)
		}
	}
}

// TestDerivedTableProjectsEverySealedRow is the drift law: every derived
// lookup is a projection of the one sealed view, so a row added to the surface
// appears in all of them without a second table being edited.
func TestDerivedTableProjectsEverySealedRow(t *testing.T) {
	scratch := scratchSpec("lint.unused.upvalue", scratchFamilyLint)
	scratch.Lane, scratch.Observation, scratch.Fact = LaneStatic, member(scratchObservationValue), Reference{}
	entries := []*Entry{
		mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice)),
		mustEntry(t, scratch),
	}
	sealed, failure := sealSurfaces(t, entries, []schema.Key{"value"})
	if failure.Available() {
		t.Fatalf("scratch table rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	table, tableOK := NewTable(view)
	if !viewOK || !tableOK || !table.Available() {
		t.Fatal("sealed diagnostic view did not project a table")
	}
	if table.Count() != len(entries) {
		t.Fatalf("derived table holds %d of %d sealed rows", table.Count(), len(entries))
	}
	position, positionOK := table.At(1)
	if !positionOK || position.Code() != scratch.Code {
		t.Fatal("ordered lookup did not see the added row")
	}
	byCode, byCodeOK := table.ForCode(scratch.Code)
	if !byCodeOK || byCode != position {
		t.Fatal("code lookup did not see the added row")
	}
	byObservation, byObservationOK := table.ForStaticObservation(scratchObservationValue)
	if !byObservationOK || byObservation != position {
		t.Fatal("static observation lookup did not see the added row")
	}
	if _, unknown := table.ForCode("advice.absent"); unknown {
		t.Fatal("code lookup answered for an undeclared row")
	}
	if _, unknown := table.ForStaticObservation(scratchObservationBranch); unknown {
		t.Fatal("static observation lookup answered for a branch population")
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// publish the same codes at different tiers are two tables. A row's publication
// tier is read from its declared default severity, so moving the severity moves
// the tier and the digest with it.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	advisory := mustEntry(t, scratchSpec("advice.always_true_guard", scratchFamilyAdvice))
	if advisory.Tier() != TierAdvisory {
		t.Fatalf("scratch row publishes tier %d, want the advisory tier", advisory.Tier())
	}
	declared, failure := sealSurfaces(t, []*Entry{advisory}, []schema.Key{"value", "heap"})
	if failure.Available() {
		t.Fatalf("advisory row rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec := scratchSpec("advice.always_true_guard", scratchFamilyAdvice)
	spec.DefaultSeverity = SeverityError
	errorTier := mustEntry(t, spec)
	if errorTier.Tier() != TierError {
		t.Fatalf("shifted row publishes tier %d, want the error tier", errorTier.Tier())
	}
	shifted, failure := sealSurfaces(t, []*Entry{errorTier}, []schema.Key{"value", "heap"})
	if failure.Available() {
		t.Fatalf("error-tier row rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a row's declared publication tier left the table digest unchanged")
	}
}
