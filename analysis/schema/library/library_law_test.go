package library

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

// scratchEntry is a stand-in row for a sibling surface. The declaration root
// requires every catalog member to be populated, so a library contract law is
// stated against a complete table rather than a half registered one.
type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

type scratchSurface struct{ kind schema.SurfaceKind }

func (contribution scratchSurface) Kind() schema.SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: scratchKey}}
}

func (contribution scratchSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// scratchKey is the authored key every stand-in surface declares, so a sealed
// validation reference has something real to resolve against.
const scratchKey = schema.Key("scratch")

// sealEntries seals one library contribution into an otherwise complete table.
// The catalog is walked rather than listed, so the surface order the
// declaration root settles on does not change what these laws assert.
func sealEntries(t *testing.T, entries []*Entry) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealSurface(t, NewSurface(entries))
}

func sealSurface(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindLibrary {
			builder.Register(contribution)
			continue
		}
		builder.Register(scratchSurface{kind: kind})
	}
	return builder.Seal()
}

func format(text string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(text)))
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok || entry == nil {
		t.Fatalf("contract kind %q rejected by construction", spec.Key)
	}
	return entry
}

// members declares one payload format identity per required form of a class,
// so a toy kind carries the complete vocabulary its class owes.
func members(class Class, salt string) []Member {
	forms := class.Required()
	out := make([]Member, 0, len(forms))
	for _, form := range forms {
		out = append(out, Member{Form: form, Payload: format(salt + "/" + formName(form))})
	}
	return out
}

// formName is the test's own spelling of a form, used only to derive distinct
// scratch payload identities.
func formName(form Form) string {
	switch form {
	case FormCallableSignature:
		return "callable-signature"
	case FormIntrinsicMarker:
		return "intrinsic-marker"
	case FormEffectLabel:
		return "effect-label"
	case FormMetatableEdge:
		return "metatable-edge"
	case FormExportValue:
		return "export-value"
	case FormResultProvenance:
		return "result-provenance"
	case FormResultRefinement:
		return "result-refinement"
	case FormSuspension:
		return "suspension"
	case FormRuleDelegation:
		return "rule-delegation"
	case FormExportType:
		return "export-type"
	case FormBootRoot:
		return "boot-root"
	case FormDeniedEntry:
		return "denied-entry"
	case FormEnvironmentSlot:
		return "environment-slot"
	case FormPrimitiveMetatable:
		return "primitive-metatable"
	case FormHostCapability:
		return "host-capability"
	default:
		return "invalid"
	}
}

func librarySpec(key schema.Key) Spec {
	return Spec{
		Key:        key,
		Class:      ClassLibrary,
		Codec:      Codec{Format: format("codec/" + string(key)), Version: 1},
		Validation: LawSet{Resolution: ResolutionSealed, Surface: schema.SurfaceKindRule, Entry: scratchKey},
		Addressing: AddressingExportPath,
		Members:    members(ClassLibrary, string(key)),
	}
}

func environmentSpec(key schema.Key) Spec {
	return Spec{
		Key:        key,
		Class:      ClassEnvironment,
		Codec:      Codec{Format: format("codec/" + string(key)), Version: 1},
		Validation: LawSet{Resolution: ResolutionDeferred, Deferred: format("lawset/" + string(key))},
		Addressing: AddressingExportPath,
		Members:    members(ClassEnvironment, string(key)),
	}
}

// toyContract is the minimal modelable declaration: one library contract kind
// and the one environment contract kind that specializes it.
func toyContract(t *testing.T) []*Entry {
	t.Helper()
	return []*Entry{
		mustEntry(t, librarySpec("contract/library")),
		mustEntry(t, environmentSpec("contract/environment")),
	}
}

func clone(entry *Entry) *Entry {
	copied := *entry
	copied.members = append([]Member(nil), entry.members...)
	return &copied
}

func mutate(entry *Entry, edit func(*Entry)) *Entry {
	copied := clone(entry)
	edit(copied)
	return copied
}

