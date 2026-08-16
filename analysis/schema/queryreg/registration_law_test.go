package queryreg

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchEntry is a stand-in row for a sibling surface. The query
// surface resolves a subject by deriving the axis surface's identity for a key
// and asking the sealed view, so a scratch axis inventory proves the same laws
// the analyzer's own axis records do.
type scratchEntry struct{ key schema.Key }

func (entry scratchEntry) Key() schema.Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

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
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindQuery:
			builder.Register(NewSurface(registrations))
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"value", "effect"}})
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	_, failure := builder.Seal()
	return failure
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
	registrations := []*Registration{
		mustRegistration(t, summarySpec("value-summary")),
		mustRegistration(t, exactSpec("effect-exact")),
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
	// The declaration is a value: mutating what the caller handed in cannot
	// reach the sealed registration.
	spec := exactSpec("effect-exact")
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
	if schema.SurfaceKindAxis >= schema.SurfaceKindQuery {
		t.Fatalf("axis catalog ordinal %d does not precede query ordinal %d", schema.SurfaceKindAxis, schema.SurfaceKindQuery)
	}
	builder := schema.NewBuilder()
	builder.Register(NewSurface([]*Registration{mustRegistration(t, summarySpec("value-summary"))}))
	builder.Register(scratchSurface{kind: schema.SurfaceKindAxis, keys: []schema.Key{"value"}})
	_, failure := builder.Seal()
	if failure.Law != schema.LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("axis surface registered after the query surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestNewRegistrationRejectsIncompleteSpec states the constructor half: a spec
// that violates a law yields no registration at all.
func TestNewRegistrationRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*RegistrationSpec){
		"family":            func(spec *RegistrationSpec) { spec.Family = "" },
		"codec":             func(spec *RegistrationSpec) { spec.Codec = identity.ContentID{} },
		"fold":              func(spec *RegistrationSpec) { spec.Fold = FoldInvalid },
		"contract":          func(spec *RegistrationSpec) { spec.Contract = identity.ContentID{} },
		"subjects":          func(spec *RegistrationSpec) { spec.Subjects = nil },
		"unnamed subject":   func(spec *RegistrationSpec) { spec.Subjects = []schema.Key{""} },
		"repeated subject":  func(spec *RegistrationSpec) { spec.Subjects = []schema.Key{"value", "value"} },
		"catalog ordinal":   func(spec *RegistrationSpec) { spec.Fold = FoldGeneral + 1 },
		"catalog underflow": func(spec *RegistrationSpec) { spec.Fold = Fold(0) },
	}
	for name, damage := range cases {
		spec := summarySpec("value-summary")
		damage(&spec)
		if registration, ok := NewRegistration(spec); ok || registration != nil {
			t.Fatalf("spec with a rejected %s admitted", name)
		}
	}
}
