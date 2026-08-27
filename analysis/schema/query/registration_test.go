package query

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/authority"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"

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

func (contribution scratchSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
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
		"query/call-callee-set", "query-result/call-callee-set", "fold-contract/call-callee-set",
		"query/population/selected-point",
		"query/population/observation",
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
func sealTable(t *testing.T, registrations []*Registration) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	return sealSurface(t, NewSurface(registrations))
}

// sealSurface seals one query contribution into a complete declaration table.
// It is the same table sealTable builds, stated over the contribution rather
// than over the inventory, so a contribution that is not this package's own
// surface is sealed under exactly the laws above.
func sealSurface(t *testing.T, contribution seal.Surface) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	builder := seal.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindQuery:
			builder.Register(contribution)
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"value", "effect", "call"}})
		case schema.SurfaceKindStructure:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{
				PopulationSelectedPoint, PopulationObservation, ProjectionSummary, ProjectionExact,
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

func (foreignSurface) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

func summarySpec(family schema.Key) Spec {
	return Spec{
		Family:      family,
		Semantic:    "semantic/query/value-summary",
		Codec:       "semantic/query-result/value-summary",
		Fold:        FoldDistributive,
		Contract:    "semantic/fold-contract/value-summary",
		Subjects:    []schema.Key{"value"},
		Population:  PopulationSelectedPoint,
		Projection:  ProjectionSummary,
		Declaration: scratchDeclaration(family),
	}
}

func exactSpec(family schema.Key) Spec {
	return Spec{
		Family:      family,
		Semantic:    "semantic/query/effect-exact",
		Codec:       "semantic/query-result/effect-exact",
		Fold:        FoldGeneral,
		Contract:    "semantic/fold-contract/effect-exact",
		Subjects:    []schema.Key{"effect", "value"},
		Population:  PopulationSelectedPoint,
		Projection:  ProjectionExact,
		Declaration: scratchDeclaration(family),
	}
}

// scratchDeclaration is the minimal nonempty owner-independent authority a
// query registration carries in these surface laws. Q is deliberately a
// row-only relation: it has no columns, keys, denominators, or addressing, but
// still has the scope required by the relation declaration graph.
func scratchDeclaration(family schema.Key) authority.Declaration {
	relationToken, relationOK := identity.DeriveContentID("query/test/relation", []byte(family))
	scopeToken, scopeOK := identity.DeriveContentID("query/test/scope", []byte(family))
	if !relationOK || !scopeOK {
		panic("query test authority token derivation failed")
	}
	declaration, declarationOK := authority.NewDeclaration(
		[]authority.RelationSpec{{Name: "Q", Token: relationToken, Scope: "QScope"}},
		nil,
		nil,
		[]authority.ScopeSpec{{Name: "QScope", Token: scopeToken, Region: region.True()}},
		nil,
	)
	if !declarationOK {
		panic("query test authority declaration rejected")
	}
	return declaration
}

func mustRegistration(t *testing.T, spec Spec) *Registration {
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

// TestQueryRegistrationOwnsAnAuthority states the atomic attachment: a
// registration retains the catalog sealed from its declaration, and the
// catalog's owner token is the exact registration contract identity.
func TestQueryRegistrationOwnsAnAuthority(t *testing.T) {
	registration := mustRegistration(t, summarySpec("value-summary"))
	catalog, ok := registration.Authority()
	if !ok || !catalog.Available() || catalog.RelationCount() != 1 {
		t.Fatalf("query authority = available=%t relations=%d, want one sealed relation", ok, catalog.RelationCount())
	}
	relation, relationOK := catalog.RelationByName("Q")
	if !relationOK || !relation.Available() {
		t.Fatal("query authority lost its row-only Q relation")
	}
	wantOwner := authority.Owner{
		Entry: schema.EntryReference{Surface: schema.SurfaceKindQuery, Key: registration.Key()},
		Token: identity.ContentID(registration.EntryID()),
	}
	if catalog.Owner() != wantOwner {
		t.Fatalf("query authority owner = %#v, want %#v", catalog.Owner(), wantOwner)
	}
	var provider authority.Provider = registration
	provided, providedOK := provider.Authority()
	if !providedOK || provided.Digest() != catalog.Digest() {
		t.Fatal("registration did not expose its retained authority through authority.Provider")
	}
}

// TestNewRegistrationRejectsAbsentAndEmptyAuthority states that query
// construction has no absent/default authority path. The generic authority
// package permits an empty attachment for owners that need one; queries do
// not.
func TestNewRegistrationRejectsAbsentAndEmptyAuthority(t *testing.T) {
	absent := summarySpec("authority-absent")
	absent.Declaration = authority.Declaration{}
	if registration, ok := New(absent, scratchRoles(t)); ok || registration != nil {
		t.Fatal("query registration admitted an absent authority declaration")
	}
	emptyDeclaration, emptyOK := authority.NewDeclaration(nil, nil, nil, nil, nil)
	if !emptyOK || !emptyDeclaration.Available() {
		t.Fatal("authority fixture's valid-empty declaration was unavailable")
	}
	empty := summarySpec("authority-empty")
	empty.Declaration = emptyDeclaration
	if registration, ok := New(empty, scratchRoles(t)); ok || registration != nil {
		t.Fatal("query registration admitted a valid-empty authority declaration")
	}
}

// TestQueryAuthoritySealRejectsHostileAttachments states the query surface's
// authority laws independently: no catalog, no empty relation set, a foreign
// owner fence, or a catalog/declaration digest drift can cross the boundary.
func TestQueryAuthoritySealRejectsHostileAttachments(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Registration)
		law    schema.LawID
		kind   schema.Disposition
	}{
		"missing catalog": {
			mutate: func(registration *Registration) { registration.catalog = authority.Catalog{} },
			law:    LawAuthorityDeclared,
			kind:   schema.DispositionIncomplete,
		},
		"empty catalog": {
			mutate: func(registration *Registration) {
				declaration, declarationOK := authority.NewDeclaration(nil, nil, nil, nil, nil)
				if !declarationOK {
					panic("valid-empty authority fixture rejected")
				}
				catalog, catalogOK := declaration.Seal(registration.catalog.Owner())
				if !catalogOK {
					panic("empty authority fixture did not seal")
				}
				registration.catalog = catalog
			},
			law:  LawAuthorityNonEmpty,
			kind: schema.DispositionIncomplete,
		},
		"foreign owner": {
			mutate: func(registration *Registration) {
				foreignOwner, ownerOK := authority.NewOwner(
					schema.EntryReference{Surface: schema.SurfaceKindQuery, Key: "foreign-family"},
					scratchAuthorityID("foreign-owner"),
				)
				if !ownerOK {
					panic("foreign authority owner unavailable")
				}
				catalog, catalogOK := registration.declaration.Seal(foreignOwner)
				if !catalogOK {
					panic("foreign authority fixture did not seal")
				}
				registration.catalog = catalog
			},
			law:  LawAuthorityOwner,
			kind: schema.DispositionMalformed,
		},
		"mutated declaration": {
			mutate: func(registration *Registration) {
				registration.declaration = scratchDeclaration("authority-mutated")
			},
			law:  LawAuthorityDigest,
			kind: schema.DispositionMalformed,
		},
		"mutated catalog": {
			mutate: func(registration *Registration) {
				changed := scratchDeclaration("authority-catalog-mutated")
				catalog, catalogOK := changed.Seal(registration.catalog.Owner())
				if !catalogOK {
					panic("mutated authority fixture did not seal")
				}
				registration.catalog = catalog
			},
			law:  LawAuthorityDigest,
			kind: schema.DispositionMalformed,
		},
	}
	for name, test := range tests {
		registration := mustRegistration(t, summarySpec(schema.Key("authority-"+name)))
		test.mutate(registration)
		failure := sealRegistrations(t, []*Registration{registration})
		if failure.Law != test.law || failure.Disposition != test.kind {
			t.Fatalf("%s: law=%d disposition=%s, want law=%d disposition=%s", name, failure.Law, failure.Disposition, test.law, test.kind)
		}
	}
}