// rejects seals a contribution that must fail, and reports the law it failed
// under. Every law below is stated red-first: the declaration that violates it
// is written down, and the exact law ordinal is asserted.
func rejects(t *testing.T, name string, entries []*Entry, law schema.LawID, disposition schema.Disposition) {
	t.Helper()
	sealed, failure := sealEntries(t, entries)
	if sealed != nil || !failure.Available() {
		t.Fatalf("%s: admitted a declaration that violates law %d", name, law)
	}
	if failure.Law != law {
		t.Fatalf("%s: law=%d want=%d disposition=%s", name, failure.Law, law, failure.Disposition)
	}
	if failure.Disposition != disposition {
		t.Fatalf("%s: disposition=%s want=%s", name, failure.Disposition, disposition)
	}
	if failure.Contributor != schema.SurfaceKindLibrary {
		t.Fatalf("%s: contributor=%d want library surface", name, failure.Contributor)
	}
}

// TestLibrarySurfaceSealsTheToyContract is the modeling proof: a minimal
// library kind and the environment kind that specializes it are declared,
// sealed, and projected back with their classes and member vocabularies
// intact.
func TestLibrarySurfaceSealsTheToyContract(t *testing.T) {
	sealed, failure := sealEntries(t, toyContract(t))
	if failure.Available() {
		t.Fatalf("toy contract rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindLibrary)
	if !viewOK {
		t.Fatal("sealed table holds no library contract surface")
	}
	table, tableOK := NewTable(view)
	if !tableOK {
		t.Fatal("sealed library contract surface did not project")
	}
	if table.Count() != 2 {
		t.Fatalf("projected kinds=%d want 2", table.Count())
	}
	if got := len(table.Class(ClassLibrary)); got != 1 {
		t.Fatalf("library kinds=%d want 1", got)
	}
	environment, environmentOK := table.Environment()
	if !environmentOK || environment.Key() != "contract/environment" {
		t.Fatal("projection does not hold the one environment contract kind")
	}
	// The environment specializes the library algebra: every base form the
	// library kind declares is declared by the environment too.
	for _, form := range ClassLibrary.Required() {
		if !environment.Declares(form) {
			t.Fatalf("environment kind does not extend base form %s", formName(form))
		}
	}
}

// TestEnvironmentClassExtendsRatherThanReplacesTheLibraryAlgebra states the
// specialization relation on the declared vocabulary itself: the environment's
// required set strictly contains the library's, and the added forms are
// exactly the environment forms.
func TestEnvironmentClassExtendsRatherThanReplacesTheLibraryAlgebra(t *testing.T) {
	base := ClassLibrary.Required()
	extended := ClassEnvironment.Required()
	declared := make(map[Form]struct{}, len(extended))
	for _, form := range extended {
		declared[form] = struct{}{}
	}
	for _, form := range base {
		if form.Environment() {
			t.Fatalf("library class requires environment form %s", formName(form))
		}
		if _, present := declared[form]; !present {
			t.Fatalf("environment class drops base form %s", formName(form))
		}
	}
	if len(extended) <= len(base) {
		t.Fatalf("environment forms=%d base forms=%d; specialization adds nothing", len(extended), len(base))
	}
	for _, form := range extended {
		if !form.Available() {
			t.Fatalf("required form %d is not an available form", form)
		}
	}
}

// TestFormCatalogExcludesTheEnvironmentFloorSentinel keeps the split marker
// out of the declarable vocabulary: it separates the base algebra from the
// environment extension and is not itself a member shape.
func TestFormCatalogExcludesTheEnvironmentFloorSentinel(t *testing.T) {
	if formEnvironmentFloor.Available() {
		t.Fatal("the environment floor sentinel is declarable as a member form")
	}
	if FormInvalid.Available() || formLimit.Available() {
		t.Fatal("a catalog bound is declarable as a member form")
	}
	if !FormRuleDelegation.Available() || FormRuleDelegation.Environment() {
		t.Fatal("the last base form is misclassified")
	}
	if !FormBootRoot.Environment() || !FormHostCapability.Environment() {
		t.Fatal("an environment form is not classified as one")
	}
}

// TestFormCatalogPartitionsAtTheEnvironmentFloor pins which half of the
// member-form algebra each form belongs to. The partition is declared data: a
// form below the floor is one any contract owner may state about its own
// members, and a form at or above it is a shape whose only effect is on the
// global environment. A denial is owner-declared member data - a library states
// a member it declares and refuses to publish - so it belongs to the base half,
// and only the four shapes that reach outside a contract's own export graph
// belong to the environment's.
func TestFormCatalogPartitionsAtTheEnvironmentFloor(t *testing.T) {
	base := []Form{
		FormCallableSignature,
		FormIntrinsicMarker,
		FormEffectLabel,
		FormMetatableEdge,
		FormExportValue,
		FormResultProvenance,
		FormResultRefinement,
		FormSuspension,
		FormRuleDelegation,
		FormDeniedEntry,
		FormExportType,
	}
	environment := []Form{
		FormBootRoot,
		FormEnvironmentSlot,
		FormPrimitiveMetatable,
		FormHostCapability,
	}
	for _, form := range base {
		if !form.Available() {
			t.Fatalf("base form %s is outside the catalog", formName(form))
		}
		if form.Environment() {
			t.Fatalf("base form %s is partitioned as an environment form", formName(form))
		}
	}
	for _, form := range environment {
		if !form.Available() {
			t.Fatalf("environment form %s is outside the catalog", formName(form))
		}
		if !form.Environment() {
			t.Fatalf("environment form %s is partitioned as a base form", formName(form))
		}
	}
	if declared := len(ClassLibrary.Required()); declared != len(base) {
		t.Fatalf("the library class requires %d forms, the partition pins %d", declared, len(base))
	}
	if declared := len(ClassEnvironment.Required()); declared != len(base)+len(environment) {
		t.Fatalf("the environment class requires %d forms, the partition pins %d", declared, len(base)+len(environment))
	}
}

// TestCoverageMappingIsCarriedByTheDeclaredForms is the executable half of the
// absorption coverage mapping. Each row is a shape one of the four absorption
// targets expresses today, and the form the environment contract kind declares
// to carry it. A shape with no carrying form is a hole in the vocabulary.
func TestCoverageMappingIsCarriedByTheDeclaredForms(t *testing.T) {
	coverage := []struct {
		source string
		shape  string
		form   Form
	}{
		// analysis/program/target/catalogue_laws.go
		{"profile", "OperationSpec.Input / Outcomes ValuesSpec", FormCallableSignature},
		{"profile", "OperationSpec.Callbacks", FormCallableSignature},
		{"profile", "OperationSpec.Subedges", FormCallableSignature},
		{"profile", "OutcomeSpec.Kind terminal disposition", FormCallableSignature},
		{"profile", "OperationSpec.Effects RowSpec", FormEffectLabel},
		{"profile", "OutcomeSpec.ResultAliases", FormResultProvenance},
		{"profile", "OutcomeSpec.FreshResults", FormResultProvenance},
		{"profile", "OutcomeSpec.Produced + CaptureSpec", FormResultProvenance},
		{"profile", "OperationSpec.Suspensions", FormSuspension},
		{"profile", "Rule-owned conditional result selection", FormRuleDelegation},
		// analysis/program/target/catalogue_boot.go
		{"boot", "InitialRootSpec", FormBootRoot},
		{"boot", "InitialEntrySpec operation value", FormCallableSignature},
		{"boot", "InitialEntrySpec constant/root value + mutability", FormExportValue},
		{"boot", "InitialEntrySpec denied and absent value", FormDeniedEntry},
		{"boot", "InitialBindingSpec", FormEnvironmentSlot},
		{"boot", "InitialMetatableAttachmentSpec", FormPrimitiveMetatable},
		{"boot", "stringMetaRoot __index edge to stringRoot", FormMetatableEdge},
		// stdlib/*_manifest.go
		{"stdlib", "signature.Function.Type", FormCallableSignature},
		{"stdlib", "signature.Function.Effect labels", FormEffectLabel},
		{"stdlib", "capability descriptor status gate", FormHostCapability},
		{"stdlib", "ResultSlotCondition", FormResultRefinement},
		{"stdlib", "PositionalResultSlot", FormResultRefinement},
		// postcondition.NormalReturnRefinement is a label of the callable's own
		// effect row, so it is carried by the envelope this map already maps that
		// row to. It is named here as the effect label it is rather than as a
		// second member form: one fact, one statement, and a refinement published
		// both in the row and beside it would be one fact under two authorities
		// with nothing to keep them agreeing. The refinements the
		// FormResultRefinement rows above carry are the other kind - a predicate
		// over caller DATA, which no effect label states.
		{"stdlib", "postcondition.NormalReturnRefinement", FormEffectLabel},
		{"stdlib", "returns.Return callback transform", FormResultProvenance},
		{"stdlib", "bareGlobals", FormEnvironmentSlot},
		// analysis/module/signature/intrinsic.go
		{"signature", "signature.Intrinsic", FormIntrinsicMarker},
		// analysis/domain/type/stringlib/stringlib.go
		{"stringlib", "methods table entry", FormCallableSignature},
		{"stringlib", "colon-method receiver binding via __index", FormMetatableEdge},
		{"stringlib", "MatchReturnTypes / CaptureTypes over a literal pattern", FormRuleDelegation},
		// string.dump is modeled as a member that never returns, which is the
		// library refusing to publish it. The refusal is the library's own
		// statement about its own member, so the base algebra carries it.
		{"stringlib", "dump modeled as a member that never returns", FormDeniedEntry},
		// analysis/domain/type/ambient/types.go
		{"ambient", "ChannelGeneric named runtime type", FormExportType},
		{"ambient", "BuiltinTableTopMarker named runtime type", FormExportType},
	}
	sealed, failure := sealEntries(t, toyContract(t))
	if failure.Available() {
		t.Fatalf("toy contract rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, _ := sealed.Surface(schema.SurfaceKindLibrary)
	table, tableOK := NewTable(view)
	if !tableOK {
		t.Fatal("sealed library contract surface did not project")
	}
	environment, environmentOK := table.Environment()
	if !environmentOK {
		t.Fatal("projection holds no environment contract kind")
	}
	for _, row := range coverage {
		if !row.form.Available() {
			t.Fatalf("%s: %s maps to a form outside the catalog", row.source, row.shape)
		}
		if !environment.Declares(row.form) {
			t.Fatalf("%s: %s has no carrying form in the environment contract kind", row.source, row.shape)
		}
		if _, resolved := environment.Payload(row.form); !resolved {
			t.Fatalf("%s: %s maps to a form with no declared payload identity", row.source, row.shape)
		}
	}
	// The forms only the environment carries must be refused by a library
	// kind, so absorbing the boot ledger cannot be done by a library.
	library := table.Class(ClassLibrary)
	if len(library) != 1 {
		t.Fatalf("library kinds=%d want 1", len(library))
	}
	for _, form := range []Form{FormBootRoot, FormEnvironmentSlot, FormPrimitiveMetatable, FormHostCapability} {
		if library[0].Declares(form) {
			t.Fatalf("library contract kind declares environment form %s", formName(form))
		}
	}
	// A denial is not one of them. The boot ledger's denied entries are the
	// environment stating its own refusals, and a library states its own the
	// same way, so both classes carry the form.
	if !library[0].Declares(FormDeniedEntry) {
		t.Fatal("the library contract kind cannot state a member it declares and refuses to publish")
	}
}

// TestValueProvenanceRejectsNameAddressedKinds is the surface's governing law.
// A contract addressed by dotted global name is attached to a rebindable slot
// rather than to the value it describes, so it is rejected as the kind it is.
func TestValueProvenanceRejectsNameAddressedKinds(t *testing.T) {
	spec := librarySpec("contract/library")
	spec.Addressing = AddressingGlobalName
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a name-addressed contract kind was admitted at construction")
	}
	base := toyContract(t)
	named := mutate(base[0], func(entry *Entry) { entry.addressing = AddressingGlobalName })
	rejects(t, "name-addressed kind", []*Entry{named, base[1]}, LawAddressingProvenance, schema.DispositionMalformed)
	if AddressingGlobalName.ValueProvenance() {
		t.Fatal("name addressing claims value provenance")
	}
	if !AddressingExportPath.ValueProvenance() {
		t.Fatal("export-path addressing does not claim value provenance")
	}
}

func TestAddressingMustBeDeclared(t *testing.T) {
	spec := librarySpec("contract/library")
	spec.Addressing = AddressingInvalid
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a kind with no addressing form was admitted at construction")
	}
	base := toyContract(t)
	silent := mutate(base[0], func(entry *Entry) { entry.addressing = AddressingInvalid })
	rejects(t, "undeclared addressing", []*Entry{silent, base[1]}, LawAddressingDeclared, schema.DispositionIncomplete)
}

// foreignSurface contributes a row that is not this surface's entry type.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindLibrary }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "foreign"}}
}

func (contribution foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

func TestForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealSurface(t, foreignSurface{})
	if sealed != nil || !failure.Available() {
		t.Fatal("a foreign row was admitted into the library contract surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
}

func TestContractIdentityIsThisSurfacesDerivation(t *testing.T) {
	base := toyContract(t)
	borrowed := mutate(base[0], func(entry *Entry) {
		entry.id = schema.NewEntryID(schema.SurfaceKindRule, entry.key)
	})
	rejects(t, "borrowed identity", []*Entry{borrowed, base[1]}, LawContractIdentity, schema.DispositionMalformed)
}

func TestClassMustBeDeclared(t *testing.T) {
	spec := librarySpec("contract/library")
	spec.Class = ClassInvalid
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a classless contract kind was admitted at construction")
	}
	base := toyContract(t)
	classless := mutate(base[0], func(entry *Entry) { entry.class = ClassInvalid })
	rejects(t, "undeclared class", []*Entry{classless, base[1]}, LawClassDeclared, schema.DispositionIncomplete)
}

func TestCodecIdentityMustBeDeclaredAndVersioned(t *testing.T) {
	base := toyContract(t)

	spec := librarySpec("contract/library")
	spec.Codec.Format = identity.ContentID{}
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a codec-less contract kind was admitted at construction")
	}
	anonymous := mutate(base[0], func(entry *Entry) { entry.codec.Format = identity.ContentID{} })
	rejects(t, "undeclared codec", []*Entry{anonymous, base[1]}, LawCodecDeclared, schema.DispositionIncomplete)

	spec = librarySpec("contract/library")
	spec.Codec.Version = 0
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("an unversioned contract kind was admitted at construction")
	}
	unversioned := mutate(base[0], func(entry *Entry) { entry.codec.Version = 0 })
	rejects(t, "unversioned codec", []*Entry{unversioned, base[1]}, LawCodecVersioned, schema.DispositionIncomplete)
}

