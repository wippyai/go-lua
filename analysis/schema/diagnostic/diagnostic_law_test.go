package diagnostic

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
)

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
func scratchSpec(code Code, family Family) Spec {
	return Spec{
		Code:            code,
		Family:          family,
		DefaultSeverity: SeverityHint,
		Lane:            LaneBranch,
		Observation:     programartifact.DiagnosticObservationBranchCondition,
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
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindDiagnostic:
			builder.Register(NewSurface(entries))
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
		mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice)),
		mustEntry(t, scratchSpec("value.reference.unresolved", FamilyValue)),
	}
	if failure := sealEntries(t, entries); failure.Available() {
		t.Fatalf("complete diagnostic inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticCodeIsThisSurfaceDerivation states that a diagnostic carries
// this surface's own derivation of its code, so a row cannot travel under
// another surface's identity.
func TestDiagnosticCodeIsThisSurfaceDerivation(t *testing.T) {
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	entry.id = schema.NewEntryID(schema.SurfaceKindRule, schema.Key(entry.code))
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawCodeIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticCodeIsUnique states that two rows cannot publish one code. The
// entry identity is derived from the code, so the root rejects the duplicate.
func TestDiagnosticCodeIsUnique(t *testing.T) {
	first := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	second := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	failure := sealEntries(t, []*Entry{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate diagnostic code sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticFamilyIsDeclared states that a row names the family it is
// published under and that the published code carries that same family, so
// publication gating by family cannot disagree with the code a reader sees.
func TestDiagnosticFamilyIsDeclared(t *testing.T) {
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	entry.family = FamilyLint
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawFamilyDeclared || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("code published outside its declared family sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if _, ok := New(scratchSpec("advice.always_true_guard", FamilyInvalid)); ok {
		t.Fatal("row without a declared family admitted")
	}
	if _, ok := New(scratchSpec("always_true_guard", FamilyAdvice)); ok {
		t.Fatal("code without a family segment admitted")
	}
}

// TestDiagnosticTierFollowsDefaultSeverity states the advisory-tier law: the
// publication tier is derived from the declared default severity and is never
// declared beside it, so the two cannot drift.
func TestDiagnosticTierFollowsDefaultSeverity(t *testing.T) {
	advisory := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	if advisory.Tier() != TierAdvisory {
		t.Fatalf("hint default severity published tier %d", advisory.Tier())
	}
	spec := scratchSpec("value.reference.unresolved", FamilyValue)
	spec.DefaultSeverity = SeverityError
	if mustEntry(t, spec).Tier() != TierError {
		t.Fatal("error default severity did not publish the error tier")
	}
	damaged := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
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
		spec := scratchSpec("advice.always_true_guard", FamilyAdvice)
		spec.Message = damaged
		if _, ok := New(spec); ok {
			t.Fatalf("template %q admitted", damaged)
		}
		spec = scratchSpec("advice.always_true_guard", FamilyAdvice)
		spec.Evidence[0].Detail = damaged
		if _, ok := New(spec); ok {
			t.Fatalf("evidence template %q admitted", damaged)
		}
	}
	entry := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
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
	unread := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	unread.requirements = RequiresSubject | RequiresTarget
	failure := sealEntries(t, []*Entry{unread})
	if failure.Law != LawRequirementCovered || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("unread requirement sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	unrequired := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
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
	spec := scratchSpec("advice.redundant_claim", FamilyAdvice)
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
	spec := scratchSpec("advice.always_true_guard", FamilyAdvice)
	spec.Observation = programartifact.DiagnosticObservationInvalid
	if _, ok := New(spec); ok {
		t.Fatal("producing lane without an observation population admitted")
	}
	spec = scratchSpec("lint.unused.local", FamilyLint)
	spec.Lane = LaneDeclared
	spec.Fact = Reference{}
	if _, ok := New(spec); ok {
		t.Fatal("declared lane carrying an observation population admitted")
	}
	spec.Observation = programartifact.DiagnosticObservationInvalid
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
	first := scratchSpec("type.reference.unresolved", FamilyType)
	first.Lane, first.Observation, first.Fact = LaneStatic, programartifact.DiagnosticObservationTypeReferenceUnresolved, Reference{}
	second := scratchSpec("value.reference.unresolved", FamilyValue)
	second.Lane, second.Observation, second.Fact = LaneStatic, programartifact.DiagnosticObservationTypeReferenceUnresolved, Reference{}
	failure := sealEntries(t, []*Entry{mustEntry(t, first), mustEntry(t, second)})
	if failure.Law != LawObservationUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared static observation population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDiagnosticFactReferenceResolves states the cross-surface law: a row that
// names the declaration its subjects are decided by resolves that name against
// the same sealed table, and a solver-observed row must name one.
func TestDiagnosticFactReferenceResolves(t *testing.T) {
	absent := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	absent.fact = Reference{Surface: schema.SurfaceKindAxis, Key: "absent"}
	failure := sealEntries(t, []*Entry{absent})
	if failure.Law != LawFactResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved fact reference sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	ruled := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	ruled.fact = Reference{Surface: schema.SurfaceKindRule, Key: "value-source"}
	if failure = sealEntries(t, []*Entry{ruled}); failure.Available() {
		t.Fatalf("resolved rule reference rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// A reference upward names a table that is not sealed yet; the catalog
	// order is the dependency order, so it is malformed rather than deferred.
	upward := mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice))
	upward.fact = Reference{Surface: schema.SurfaceKindDiagnostic, Key: "advice.always_true_guard"}
	failure = sealEntries(t, []*Entry{upward})
	if failure.Law != LawFactResolves || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("upward fact reference sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	rootless := scratchSpec("advice.always_true_guard", FamilyAdvice)
	rootless.Fact = Reference{}
	if _, ok := New(rootless); ok {
		t.Fatal("solver-observed row without a fact reference admitted")
	}
}

// TestDiagnosticRenderPlanIsComplete states that a row declares the sections
// it publishes, in one order, without repeating a section.
func TestDiagnosticRenderPlanIsComplete(t *testing.T) {
	spec := scratchSpec("advice.always_true_guard", FamilyAdvice)
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
	spec = scratchSpec("advice.always_true_guard", FamilyAdvice)
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
		"code":     func(spec *Spec) { spec.Code = "" },
		"family":   func(spec *Spec) { spec.Family = FamilyInvalid },
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
		spec := scratchSpec("advice.always_true_guard", FamilyAdvice)
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
	scratch := scratchSpec("lint.unused.upvalue", FamilyLint)
	scratch.Lane, scratch.Observation, scratch.Fact = LaneStatic, programartifact.DiagnosticObservationValueReferenceUnresolved, Reference{}
	entries := []*Entry{
		mustEntry(t, scratchSpec("advice.always_true_guard", FamilyAdvice)),
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
	byObservation, byObservationOK := table.ForStaticObservation(programartifact.DiagnosticObservationValueReferenceUnresolved)
	if !byObservationOK || byObservation != position {
		t.Fatal("static observation lookup did not see the added row")
	}
	if _, unknown := table.ForCode("advice.absent"); unknown {
		t.Fatal("code lookup answered for an undeclared row")
	}
	if _, unknown := table.ForStaticObservation(programartifact.DiagnosticObservationBranchCondition); unknown {
		t.Fatal("static observation lookup answered for a branch population")
	}
}
