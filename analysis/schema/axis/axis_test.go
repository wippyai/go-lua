package axis

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

// TestAxisSurfaceSealsCompleteInventory is the baseline: a complete axis
// declaration is admitted, indexed, and sealed with no verdict.
func TestAxisSurfaceSealsCompleteInventory(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, scratchSpec("heap", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("complete axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// scratchForeignSurface contributes rows of a foreign record under this
// surface's own kind and states this surface's laws over them. It is how a
// contribution that is not an axis declaration reaches the axis surface through
// the public seal path.
type scratchForeignSurface struct{}

func (scratchForeignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindAxis }

func (scratchForeignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchRuleEntry{key: "scratch-foreign"}}
}

func (scratchForeignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface[scratchInputs]{}.Seal(view, sealed)
}

// TestAxisSurfaceRejectsAForeignRow states that the axis surface reads axis
// declarations and nothing else. A row of another record type carries none of
// the declared data every axis law is stated over, so it is rejected as the
// wrong shape rather than read as a partially declared axis.
func TestAxisSurfaceRejectsAForeignRow(t *testing.T) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindAxis {
			builder.Register(scratchForeignSurface{})
			continue
		}
		builder.Register(scratchRuleSurface{kind: kind})
	}
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a foreign row was admitted into the axis surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign row rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindAxis {
		t.Fatalf("shape verdict named surface %d, not the axis surface", failure.Contributor)
	}
}