func TestCodecIdentityIsUniqueAcrossKinds(t *testing.T) {
	base := toyContract(t)
	second := mustEntry(t, librarySpec("contract/second"))
	shared := mutate(second, func(entry *Entry) { entry.codec.Format = base[0].codec.Format })
	rejects(t, "shared codec identity", []*Entry{base[0], shared, base[1]}, LawCodecUnique, schema.DispositionDuplicate)
}

func TestValidationReferenceMustStateItsResolution(t *testing.T) {
	spec := librarySpec("contract/library")
	spec.Validation = LawSet{}
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a kind with no validation reference was admitted at construction")
	}
	base := toyContract(t)
	silent := mutate(base[0], func(entry *Entry) { entry.validation = LawSet{} })
	rejects(t, "unstated resolution", []*Entry{silent, base[1]}, LawValidationDeclared, schema.DispositionIncomplete)
}

func TestSealedValidationReferenceMustNameASurfaceBelow(t *testing.T) {
	spec := librarySpec("contract/library")
	spec.Validation = LawSet{Resolution: ResolutionSealed, Surface: schema.SurfaceKindLibrary, Entry: scratchKey}
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("a self-referencing validation reference was admitted at construction")
	}
	base := toyContract(t)
	upward := mutate(base[0], func(entry *Entry) {
		entry.validation = LawSet{Resolution: ResolutionSealed, Surface: schema.SurfaceKindLibrary, Entry: scratchKey}
	})
	rejects(t, "upward validation reference", []*Entry{upward, base[1]}, LawValidationPhase, schema.DispositionMalformed)
}

