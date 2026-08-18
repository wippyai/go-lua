package query

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// scratchEntry is a stand-in row for a sibling surface. The query
// surface resolves a subject by deriving the axis surface's identity for a key
// and asking the sealed view, so a scratch axis inventory proves the same laws
// the analyzer's own axis records do.
type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(*framing.Writer) error { return nil }

type scratchSurface struct {
	kind schema.SurfaceKind
	keys []schema.Key
}

func (contribution scratchSurface) Kind() schema.SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []schema.Entry {
	keys := contribution.keys
	if len(keys) == 0 {
		keys = []schema.Key{"scratch"}
	}
	entries := make([]schema.Entry, len(keys))
	for index, key := range keys {
		entries[index] = scratchEntry{key: key}
	}
	return entries
}

func (contribution scratchSurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

// scratchRoles is the role vocabulary the scratch families below are admitted
// against. A family names its identities as roles, so a law over one needs a
// resolved vocabulary exactly as the analyzer's own table does.
func scratchRoles(t *testing.T) vocabulary.Roles {
	t.Helper()
	entries, entriesOK := structure.Collect(vocabulary.RoleSpecs(
		"query/value-summary", "query-result/value-summary", "fold-contract/value-summary",
		"query/effect-exact", "query-result/effect-exact", "fold-contract/effect-exact",
		"query/population/selected-point",
		"query/projection/summary",
		"query/projection/exact",
	))
	if !entriesOK {
		t.Fatal("scratch role inventory rejected by construction")
	}
	roles, ok := vocabulary.NewRoles(entries)
	if !ok {
		t.Fatal("scratch role vocabulary did not resolve")
	}
	return roles
}

// scratchContributor is a stand-in contributor: it declares the three hooks a
// family is answered by without opening a real slot, so a law over the
// declaration is stated without a schema builder behind it.
func scratchContributor[F, R any](spec *Spec[F, R]) {
	spec.Declare = func(Declaration) (F, bool) {
		var fragment F
		return fragment, true
	}
	spec.Bind = func(Binding[F]) bool { return true }
	spec.Recover = func(Sealed[F]) (R, bool) {
		var implementation R
		return implementation, true
	}
}

// sealRegistrations seals one query inventory into a complete declaration
// table. The catalog is walked rather than listed, so the surfaces the
// declaration root settles on do not change what these laws assert, and the
// axis surface a family resolves its subjects against carries a real inventory.
func sealRegistrations(t *testing.T, registrations []*Registration) schema.SealFailure {
	t.Helper()
	_, failure := sealTable(t, registrations)
	return failure
}

// sealTable is the same seal, read for the table it produces rather than for
// the verdict alone.
func sealTable(t *testing.T, registrations []*Registration) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealSurface(t, NewSurface(registrations))
}

// sealSurface seals one query contribution into a complete declaration table.
// It is the same table sealTable builds, stated over the contribution rather
// than over the inventory, so a contribution that is not this package's own
// surface is sealed under exactly the laws above.
func sealSurface(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindQuery:
			builder.Register(contribution)
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"value", "effect"}})
		case schema.SurfaceKindStructure:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{
				PopulationSelectedPoint, ProjectionSummary, ProjectionExact,
			}})
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	return builder.Seal()
}

// foreignSurface contributes a row that is not this surface's entry type, under
// this surface's own kind, and states this surface's laws over it.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindQuery }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "foreign"}}
}