// TestAxisIdentityIsThisSurfaceDerivation states that an axis carries this
// surface's own derivation of its key. An entry identity minted for another
// surface names another entry, so it may not travel here.
func TestAxisIdentityIsThisSurfaceDerivation(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.id = schema.NewEntryID(schema.SurfaceKindRule, template.key)
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawAxisIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisKeyIsUnique states that two axes cannot share one authored
// identity. The entry identity is derived from the key, so the declaration
// root rejects the duplicate before any axis law is reached.
func TestAxisKeyIsUnique(t *testing.T) {
	first := mustTemplate(t, scratchSpec("value", valueRole))
	second := mustTemplate(t, scratchSpec("value", heapRole))
	failure := sealTemplates(t, []*Template[scratchInputs]{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate axis key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisSemanticIsDeclared states that an axis resolves to one canonical
// engine identity in the vocabulary it is sealed against.
func TestAxisSemanticIsDeclared(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	template.semantic = absentRole
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawSemanticIdentity || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("axis without a canonical identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxisRequiredFieldsAreComplete states that every closure field of an axis
// entry is present. An axis whose bind thunk is missing cannot be bound, and
// the table says so at seal rather than at bind.
func TestAxisRequiredFieldsAreComplete(t *testing.T) {
	for _, missing := range []string{"semantic"} {
		template := mustTemplate(t, scratchSpec("value", valueRole))
		switch missing {
		case "semantic":
			template.semantic = ""
		}
		failure := sealTemplates(t, []*Template[scratchInputs]{template})
		if failure.Law != LawFieldComplete || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("axis without %s sealed: law=%d disposition=%s", missing, failure.Law, failure.Disposition)
		}
	}
}

// TestAxisMetadataIsComplete states the declared half of the same
// completeness: an axis identity alone is not a declaration.
func TestAxisMetadataIsComplete(t *testing.T) {
	for _, missing := range []string{"storage", "cardinality", "lifetime", "mutability", "concurrency"} {
		template := mustTemplate(t, scratchSpec("value", valueRole))
		switch missing {
		case "storage":
			template.storage = StorageInvalid
		case "cardinality":
			template.cardinality = CardinalityInvalid
		case "lifetime":
			template.lifetime = LifetimeInvalid
		case "mutability":
			template.mutability = MutabilityInvalid
		case "concurrency":
			template.concurrency = ConcurrencyInvalid
		}
		failure := sealTemplates(t, []*Template[scratchInputs]{template})
		if failure.Law != LawMetadataComplete || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("axis without %s sealed: law=%d disposition=%s", missing, failure.Law, failure.Disposition)
		}
	}
}

// TestAxisDependencyEdgesResolve states that a declared dependency names an
// axis in this table.
func TestAxisDependencyEdgesResolve(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Dependencies = []schema.Key{"absent"}
	template := mustTemplate(t, spec)
	failure := sealTemplates(t, []*Template[scratchInputs]{template})
	if failure.Law != LawDependencyResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved dependency sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	resolved := scratchSpec("heap", heapRole)
	resolved.Dependencies = []schema.Key{"value"}
	if failure = sealTemplates(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, resolved),
	}); failure.Available() {
		t.Fatalf("resolved dependency rejected: law=%d", failure.Law)
	}
}

// TestAxisSemanticIsUnique states that two axes cannot claim one canonical
// engine identity.
func TestAxisSemanticIsUnique(t *testing.T) {
	first := mustTemplate(t, scratchSpec("value", valueRole))
	second := mustTemplate(t, scratchSpec("heap", valueRole))
	failure := sealTemplates(t, []*Template[scratchInputs]{first, second})
	if failure.Law != LawSemanticUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared axis semantic sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestAxesBindBeforeRules states the root's phase law over the axis surface.
// The declaration catalog order is the bind phase order, so a table that
// registers the rule surface before the axis surface is rejected by the root.
func TestAxesBindBeforeRules(t *testing.T) {
	builder := schema.NewBuilder()
	builder.Register(scratchRuleSurface{kind: schema.SurfaceKindRule})
	builder.Register(NewSurface([]*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	}))
	_, failure := builder.Seal()
	if failure.Law != schema.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("rule surface registered before the axis surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestNewRejectsIncompleteSpec states the constructor half of completeness: a
// spec missing any required field yields no template at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*Spec[scratchInputs]){
		"key":     func(spec *Spec[scratchInputs]) { spec.Key = "" },
		"storage": func(spec *Spec[scratchInputs]) { spec.Storage = StorageInvalid },
		"cardinality": func(spec *Spec[scratchInputs]) {
			spec.Cardinality = CardinalityInvalid
		},
		"lifetime": func(spec *Spec[scratchInputs]) {
			spec.Lifetime = LifetimeInvalid
		},
		"mutability": func(spec *Spec[scratchInputs]) {
			spec.Mutability = MutabilityInvalid
		},
		"concurrency": func(spec *Spec[scratchInputs]) {
			spec.Concurrency = ConcurrencyInvalid
		},
		"semantic": func(spec *Spec[scratchInputs]) { spec.Semantic = "" },
		"self-edge": func(spec *Spec[scratchInputs]) {
			spec.Dependencies = []schema.Key{"value"}
		},
	}
	for missing, damage := range cases {
		spec := scratchSpec("value", valueRole)
		damage(&spec)
		if template, ok := New(spec); ok || template != nil {
			t.Fatalf("spec without %s admitted", missing)
		}
	}
}

// enginePublishedSpec is one axis whose column the engine fills. It declares a
// coordinate space, a writer principal and a published column, and no hot half
// at all: no cold fragment, no factor binding, and no algebra of one, because
// the pass that fills its column is not a factor lane.
func enginePublishedSpec(key, semantic schema.Key) Spec[scratchInputs] {
	return Spec[scratchInputs]{
		Key:         key,
		Storage:     StorageEngine,
		Cardinality: CardinalitySparse,
		Lifetime:    LifetimeProgram,
		Mutability:  MutabilityFrozen,
		Concurrency: ConcurrencyShared,
		Frame:       Frame{Outputs: []Output{{Key: key + "/facts", Writer: key}}},
		Semantic:    semantic,
	}
}

// TestEnginePublishedAxisSealsWithoutAHotHalf states that a non-factor
// principal is declarable. An execution-reachability pass publishes its column
// itself: there is no factor cell to bind and no rule lane to write it, so the
// surface admits an axis that declares the space, the writer and the column and
// stops there. Requiring a hot half of it would make the only way to declare a
// published fact population an empty factor binding.
func TestEnginePublishedAxisSealsWithoutAHotHalf(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
		mustTemplate(t, enginePublishedSpec("reachability", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("engine-published axis rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	engine := templates[1]
	if engine.Storage() != StorageEngine || engine.Storage().Bound() {
		t.Fatal("the engine-published axis reports a bound storage")
	}
	if engine.MountDeclared() {
		t.Fatal("the engine-published axis declares a mount")
	}
}

// TestAdoptProjectsOneEngineAlgebra states the single conversion law: the
// surface's ordinal view of an algebra is the engine spec the owner binds,
// projected, and a key outside the declared space is rejected rather than
// truncated into a neighbour.
func TestAdoptProjectsOneEngineAlgebra(t *testing.T) {
	type carrier uint32
	admitted := map[carrier]bool{0: true, 1: true}
	spec := CarrierAlgebra[carrier, uint64]{
		KeyEnd:      2,
		Lattice:     scratchLattice(),
		Default:     0,
		AdmitAt:     func(key carrier, value uint64) bool { return admitted[key] },
		Fingerprint: func(value uint64) uint64 { return value ^ 7 },
		Widen:       CarrierRank[carrier, uint64]{Width: 2, At: func(key carrier, value uint64, component int) uint64 { return uint64(key) + uint64(component) }},
	}
	algebra, ok := Adopt(spec)
	if !ok || !algebra.Available() {
		t.Fatal("complete engine spec was not projected")
	}
	if algebra.KeyEnd != 2 || algebra.Fingerprint(1) != spec.Fingerprint(1) || algebra.Default != spec.Default {
		t.Fatal("projection lost the declared algebra")
	}
	if !algebra.AdmitAt(1, 0) || algebra.AdmitAt(2, 0) {
		t.Fatal("projected admission does not fence the declared key space")
	}
	if algebra.Widen.Width != 2 || algebra.Widen.At(1, 0, 1) != 2 || algebra.Widen.At(2, 0, 1) != 0 {
		t.Fatal("projected widen rank does not fence the declared key space")
	}
	if !algebra.Narrow.Absent() || algebra.Narrow.Available() {
		t.Fatal("undeclared narrow rank was projected as declared")
	}
	spec.AdmitAt = nil
	if _, admitted := Adopt(spec); admitted {
		t.Fatal("engine spec without an admission predicate was projected")
	}
}

// scratchAuthority stands in for one domain's sealed Link authority. The
// surface is blind to it, so a scratch authority proves the same laws a real
// factor schema does.
type scratchAuthority struct{ mounts int }

// scratchRejection stands in for one domain's own rejection evidence.
type scratchRejection uint8

const (
	scratchRejectionNone scratchRejection = iota
	scratchRejectionInput
)

// mountedSpec is one axis declaration that seals its own Link authority. The
// counter records every invocation so the table's iteration law is stated over
// observed calls rather than over the record the calls produced.
func mountedSpec(key, semantic schema.Key, order *[]schema.Key, admit bool) Spec[scratchInputs] {
	spec := scratchSpec(key, semantic)
	spec.Mount = NewMount(func(context Mounting[scratchInputs]) (*scratchAuthority, scratchRejection, bool) {
		*order = append(*order, key)
		if !admit || !context.Inputs.ready {
			return nil, scratchRejectionInput, false
		}
		return &scratchAuthority{mounts: 1}, scratchRejectionNone, true
	})
	return spec
}

// TestMountedAxisSealsItsOwnAuthority states the mount hook's contract: the
// declared hook receives the composition's own record and its result is
// recovered at the type the owner sealed.
func TestMountedAxisSealsItsOwnAuthority(t *testing.T) {
	var order []schema.Key
	template := mustTemplate(t, mountedSpec("value", valueRole, &order, true))
	if !template.MountDeclared() {
		t.Fatalf("axis declaring a mount reports none")
	}
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if !ok || !authority.Available() || rejection.Available() {
		t.Fatalf("declared mount rejected an admissible record: ok=%v authority=%v rejection=%v", ok, authority.Available(), rejection.Available())
	}
	sealed, sealedOK := Payload[*scratchAuthority](authority)
	if !sealedOK || sealed == nil || sealed.mounts != 1 {
		t.Fatalf("mounted authority did not recover at its declared type")
	}
	if len(order) != 1 || order[0] != "value" {
		t.Fatalf("declared mount invoked %d times, want exactly once", len(order))
	}
}

// TestUndeclaredMountAdmitsEmpty states the admission law: an axis that seals
// no authority of its own mounts empty rather than failing. Its authority is
// supplied by another owner, and the phase must not read that as a rejection.
func TestUndeclaredMountAdmitsEmpty(t *testing.T) {
	template := mustTemplate(t, scratchSpec("value", valueRole))
	if template.MountDeclared() {
		t.Fatalf("axis declaring no mount reports one")
	}
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if !ok {
		t.Fatalf("axis declaring no mount rejected the phase")
	}
	if authority.Available() || rejection.Available() {
		t.Fatalf("axis declaring no mount produced a payload: authority=%v rejection=%v", authority.Available(), rejection.Available())
	}
}

// TestRejectedMountCarriesDomainEvidence states that a rejecting mount hands
// back its own domain evidence and no authority, so the composition reports the
// rejection the domain stated rather than a generic verdict.
func TestRejectedMountCarriesDomainEvidence(t *testing.T) {
	var order []schema.Key
	template := mustTemplate(t, mountedSpec("value", valueRole, &order, false))
	authority, rejection, ok := template.Mount(scratchInputs{ready: true})
	if ok || authority.Available() {
		t.Fatalf("rejecting mount published an authority")
	}
	evidence, evidenceOK := Payload[scratchRejection](rejection)
	if !evidenceOK || evidence != scratchRejectionInput {
		t.Fatalf("rejecting mount lost its domain evidence: ok=%v evidence=%v", evidenceOK, evidence)
	}
}

// TestMountPhaseRunsEveryDeclaredMountOnceInCatalogOrder states the generic
// iteration law: a table walk invokes each declared mount exactly once, in the
// catalog's own order, and passes over the axes that declare none.
func TestMountPhaseRunsEveryDeclaredMountOnceInCatalogOrder(t *testing.T) {
	var order []schema.Key
	templates := []*Template[scratchInputs]{
		mustTemplate(t, mountedSpec("heap", heapRole, &order, true)),
		mustTemplate(t, scratchSpec("pack", packRole)),
		mustTemplate(t, mountedSpec("value", valueRole, &order, true)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("complete mounted inventory rejected: %v", failure)
	}
	mounted := 0
	for _, template := range templates {
		authority, _, ok := template.Mount(scratchInputs{ready: true})
		if !ok {
			t.Fatalf("axis %q rejected the mount phase", template.Key())
		}
		if authority.Available() != template.MountDeclared() {
			t.Fatalf("axis %q published an authority it did not declare a mount for", template.Key())
		}
		if authority.Available() {
			mounted++
		}
	}
	if mounted != 2 {
		t.Fatalf("mount phase sealed %d authorities, want one per declared mount", mounted)
	}
	if len(order) != 2 || order[0] != "heap" || order[1] != "value" {
		t.Fatalf("mount phase ran %v, want the catalog order [heap value]", order)
	}
}

func entryContentBytes(t *testing.T, template *Template[scratchInputs]) string {
	t.Helper()
	var sink bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&sink, "analysis/axis-entry-content-law/v1", 1); err != nil {
		t.Fatalf("axis %q content stream: %v", template.Key(), err)
	}
	if err := template.EntryContent(&writer); err != nil {
		t.Fatalf("axis %q entry content: %v", template.Key(), err)
	}
	return sink.String()
}

// TestDeclaringAMountDoesNotMoveEntryContent states the content boundary: which
// owner seals an axis's Link authority is wiring, not declared content, so
// moving a domain onto its own mount leaves the declaration digest exactly
// where it was. Only a changed coordinate space may move it.
func TestDeclaringAMountDoesNotMoveEntryContent(t *testing.T) {
	var order []schema.Key
	plain := mustTemplate(t, scratchSpec("value", valueRole))
	mounting := mustTemplate(t, mountedSpec("value", valueRole, &order, true))
	if !mounting.MountDeclared() || plain.MountDeclared() {
		t.Fatalf("mount declaration fixture is not the pair the law is about")
	}
	if entryContentBytes(t, plain) != entryContentBytes(t, mounting) {
		t.Fatalf("declaring a mount moved the axis entry's content")
	}
	if len(order) != 0 {
		t.Fatalf("writing entry content invoked the mount hook")
	}
}

// TestNilTemplateRejectsMount keeps the surface total: an absent template is a
// rejection rather than a panic.
func TestNilTemplateRejectsMount(t *testing.T) {
	var template *Template[scratchInputs]
	if _, _, ok := template.Mount(scratchInputs{ready: true}); ok {
		t.Fatalf("absent template admitted a mount")
	}
	if template.MountDeclared() {
		t.Fatalf("absent template reports a declared mount")
	}
}

// dependentSpec is one axis declaration that seals over a peer's authority.
func dependentSpec(key, semantic schema.Key, dependencies ...schema.Key) Spec[scratchInputs] {
	spec := scratchSpec(key, semantic)
	spec.Dependencies = dependencies
	return spec
}

// TestDependencyOrderPlacesEveryAxisAfterItsDependencies states the derivation
// a dependency-respecting phase walks: an axis follows the axes it declared an
// edge to, whatever the catalog order was.
func TestDependencyOrderPlacesEveryAxisAfterItsDependencies(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, dependentSpec("value", valueRole, "heap")),
		mustTemplate(t, dependentSpec("pack", packRole)),
		mustTemplate(t, dependentSpec("heap", heapRole)),
	}
	if failure := sealTemplates(t, templates); failure.Available() {
		t.Fatalf("acyclic inventory rejected: %v", failure)
	}
	ordered, ok := DependencyOrder(templates)
	if !ok || len(ordered) != len(templates) {
		t.Fatalf("dependency order rejected an acyclic inventory: ok=%v placed=%d", ok, len(ordered))
	}
	positions := make(map[schema.Key]int, len(ordered))
	for index, template := range ordered {
		positions[template.Key()] = index
	}
	for _, template := range templates {
		for index := 0; index < template.DependencyCount(); index++ {
			dependency, _ := template.DependencyAt(index)
			if positions[dependency] >= positions[template.Key()] {
				t.Fatalf("axis %q is ordered before its dependency %q", template.Key(), dependency)
			}
		}
	}
	// The order is stable: axes no edge separates keep the catalog's own order.
	if positions["pack"] <= positions["heap"] && positions["heap"] < positions["value"] {
		return
	}
	t.Fatalf("dependency order did not keep the unconstrained catalog order: %v", positions)
}

// TestDeclaredCycleIsRejectedAtSeal states that a cycle is a declaration error
// rather than a walk that never begins: the table refuses to seal, and it names
// an axis the cycle blocked.
func TestDeclaredCycleIsRejectedAtSeal(t *testing.T) {
	templates := []*Template[scratchInputs]{
		mustTemplate(t, dependentSpec("value", valueRole, "heap")),
		mustTemplate(t, dependentSpec("heap", heapRole, "value")),
	}
	failure := sealTemplates(t, templates)
	if !failure.Available() || failure.Law != LawDependencyAcyclic {
		t.Fatalf("cyclic inventory sealed: %v", failure)
	}
	if ordered, ok := DependencyOrder(templates); ok || len(ordered) != 0 {
		t.Fatalf("dependency order admitted a cycle: ok=%v placed=%d", ok, len(ordered))
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same axes and declare different coordinate spaces are two tables.
// The storage an axis's facts live in is read by every consumer of the space,
// so moving it moves the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	})
	if failure.Available() {
		t.Fatalf("scratch axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec := scratchSpec("value", valueRole)
	spec.Storage = StorageStatic
	shifted, failure := sealTable(t, []*Template[scratchInputs]{mustTemplate(t, spec)})
	if failure.Available() {
		t.Fatalf("axis with a shifted storage rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("an axis's declared storage left the table digest unchanged")
	}
}

// TestTableDigestCoversSemanticIdentity is the identity half of the same drift
// law. An axis's canonical identity is the role it selects from the closed
// vocabulary, and every engine binding of the axis is made under that identity,
// so two catalogs whose axes name the same key and select different roles are
// two tables. The two inventories below differ in the selected role and in
// nothing else.
func TestTableDigestCoversSemanticIdentity(t *testing.T) {
	declared, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", valueRole)),
	})
	if failure.Available() {
		t.Fatalf("scratch axis inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	shifted, failure := sealTable(t, []*Template[scratchInputs]{
		mustTemplate(t, scratchSpec("value", heapRole)),
	})
	if failure.Available() {
		t.Fatalf("axis with a shifted semantic role rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("an axis's selected semantic role left the table digest unchanged")
	}
}

// TestMountedArtifactDoesNotCarryProgramArtifact states the mount-phase
// fence: contributors receive the sealed ingress snapshot, not the owner.
func TestMountedArtifactDoesNotCarryProgramArtifact(t *testing.T) {
	row := reflect.TypeOf(MountedArtifact{})
	for index := 0; index < row.NumField(); index++ {
		field := row.Field(index)
		if strings.Contains(field.Type.String(), "programartifact") {
			t.Fatalf("MountedArtifact.%s is %s", field.Name, field.Type)
		}
	}
	if _, ok := row.FieldByName("Snapshot"); !ok {
		t.Fatal("MountedArtifact has no Snapshot")
	}
}