func TestSealedValidationReferenceMustResolve(t *testing.T) {
	base := toyContract(t)
	dangling := mutate(base[0], func(entry *Entry) {
		entry.validation = LawSet{Resolution: ResolutionSealed, Surface: schema.SurfaceKindRule, Entry: "absent"}
	})
	rejects(t, "dangling validation reference", []*Entry{dangling, base[1]}, LawValidationResolves, schema.DispositionIncomplete)
}

// TestDeferredValidationReferenceIsHonest keeps deferral from being a way to
// half-resolve: a deferred reference carries a form-valid law-set identity and
// nothing that looks like a resolution against a surface.
func TestDeferredValidationReferenceIsHonest(t *testing.T) {
	base := toyContract(t)
	half := mutate(base[1], func(entry *Entry) {
		entry.validation = LawSet{
			Resolution: ResolutionDeferred,
			Deferred:   format("lawset/deferred"),
			Surface:    schema.SurfaceKindRule,
			Entry:      scratchKey,
		}
	})
	rejects(t, "half-resolved deferral", []*Entry{base[0], half}, LawValidationDeferred, schema.DispositionMalformed)

	empty := mutate(base[1], func(entry *Entry) {
		entry.validation = LawSet{Resolution: ResolutionDeferred}
	})
	rejects(t, "identity-less deferral", []*Entry{base[0], empty}, LawValidationDeferred, schema.DispositionMalformed)

	// An honestly deferred reference is admitted: the toy environment kind
	// declares one, and the baseline seal is green.
	if base[1].Validation().Resolution != ResolutionDeferred {
		t.Fatal("the toy environment kind does not exercise deferral")
	}
}

