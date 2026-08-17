package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// TestLibraryTableSeals states that the authored contract kind inventory is
// admitted and sealed by the one declaration root, and that it projects back
// with the class split and the environment singularity the surface's laws
// promise: a consumer resolving a kind reads the declaration itself rather than
// a restatement of it.
func TestLibraryTableSeals(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindLibrary)
	if !viewOK {
		t.Fatal("sealed table holds no library contract surface")
	}
	// The published contract kind space is the two exported keys a loader
	// resolves an instance by. It is stated independently of the authored
	// inventory, so a kind declared under no published key, or a published key
	// no kind declares, is a verdict rather than a table that agrees with
	// itself.
	for _, kind := range []schema.Key{LibraryContractKind, EnvironmentContractKind} {
		if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindLibrary, kind)); !declared {
			t.Fatalf("published contract kind %q is declared by no sealed row", kind)
		}
	}
	if view.Count() != 2 {
		t.Fatalf("sealed library surface holds %d rows for 2 published contract kinds", view.Count())
	}
	table, tableOK := library.NewTable(view)
	if !tableOK {
		t.Fatal("sealed library contract surface did not project")
	}
	roles, rolesOK := SemanticRoles()
	specs, specsOK := librarySpecs(roles)
	if !rolesOK || !specsOK {
		t.Fatal("declared contract identities did not resolve")
	}
	libraries := table.Class(library.ClassLibrary)
	if len(libraries) != 1 || libraries[0].Key() != LibraryContractKind {
		t.Fatalf("library class projects %d kinds, want the one authored library kind", len(libraries))
	}
	environment, environmentOK := table.Environment()
	if !environmentOK || environment.Key() != EnvironmentContractKind {
		t.Fatalf("projection does not hold the one authored environment contract kind")
	}
	// Every authored member form carries the payload format identity it was
	// declared with, so the projection a loader reads is the declaration.
	for _, spec := range specs {
		row, rowOK := view.ByID(schema.NewEntryID(schema.SurfaceKindLibrary, spec.Key))
		entry, entryOK := row.(*library.Entry)
		if !rowOK || !entryOK {
			t.Fatalf("authored kind %q is not sealed as a library entry", spec.Key)
		}
		for _, member := range spec.Members {
			payload, resolved := entry.Payload(member.Form)
			if !resolved || payload != member.Payload {
				t.Fatalf("kind %q does not carry the authored payload identity of form %d", spec.Key, member.Form)
			}
		}
	}
}

// TestLibraryKindsDeclareEveryFormTheirClassOwes states the completeness the
// member-form law demands, over the authored rows rather than over a toy: the
// library kind carries the whole base algebra, the environment kind carries it
// and the four forms only it may own, and neither declares a form outside its
// class.
func TestLibraryKindsDeclareEveryFormTheirClassOwes(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, _ := sealed.Surface(schema.SurfaceKindLibrary)
	table, tableOK := library.NewTable(view)
	if !tableOK {
		t.Fatal("sealed library contract surface did not project")
	}
	environment, environmentOK := table.Environment()
	if !environmentOK {
		t.Fatal("projection holds no environment contract kind")
	}
	declared := table.Class(library.ClassLibrary)
	if len(declared) != 1 {
		t.Fatalf("library kinds=%d want 1", len(declared))
	}
	for class, entry := range map[library.Class]*library.Entry{
		library.ClassLibrary:     declared[0],
		library.ClassEnvironment: environment,
	} {
		for _, form := range class.Required() {
			if !entry.Declares(form) {
				t.Fatalf("kind %q does not declare required form %d", entry.Key(), form)
			}
			if _, resolved := entry.Payload(form); !resolved {
				t.Fatalf("kind %q declares form %d with no payload format identity", entry.Key(), form)
			}
		}
	}
	// The environment extension is the environment's alone: an individual
	// library never declares a shape whose only effect is on the global
	// environment.
	for _, member := range declared[0].Members() {
		if member.Form.Environment() {
			t.Fatalf("library contract kind declares environment form %d", member.Form)
		}
	}
}

// scratchEntry and scratchSurface stand in for a sibling surface. The root
// requires every catalog member to be populated, so the drift law below states
// itself against a complete table rather than a half registered one.
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