func scratchAuthorityID(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("query/test/authority", []byte(label))
	if !ok {
		panic("query test authority token derivation failed")
	}
	return value
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
	if failure.Law != seal.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
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
	builder := seal.NewBuilder()
	builder.Register(NewSurface([]*Registration{mustRegistration(t, summarySpec("value-summary"))}))
	builder.Register(scratchSurface{kind: schema.SurfaceKindAxis, keys: []schema.Key{"value"}})
	_, failure := builder.Seal()
	if failure.Law != seal.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("axis surface registered after the query surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestQueriesRequireASealedAxisSurface is the other half of the phase law,
// stated by this surface rather than by the root. Registration order is only
// strictly increasing, so a table may skip the axis surface entirely; a family
// that cannot reach the axis inventory cannot resolve one subject, and says so
// instead of sealing a read over nothing.
func TestQueriesRequireASealedAxisSurface(t *testing.T) {
	builder := seal.NewBuilder()
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
	type damaged = Spec
	cases := map[string]func(*damaged){
		"family":           func(spec *damaged) { spec.Family = "" },
		"authority absent": func(spec *damaged) { spec.Declaration = authority.Declaration{} },
		"authority empty": func(spec *damaged) {
			declaration, ok := authority.NewDeclaration(nil, nil, nil, nil, nil)
			if !ok {
				panic("valid-empty authority fixture rejected")
			}
			spec.Declaration = declaration
		},
		"semantic":         func(spec *damaged) { spec.Semantic = "semantic/query/absent" },
		"codec":            func(spec *damaged) { spec.Codec = "" },
		"fold":             func(spec *damaged) { spec.Fold = FoldInvalid },
		"contract":         func(spec *damaged) { spec.Contract = "semantic/fold-contract/absent" },
		"subjects":         func(spec *damaged) { spec.Subjects = nil },
		"unnamed subject":  func(spec *damaged) { spec.Subjects = []schema.Key{""} },
		"repeated subject": func(spec *damaged) { spec.Subjects = []schema.Key{"value", "value"} },
		"catalog ordinal":  func(spec *damaged) { spec.Fold = FoldGeneral + 1 },
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

// TestRestrictSubjectsRequiresEveryDeclaredKey states the narrowing law:
// a declared subject the pass produced no payload for leaves the view
// unavailable rather than partial.
func TestRestrictSubjectsRequiresEveryDeclaredKey(t *testing.T) {
	if _, ok := RestrictSubjects(NewSubjects(nil), []schema.Key{"effect"}); ok {
		t.Fatal("restrict admitted a subject the pass did not produce")
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

// TestObservationPopulationIsASealedPopulationRole states that an
// observation-only family is a normal sealed query declaration with a
// distinct population identity. Selection code can therefore exclude it by
// the population value rather than by restating a family name.
func TestObservationPopulationIsASealedPopulationRole(t *testing.T) {
	spec := exactSpec("call-callee-set")
	spec.Semantic = "semantic/query/call-callee-set"
	spec.Codec = "semantic/query-result/call-callee-set"
	spec.Contract = "semantic/fold-contract/call-callee-set"
	spec.Subjects = []schema.Key{"call"}
	spec.Population = PopulationObservation
	registration := mustRegistration(t, spec)
	if registration.Population() != PopulationObservation || registration.PopulationKind() != PopulationKindObservation {
		t.Fatalf("observation family population = %q, want %q", registration.Population(), PopulationObservation)
	}
	if failure := sealRegistrations(t, []*Registration{registration}); failure.Available() {
		t.Fatalf("observation-only family rejected by sealed population role: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestProducerEnvelopeIsTheOwnerCompatibilityWitness states the boundary
// contract consumed by observations: a complete query registration vouches
// for its resolved execution lane and the exact freezer identity it owns.
// Observation geometry is deliberately absent from this envelope; the query
// owner proves only the producer-side facts it can actually own.
func TestProducerEnvelopeIsTheOwnerCompatibilityWitness(t *testing.T) {
	registration := mustRegistration(t, summarySpec("value-summary"))
	envelope, ok := registration.ProducerEnvelope()
	if !ok || !envelope.Available() {
		t.Fatal("complete query registration did not issue a producer envelope")
	}
	freezer, freezerOK := scratchRoles(t).Key("semantic/query-result/value-summary")
	if !freezerOK || envelope.Population != PopulationKindSelectedPoint || envelope.Codec != freezer {
		t.Fatalf("producer envelope = %#v, want selected-point and the owner's freezer", envelope)
	}

	for name, damage := range map[string]func(*Registration){
		"population meaning": func(registration *Registration) {
			registration.populationKind = PopulationKindObservation
		},
		"population spelling": func(registration *Registration) {
			registration.population = PopulationObservation
		},
		"codec digest": func(registration *Registration) {
			registration.codec = identity.ContentID{}
		},
		"freezer identity": func(registration *Registration) {
			registration.freezer, _ = scratchRoles(t).Key("semantic/query-result/effect-exact")
		},
	} {
		broken := mustRegistration(t, summarySpec("value-summary"))
		damage(broken)
		if envelope, ok := broken.ProducerEnvelope(); ok || envelope.Available() {
			t.Fatalf("producer envelope survived nearest %s drift: %#v", name, envelope)
		}
	}
}