func TestMemberFormsMustBeDeclaredCompletely(t *testing.T) {
	base := toyContract(t)

	payloadless := mutate(base[0], func(entry *Entry) {
		entry.members[0].Payload = identity.ContentID{}
	})
	rejects(t, "payload-less member form", []*Entry{payloadless, base[1]}, LawMemberFormDeclared, schema.DispositionIncomplete)

	outside := mutate(base[0], func(entry *Entry) {
		entry.members[0].Form = formEnvironmentFloor
	})
	rejects(t, "sentinel member form", []*Entry{outside, base[1]}, LawMemberFormDeclared, schema.DispositionIncomplete)

	spec := librarySpec("contract/library")
	spec.Members = spec.Members[1:]
	if entry, ok := New(spec); ok || entry != nil {
		t.Fatal("an incomplete member vocabulary was admitted at construction")
	}
	short := mutate(base[0], func(entry *Entry) { entry.members = entry.members[1:] })
	rejects(t, "incomplete member vocabulary", []*Entry{short, base[1]}, LawMemberFormComplete, schema.DispositionIncomplete)
}

func TestMemberFormsAndPayloadsAreClaimedOnce(t *testing.T) {
	base := toyContract(t)

	repeated := mutate(base[0], func(entry *Entry) {
		entry.members = append(entry.members, Member{Form: entry.members[0].Form, Payload: format("duplicate/form")})
	})
	rejects(t, "repeated member form", []*Entry{repeated, base[1]}, LawMemberFormUnique, schema.DispositionDuplicate)

	aliased := mutate(base[0], func(entry *Entry) {
		entry.members[1].Payload = entry.members[0].Payload
	})
	rejects(t, "aliased payload identity", []*Entry{aliased, base[1]}, LawMemberFormUnique, schema.DispositionDuplicate)
}

