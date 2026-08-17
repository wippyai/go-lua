package query

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
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

func contractID(role string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(role)))
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

func summarySpec(family schema.Key) RegistrationSpec {
	return RegistrationSpec{
		Family:   family,
		Codec:    contractID("codec/" + string(family)),
		Fold:     FoldDistributive,
		Contract: contractID("fold-contract/" + string(family)),
		Subjects: []schema.Key{"value"},
	}
}

func exactSpec(family schema.Key) RegistrationSpec {
	return RegistrationSpec{
		Family:   family,
		Codec:    contractID("codec/" + string(family)),
		Fold:     FoldGeneral,
		Contract: contractID("fold-contract/" + string(family)),
		Subjects: []schema.Key{"effect", "value"},
	}
}

func mustRegistration(t *testing.T, spec RegistrationSpec) *Registration {
	t.Helper()
	registration, ok := NewRegistration(spec)
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
// that violates a law yields no registration at all.
func TestNewRegistrationRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*RegistrationSpec){
		"family":           func(spec *RegistrationSpec) { spec.Family = "" },
		"codec":            func(spec *RegistrationSpec) { spec.Codec = identity.ContentID{} },
		"fold":             func(spec *RegistrationSpec) { spec.Fold = FoldInvalid },
		"contract":         func(spec *RegistrationSpec) { spec.Contract = identity.ContentID{} },
		"subjects":         func(spec *RegistrationSpec) { spec.Subjects = nil },
		"unnamed subject":  func(spec *RegistrationSpec) { spec.Subjects = []schema.Key{""} },
		"repeated subject": func(spec *RegistrationSpec) { spec.Subjects = []schema.Key{"value", "value"} },
		"catalog ordinal":  func(spec *RegistrationSpec) { spec.Fold = FoldGeneral + 1 },
	}
	for name, damage := range cases {
		spec := summarySpec("value-summary")
		damage(&spec)
		if registration, ok := NewRegistration(spec); ok || registration != nil {
			t.Fatalf("spec with a rejected %s admitted", name)
		}
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
	shifted, failure := sealTable(t, []*Registration{mustRegistration(t, spec)})
	if failure.Available() {
		t.Fatalf("query family with a shifted fold class rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a query family's declared fold class left the table digest unchanged")
	}
}