// sealLibraryKinds seals one contract kind inventory into an otherwise complete
// table. The catalog is walked rather than listed, so the surfaces the
// declaration root settles on do not change what this law asserts.
func sealLibraryKinds(t *testing.T, entries []*library.Entry) *schema.Schema {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind == schema.SurfaceKindLibrary {
			builder.Register(library.NewSurface(entries))
			continue
		}
		builder.Register(scratchSurface{kind: kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("library inventory rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	return sealed
}

// TestDeclaredKindsReachTheDerivedViews is the drift law of this inventory: a
// declared kind is folded into the table's digest and is reachable in the
// derived projection, so a consumer derived from either cannot fall behind the
// declaration.
func TestDeclaredKindsReachTheDerivedViews(t *testing.T) {
	roles, rolesOK := SemanticRoles()
	if !rolesOK {
		t.Fatal("semantic role vocabulary")
	}
	kinds, kindsOK := libraryKinds(roles)
	if !kindsOK {
		t.Fatal("authored contract kind inventory rejected at construction")
	}
	baseline := sealLibraryKinds(t, kinds)
	drifted := sealLibraryKinds(t, append(kinds[:len(kinds):len(kinds)], mustLibraryKind(t, driftSpec(t))))
	if baseline.Digest() == drifted.Digest() {
		t.Fatal("a declared contract kind left the table digest unchanged")
	}
	view, viewOK := drifted.Surface(schema.SurfaceKindLibrary)
	if !viewOK {
		t.Fatal("sealed table holds no library contract surface")
	}
	if _, declared := view.ByID(schema.NewEntryID(schema.SurfaceKindLibrary, driftSpec(t).Key)); !declared {
		t.Fatal("the drifted contract kind is absent from the sealed view")
	}
	table, tableOK := library.NewTable(view)
	if !tableOK {
		t.Fatal("drifted library contract surface did not project")
	}
	if table.Count() != len(kinds)+1 {
		t.Fatalf("projected kinds=%d want %d", table.Count(), len(kinds)+1)
	}
}

// driftRoles is the declared contract vocabulary extended with the two roles
// the drift kind below is declared under. The extension is a contribution, so
// the probe declares its own identities exactly as a domain would.
func driftRoles(t *testing.T) vocabulary.Roles {
	t.Helper()
	entries, entriesOK := structure.Collect(contractRoles(), vocabulary.RoleSpecs("contract-codec/library-drift", "contract-lawset/library-drift"))
	roles, rolesOK := vocabulary.NewRoles(entries)
	if !entriesOK || !rolesOK {
		t.Fatal("drift contract vocabulary did not resolve")
	}
	return roles
}

// driftSpec is one further library kind, declared only to state the drift law
// over a table that holds it.
func driftSpec(t *testing.T) library.Spec {
	t.Helper()
	roles := driftRoles(t)
	codec, codecOK := contractIdentity(roles, "contract-codec/library-drift")
	laws, lawsOK := contractIdentity(roles, "contract-lawset/library-drift")
	members, membersOK := contractMembers(roles, library.ClassLibrary)
	if !codecOK || !lawsOK || !membersOK {
		t.Fatal("drift contract kind did not resolve its declared identities")
	}
	return library.Spec{
		Key:        "library-drift",
		Class:      library.ClassLibrary,
		Codec:      library.Codec{Format: codec, Version: contractCodecVersion},
		Validation: library.LawSet{Resolution: library.ResolutionDeferred, Deferred: laws},
		Addressing: library.AddressingExportPath,
		Members:    members,
	}
}

func mustLibraryKind(t *testing.T, spec library.Spec) *library.Entry {
	t.Helper()
	entry, ok := library.New(spec)
	if !ok || entry == nil {
		t.Fatalf("contract kind %q rejected by construction", spec.Key)
	}
	return entry
}

// TestLibrarySurfaceSealsLastInTheCatalog states the phase law for this
// surface: a kind resolves the validation law set its instances are checked
// under against a surface sealed below it, so no surface follows the library
// surface for such a reference to name.
func TestLibrarySurfaceSealsLastInTheCatalog(t *testing.T) {
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		if kind > schema.SurfaceKindLibrary {
			t.Fatalf("surface ordinal %d follows the library ordinal %d", kind, schema.SurfaceKindLibrary)
		}
	}
}