// TestLibraryKindCannotDeclareAnEnvironmentForm is the tenancy law: an
// individual library never mutates the global environment, so it cannot even
// declare the shapes that would.
func TestLibraryKindCannotDeclareAnEnvironmentForm(t *testing.T) {
	base := toyContract(t)
	overreaching := mutate(base[0], func(entry *Entry) {
		entry.members = append(entry.members, Member{Form: FormEnvironmentSlot, Payload: format("overreach/slot")})
	})
	rejects(t, "library kind claiming an environment slot", []*Entry{overreaching, base[1]}, LawEnvironmentExclusive, schema.DispositionMalformed)

	boot := mutate(base[0], func(entry *Entry) {
		entry.members = append(entry.members, Member{Form: FormBootRoot, Payload: format("overreach/boot")})
	})
	rejects(t, "library kind claiming a boot root", []*Entry{boot, base[1]}, LawEnvironmentExclusive, schema.DispositionMalformed)
}

// TestSurfaceDeclaresOneEnvironmentAndAtLeastOneLibrary states the population
// law. Two environment kinds are two competing formats for the one initial
// environment, and a mount would have no ground to choose between them.
func TestSurfaceDeclaresOneEnvironmentAndAtLeastOneLibrary(t *testing.T) {
	base := toyContract(t)
	rejects(t, "no environment kind", []*Entry{base[0]}, LawClassPopulated, schema.DispositionIncomplete)
	rejects(t, "no library kind", []*Entry{base[1]}, LawClassPopulated, schema.DispositionIncomplete)

	second := mustEntry(t, environmentSpec("contract/environment-two"))
	rejects(t, "two environment kinds", []*Entry{base[0], base[1], second}, LawClassPopulated, schema.DispositionDuplicate)
}