func (foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// scratchFragment and scratchImplementation are the cold and sealed payload types of
// the scratch families. They carry nothing: what these laws state is the
// declaration, and the payloads only have to be recoverable at their own type.
type scratchFragment struct{}

type scratchImplementation struct{}

func summarySpec(family schema.Key) Spec[scratchFragment, scratchImplementation] {
	spec := Spec[scratchFragment, scratchImplementation]{
		Family:     family,
		Semantic:   "semantic/query/value-summary",
		Codec:      "semantic/query-result/value-summary",
		Fold:       FoldDistributive,
		Contract:   "semantic/fold-contract/value-summary",
		Subjects:   []schema.Key{"value"},
		Population: PopulationSelectedPoint,
		Projection: ProjectionSummary,
	}
	scratchContributor(&spec)
	return spec
}

func exactSpec(family schema.Key) Spec[scratchFragment, scratchImplementation] {
	spec := Spec[scratchFragment, scratchImplementation]{
		Family:     family,
		Semantic:   "semantic/query/effect-exact",
		Codec:      "semantic/query-result/effect-exact",
		Fold:       FoldGeneral,
		Contract:   "semantic/fold-contract/effect-exact",
		Subjects:   []schema.Key{"effect", "value"},
		Population: PopulationSelectedPoint,
		Projection: ProjectionExact,
	}
	scratchContributor(&spec)
	return spec
}

func mustRegistration(t *testing.T, spec Spec[scratchFragment, scratchImplementation]) *Registration {
	t.Helper()
	registration, ok := New(spec, scratchRoles(t))
	if !ok || registration == nil {
		t.Fatalf("scratch query family %q rejected by construction", spec.Family)
	}
	return registration
}

// TestQuerySurfaceSealsCompleteInventory is the baseline: a complete family
// inventory is admitted, indexed, and sealed with no verdict.
func TestQuerySurfaceSealsCompleteInventory(t *testing.T) {
	spec := exactSpec("effect-exact")
	registrations := []*Registration{
		mustRegistration(t, summarySpec("value-summary")),
		mustRegistration(t, spec),
	}
	if failure := sealRegistrations(t, registrations); failure.Available() {
		t.Fatalf("complete query inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	registration := registrations[1]
	if registration.Fold() != FoldGeneral || registration.SubjectCount() != 2 {
		t.Fatal("declared fold contract or subject membership lost")
	}
	if registration.Population() != PopulationSelectedPoint || registration.Projection() != ProjectionExact {
		t.Fatal("declared population or projection lost")
	}
	if subject, ok := registration.SubjectAt(0); !ok || subject != "effect" {
		t.Fatal("declared subject order lost")
	}
	// The declaration is a value: this registration was built from this very
	// spec, so rewriting the authored subject list now must not reach it. The
	// read-back half needs no mutation: a subject is handed back one key at a
	// time, and a key is a value.
	spec.Subjects[0] = "value"
	if subject, ok := registration.SubjectAt(0); !ok || subject != "effect" {
		t.Fatal("the sealed subject list aliases its authored spec")
	}
}

// TestQueryRegistrationIdentityIsThisSurfaceDerivation states that a family
// carries this surface's own derivation of its key.
func TestQueryRegistrationIdentityIsThisSurfaceDerivation(t *testing.T) {
	registration := mustRegistration(t, summarySpec("value-summary"))
	registration.id = schema.NewEntryID(schema.SurfaceKindAxis, registration.family)
	failure := sealRegistrations(t, []*Registration{registration})
	if failure.Law != LawRegistrationIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestQueryFamilyIsUnique states that two families cannot share one authored
// identity.
func TestQueryFamilyIsUnique(t *testing.T) {
	first := mustRegistration(t, summarySpec("value-summary"))
	second := mustRegistration(t, exactSpec("value-summary"))
	failure := sealRegistrations(t, []*Registration{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate query family sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestQueryCodecIsDeclaredAndUnique states that a family names the identity its
// results are frozen under, and that no two families publish under one.
func TestQueryCodecIsDeclaredAndUnique(t *testing.T) {
	registration := mustRegistration(t, summarySpec("value-summary"))
	registration.codec = identity.ContentID{}
	failure := sealRegistrations(t, []*Registration{registration})
	if failure.Law != LawCodecDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("family without a result codec sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	first := mustRegistration(t, summarySpec("value-summary"))
	second := mustRegistration(t, exactSpec("effect-exact"))
	second.codec = first.codec
	if failure = sealRegistrations(t, []*Registration{first, second}); failure.Law != LawCodecUnique ||
		failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared result codec sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Entry != first.ID() {
		t.Fatalf("verdict named entry %x, not the prior claimant", failure.Entry)
	}
}

// TestQueryFoldContractIsDeclared states that a fold claim travels with the
// contract that discharges it.
func TestQueryFoldContractIsDeclared(t *testing.T) {
	for name, damage := range map[string]func(*Registration){
		"fold":     func(registration *Registration) { registration.fold = FoldInvalid },
		"catalog":  func(registration *Registration) { registration.fold = FoldGeneral + 1 },
		"contract": func(registration *Registration) { registration.contract = identity.ContentID{} },
	} {
		registration := mustRegistration(t, summarySpec("value-summary"))
		damage(registration)
		failure := sealRegistrations(t, []*Registration{registration})
		if failure.Law != LawFoldDeclared || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("family without a declared %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestQuerySubjectsAreDeclaredAndResolve states that a family reads something,
// that it names each subject once, and that every subject is an axis in this
// table.
func TestQuerySubjectsAreDeclaredAndResolve(t *testing.T) {
	registration := mustRegistration(t, summarySpec("value-summary"))
	registration.subjects = nil
	failure := sealRegistrations(t, []*Registration{registration})
	if failure.Law != LawSubjectDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("family without a subject sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	unnamed := mustRegistration(t, summarySpec("value-summary"))
	unnamed.subjects = []schema.Key{""}
	if failure = sealRegistrations(t, []*Registration{unnamed}); failure.Law != LawSubjectDeclared ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unnamed subject sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	repeated := mustRegistration(t, exactSpec("effect-exact"))
	repeated.subjects[1] = repeated.subjects[0]
	if failure = sealRegistrations(t, []*Registration{repeated}); failure.Law != LawSubjectUnique ||
		failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("repeated subject sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	absent := mustRegistration(t, summarySpec("value-summary"))
	absent.subjects = []schema.Key{"absent"}
	if failure = sealRegistrations(t, []*Registration{absent}); failure.Law != LawSubjectResolves ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("subject over an undeclared axis sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestQueriesSealAfterAxes is the phase law. A family resolves its subjects
// against the axis inventory, so the axis surface is sealed below it and a
// table registered the other way round is rejected by the root.
func TestQueriesSealAfterAxes(t *testing.T) {
	builder := schema.NewBuilder()
	builder.Register(NewSurface([]*Registration{mustRegistration(t, summarySpec("value-summary"))}))
	builder.Register(scratchSurface{kind: schema.SurfaceKindAxis, keys: []schema.Key{"value"}})
	_, failure := builder.Seal()
	if failure.Law != schema.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("axis surface registered after the query surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestQueriesRequireASealedAxisSurface is the other half of the phase law,
// stated by this surface rather than by the root. Registration order is only
// strictly increasing, so a table may skip the axis surface entirely; a family
// that cannot reach the axis inventory cannot resolve one subject, and says so
// instead of sealing a read over nothing.
func TestQueriesRequireASealedAxisSurface(t *testing.T) {
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindAxis:
		case schema.SurfaceKindQuery:
			builder.Register(NewSurface([]*Registration{mustRegistration(t, summarySpec("value-summary"))}))
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a query surface sealed without the axis inventory it resolves against")
	}
	if failure.Law != LawAxisPhase || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("law=%d disposition=%s want axis-phase/incomplete", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindQuery {
		t.Fatalf("verdict attributed to surface %d, not the query surface", failure.Contributor)
	}
}

// TestQueryForeignRowIsRejected states the entry-shape law: this surface's laws
// are stated over its own record, so a row admitted under the query kind that
// is not a registration is rejected rather than read as one.
func TestQueryForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealSurface(t, foreignSurface{})
	if sealed != nil {
		t.Fatal("a foreign row was admitted into the query surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindQuery {
		t.Fatalf("verdict attributed to surface %d, not the query surface", failure.Contributor)
	}
}

// TestNewRegistrationRejectsIncompleteSpec states the constructor half: a spec
// that violates a law, names an identity the vocabulary does not resolve, or
// withholds one hook of its contributor yields no registration at all.
func TestNewRegistrationRejectsIncompleteSpec(t *testing.T) {
	type damaged = Spec[scratchFragment, scratchImplementation]
	cases := map[string]func(*damaged){
		"family":           func(spec *damaged) { spec.Family = "" },
		"semantic":         func(spec *damaged) { spec.Semantic = "semantic/query/absent" },
		"codec":            func(spec *damaged) { spec.Codec = "" },
		"fold":             func(spec *damaged) { spec.Fold = FoldInvalid },
		"contract":         func(spec *damaged) { spec.Contract = "semantic/fold-contract/absent" },
		"subjects":         func(spec *damaged) { spec.Subjects = nil },
		"unnamed subject":  func(spec *damaged) { spec.Subjects = []schema.Key{""} },
		"repeated subject": func(spec *damaged) { spec.Subjects = []schema.Key{"value", "value"} },
		"catalog ordinal":  func(spec *damaged) { spec.Fold = FoldGeneral + 1 },
		"declare hook":     func(spec *damaged) { spec.Declare = nil },
		"bind hook":        func(spec *damaged) { spec.Bind = nil },
		"recover hook":     func(spec *damaged) { spec.Recover = nil },
		"population":       func(spec *damaged) { spec.Population = "" },
		"projection":       func(spec *damaged) { spec.Projection = "" },
		"projection fold":  func(spec *damaged) { spec.Projection = ProjectionExact },
	}
	for name, damage := range cases {
		spec := summarySpec("value-summary")
		damage(&spec)
		if registration, ok := New(spec, scratchRoles(t)); ok || registration != nil {
			t.Fatalf("spec with a rejected %s admitted", name)
		}
	}
}

// TestQueryContributorIsDeclared is the surface's own half of the same law: a
// family that reaches the table without the contributor that answers it is
// refused at seal, so a withdrawn contributor is a rejected table rather than a
// family that seals and is then answered from a fallback.
func TestQueryContributorIsDeclared(t *testing.T) {
	for name, withdraw := range map[string]func(*Registration){
		"declare": func(registration *Registration) { registration.declare = nil },
		"bind":    func(registration *Registration) { registration.bind = nil },
		"recover": func(registration *Registration) { registration.recover = nil },
	} {
		registration := mustRegistration(t, summarySpec("value-summary"))
		withdraw(registration)
		failure := sealRegistrations(t, []*Registration{registration})
		if failure.Law != LawContributorDeclared || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("family without its %s hook sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
		if failure.Entry != registration.ID() {
			t.Fatalf("verdict named entry %x, not the family whose contributor was withdrawn", failure.Entry)
		}
	}
}

// TestQueryContributorRunsOnlyOverPresentSubjects states the narrowing law's
// closed half: a family's declared subjects are narrowed before its hooks run,
// so a pass that produced no payload for one of them runs no hook at all rather
// than running one over a coordinate space that is not there.
func TestQueryContributorRunsOnlyOverPresentSubjects(t *testing.T) {
	ran := false
	spec := exactSpec("effect-exact")
	spec.Declare = func(Declaration) (scratchFragment, bool) {
		ran = true
		return scratchFragment{}, true
	}
	registration, ok := New(spec, scratchRoles(t))
	if !ok {
		t.Fatal("scratch query family rejected by construction")
	}
	if _, declared := registration.Declare(engine.NewSchema(), NewSubjects(nil)); declared {
		t.Fatal("a family declared against a pass that produced none of its subjects")
	}
	if ran {
		t.Fatal("a contributor ran with no payload for a subject its family declared")
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same families and compose their partial results differently are two
// tables. A fold class decides whether a family may be answered from disjoint
// fragments, so moving one moves the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealTable(t, []*Registration{mustRegistration(t, summarySpec("value-summary"))})
	if failure.Available() {
		t.Fatalf("toy query family rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	spec := summarySpec("value-summary")
	spec.Fold = FoldGeneral
	spec.Projection = ProjectionExact
	shifted, failure := sealTable(t, []*Registration{mustRegistration(t, spec)})
	if failure.Available() {
		t.Fatalf("query family with a shifted fold class rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a query family's declared fold class left the table digest unchanged")
	}
}

// TestQueryPopulationAndProjectionAreDeclaredAndResolve states that a family
// names the Artifact geometry it is asked at and the read shape construction
// attaches it through, that both resolve against the structural vocabulary,
// and that the projection agrees with the family's fold class.
func TestQueryPopulationAndProjectionAreDeclaredAndResolve(t *testing.T) {
	missingPopulation := mustRegistration(t, summarySpec("value-summary"))
	missingPopulation.population = ""
	failure := sealRegistrations(t, []*Registration{missingPopulation})
	if failure.Law != LawPopulationDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("family without a population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	absentPopulation := mustRegistration(t, summarySpec("value-summary"))
	absentPopulation.population = "semantic/query/population/absent"
	if failure = sealRegistrations(t, []*Registration{absentPopulation}); failure.Law != LawPopulationResolves ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("undeclared population sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	missingProjection := mustRegistration(t, summarySpec("value-summary"))
	missingProjection.projection = ""
	if failure = sealRegistrations(t, []*Registration{missingProjection}); failure.Law != LawProjectionDeclared ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("family without a projection sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	absentProjection := mustRegistration(t, summarySpec("value-summary"))
	absentProjection.projection = "semantic/query/projection/absent"
	if failure = sealRegistrations(t, []*Registration{absentProjection}); failure.Law != LawProjectionResolves ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("undeclared projection sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	mismatched := mustRegistration(t, summarySpec("value-summary"))
	mismatched.projection = ProjectionExact
	if failure = sealRegistrations(t, []*Registration{mismatched}); failure.Law != LawProjectionFold ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("fold/projection mismatch sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestArtifactQueryPlanSourceDoesNotRestateFamilyNames is the construction
// half of the population/projection law: the plan walks issued families and
// must not restate domain family spellings.
func TestArtifactQueryPlanSourceDoesNotRestateFamilyNames(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("query source location")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "artifact_query_plan.go"))
	if err != nil {
		t.Fatalf("read artifact_query_plan.go: %v", err)
	}
	text := string(src)
	if !strings.Contains(text, "composite.QueryIssuance") {
		t.Fatal("artifact_query_plan.go does not walk composite.QueryIssuance")
	}
	for _, literal := range []string{`"value-summary"`, `"effect-exact"`} {
		if strings.Contains(text, literal) {
			t.Fatalf("artifact_query_plan.go restates query family %s", literal)
		}
	}
}