// TestDeclaredKindDriftReachesTheDerivedView is the drift law: a kind added to
// the declaration appears in the sealed projection and changes the table's
// digest, so a consumer derived from the projection cannot fall behind the
// declaration.
func TestDeclaredKindDriftReachesTheDerivedView(t *testing.T) {
	baseline, failure := sealEntries(t, toyContract(t))
	if failure.Available() {
		t.Fatalf("toy contract rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	drifted := append(toyContract(t), mustEntry(t, librarySpec("contract/scratch")))
	sealed, failure := sealEntries(t, drifted)
	if failure.Available() {
		t.Fatalf("drifted contract rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if sealed.Digest() == baseline.Digest() {
		t.Fatal("a declared contract kind left the table digest unchanged")
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindLibrary)
	if !viewOK {
		t.Fatal("sealed table holds no library contract surface")
	}
	if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindLibrary, "contract/scratch")); !declared {
		t.Fatal("the scratch contract kind is absent from the sealed view")
	}
	table, tableOK := NewTable(view)
	if !tableOK {
		t.Fatal("drifted library contract surface did not project")
	}
	if table.Count() != 3 {
		t.Fatalf("projected kinds=%d want 3", table.Count())
	}
	found := false
	for _, entry := range table.Class(ClassLibrary) {
		if entry.Key() == "contract/scratch" {
			found = true
		}
	}
	if !found {
		t.Fatal("the scratch contract kind is absent from the derived class projection")
	}
}

// TestSealedDeclarationCannotBeRewrittenThroughItsMemberSlice keeps the sealed
// table immutable: a reader that mutates the slice it was handed must not
// reach the declaration behind it.
func TestSealedDeclarationCannotBeRewrittenThroughItsMemberSlice(t *testing.T) {
	entry := mustEntry(t, librarySpec("contract/library"))
	handed := entry.Members()
	if len(handed) == 0 {
		t.Fatal("the toy library kind declares no members")
	}
	handed[0] = Member{}
	if !entry.Declares(FormCallableSignature) {
		t.Fatal("mutating a handed member slice rewrote the sealed declaration")
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same contract kinds and declare different member vocabularies are
// two tables. A member form is the shape an instance is decoded as, so both the
// payload format one form is serialized in and the order the forms are declared
// in move the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	baseline, failure := sealEntries(t, toyContract(t))
	if failure.Available() {
		t.Fatalf("toy contract rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	repaid := toyContract(t)
	repaid[0] = mutate(repaid[0], func(entry *Entry) {
		entry.members[0].Payload = format("member/shifted-payload")
	})
	shifted, failure := sealEntries(t, repaid)
	if failure.Available() {
		t.Fatalf("contract with a shifted member payload rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if baseline.Digest() == shifted.Digest() {
		t.Fatal("a member form's declared payload format left the table digest unchanged")
	}
	reordered := toyContract(t)
	reordered[0] = mutate(reordered[0], func(entry *Entry) {
		entry.members[0], entry.members[1] = entry.members[1], entry.members[0]
	})
	permuted, failure := sealEntries(t, reordered)
	if failure.Available() {
		t.Fatalf("contract with a permuted member vocabulary rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if baseline.Digest() == permuted.Digest() {
		t.Fatal("the declared order of a kind's member forms left the table digest unchanged")
	}
}
