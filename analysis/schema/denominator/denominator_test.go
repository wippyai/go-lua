package denominator

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchEntry is a stand-in row for a sibling surface. A denominator resolves
// an owner by deriving that surface's identity for the key it was handed and
// asking the sealed view, so a scratch inventory proves the same laws the
// analyzer's own records do.
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

func universeID(description string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(description)))
}

// sealEntries seals one denominator inventory into a complete declaration
// table. The catalog is walked rather than listed, so the surfaces the
// declaration root settles on do not change what these laws assert, and the two
// surfaces a denominator names as an owner carry real inventories.
func sealEntries(t *testing.T, entries []*Entry) schema.SealFailure {
	t.Helper()
	_, failure := sealTable(t, entries)
	return failure
}

// sealTable is the same seal, read for the table it produces rather than for
// the verdict alone.
func sealTable(t *testing.T, entries []*Entry) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	return sealContribution(t, NewSurface(entries))
}

// sealContribution seals one arbitrary contribution under this surface's kind,
// so a law about what this surface accepts as a row is stated against the
// public seal path rather than against the unexported entry type alone.
func sealContribution(t *testing.T, contribution schema.Surface) (*schema.Schema, schema.SealFailure) {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindDenominator:
			builder.Register(contribution)
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"container", "member"}})
		case schema.SurfaceKindComposite:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"containment"}})
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	return builder.Seal()
}

// axisEntry derives one scratch coordinate world. The axis keys it is handed
// are the ones the surrounding scratch axis surface declares, so the owner a
// derived world names resolves in the table these laws are read against.
func axisEntry(t *testing.T, axis schema.Key) *Entry {
	t.Helper()
	entry, ok := Coordinate(axis, universeID("universe/"+string(axis)))
	if !ok || entry == nil {
		t.Fatalf("scratch coordinate world for axis %q rejected by derivation", axis)
	}
	return entry
}

// compositeEntry is a scratch closed world owned by a surface other than the
// axis surface and closing at seal rather than at publication. The surface
// states its laws over any owner sealed below it, so the inventories these
// laws are read against carry more than the one owner kind the analyzer's own
// derivation produces.
func compositeEntry(t *testing.T, axis schema.Key) *Entry {
	t.Helper()
	entry := axisEntry(t, axis)
	entry.owner = Owner{Surface: schema.SurfaceKindComposite, Entry: "containment"}
	entry.phase = PhaseSeal
	return entry
}

func relationSpec(key schema.Key, owner RelationOwner, form RelationForm, parents ...schema.EntryID) RelationSpec {
	return RelationSpec{Key: key, Owner: owner, Form: form, Parents: parents}
}

func mustRelation(t *testing.T, spec RelationSpec) *RelationEntry {
	t.Helper()
	entry, ok := NewRelation(spec)
	if !ok || entry == nil {
		t.Fatalf("scratch relation %q rejected by construction", spec.Key)
	}
	return entry
}

func sealRelations(t *testing.T, entries []*Entry, relations []*RelationEntry) schema.SealFailure {
	t.Helper()
	_, failure := sealContribution(t, NewSurface(entries, relations))
	return failure
}

// TestDenominatorSurfaceSealsCompleteInventory is the baseline: a complete
// denominator inventory is admitted, indexed, and sealed with no verdict.
func TestDenominatorSurfaceSealsCompleteInventory(t *testing.T) {
	entries := []*Entry{
		axisEntry(t, "container"),
		compositeEntry(t, "member"),
	}
	if failure := sealEntries(t, entries); failure.Available() {
		t.Fatalf("complete denominator inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry := entries[0]
	if entry.Owner().Surface != schema.SurfaceKindAxis || entry.Owner().Entry != "container" {
		t.Fatal("declared owner lost")
	}
	if entry.Universe() != universeID("universe/container") || entry.Phase() != PhasePublication {
		t.Fatal("declared universe or closure phase lost")
	}
}

// foreignSurface contributes a row that is not this surface's entry type,
// under this surface's kind, and states this surface's own seal over it.
type foreignSurface struct{}

func (foreignSurface) Kind() schema.SurfaceKind { return schema.SurfaceKindDenominator }

func (foreignSurface) Entries() []schema.Entry {
	return []schema.Entry{scratchEntry{key: "foreign"}}
}

func (contribution foreignSurface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	return surface{}.Seal(view, sealed)
}

// TestForeignRowIsRejected states the shape law: a denominator is read from a
// row this surface itself built, so a row that identifies one entry and is not
// one of this surface's declarations is rejected rather than skipped.
func TestForeignRowIsRejected(t *testing.T) {
	sealed, failure := sealContribution(t, foreignSurface{})
	if sealed != nil || !failure.Available() {
		t.Fatal("a foreign row was admitted into the denominator surface")
	}
	if failure.Law != LawEntryShape || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("law=%d disposition=%s want entry-shape/malformed", failure.Law, failure.Disposition)
	}
	if failure.Contributor != schema.SurfaceKindDenominator {
		t.Fatalf("contributor=%d want the denominator surface", failure.Contributor)
	}
}

// TestDenominatorIdentityIsThisSurfaceDerivation states that a denominator
// carries this surface's own derivation of its key.
func TestDenominatorIdentityIsThisSurfaceDerivation(t *testing.T) {
	entry := axisEntry(t, "container")
	entry.id = schema.NewEntryID(schema.SurfaceKindAxis, entry.key)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawDenominatorIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorKeyIsUnique states that two closed worlds cannot share one
// authored identity.
func TestDenominatorKeyIsUnique(t *testing.T) {
	first := axisEntry(t, "container")
	second := compositeEntry(t, "container")
	second.universe = universeID("universe/second-container")
	failure := sealEntries(t, []*Entry{first, second})
	if failure.Law != schema.LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate denominator key sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorOwnerIsDeclared states that a universe belongs to something.
func TestDenominatorOwnerIsDeclared(t *testing.T) {
	for name, damage := range map[string]func(*Entry){
		"surface": func(entry *Entry) { entry.owner.Surface = schema.SurfaceKindInvalid },
		"entry":   func(entry *Entry) { entry.owner.Entry = "" },
	} {
		entry := axisEntry(t, "container")
		damage(entry)
		failure := sealEntries(t, []*Entry{entry})
		if failure.Law != LawOwnerDeclared || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("denominator without an owner %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestDenominatorOwnerIsSealedBelow states the phase law: an owner is resolved
// against a surface already sealed under this one, so a reference upward names
// a table that does not exist yet and is rejected rather than deferred.
func TestDenominatorOwnerIsSealedBelow(t *testing.T) {
	entry := axisEntry(t, "container")
	entry.owner.Surface = schema.SurfaceKindQuery
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawOwnerPhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("owner above this surface sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry.owner.Surface = schema.SurfaceKindDenominator
	if failure = sealEntries(t, []*Entry{entry}); failure.Law != LawOwnerPhase ||
		failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("self-owned denominator sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorOwnerResolves states that the owning entry exists in the
// surface it names.
func TestDenominatorOwnerResolves(t *testing.T) {
	entry := axisEntry(t, "container")
	entry.owner.Entry = "absent"
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawOwnerResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved owner sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// The owner is resolved in the surface it names, not in any surface: an
	// axis key is not a composite key.
	crossed := axisEntry(t, "container")
	crossed.owner.Surface = schema.SurfaceKindComposite
	if failure = sealEntries(t, []*Entry{crossed}); failure.Law != LawOwnerResolves ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("owner resolved against the wrong surface: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorUniverseIsDeclared states that the set description is named.
func TestDenominatorUniverseIsDeclared(t *testing.T) {
	entry := axisEntry(t, "container")
	entry.universe = identity.ContentID{}
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawUniverseDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("denominator without a universe sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorUniverseIsUnique states that one universe description is one
// closed world: a totality claim may not depend on which name it was made
// under.
func TestDenominatorUniverseIsUnique(t *testing.T) {
	first := axisEntry(t, "container")
	second := compositeEntry(t, "member")
	second.universe = first.universe
	failure := sealEntries(t, []*Entry{first, second})
	if failure.Law != LawUniverseUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("shared universe sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Entry != first.ID() {
		t.Fatalf("verdict named entry %x, not the prior claimant", failure.Entry)
	}
}

// TestDenominatorPhaseIsDeclared states that a closed world says when it
// closes.
func TestDenominatorPhaseIsDeclared(t *testing.T) {
	entry := axisEntry(t, "container")
	entry.phase = PhaseInvalid
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawPhaseDeclared || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("denominator without a closure phase sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry.phase = PhasePublication + 1
	if failure = sealEntries(t, []*Entry{entry}); failure.Law != LawPhaseDeclared ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("denominator with an out of catalog phase sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestCoordinateIsDerivedFromTheAxisItDescribes states the derivation half of
// this surface: a coordinate world has no authored content beyond the axis it
// belongs to and the identity of the set description. The identity a verdict
// carries, the owning surface entry and the closure phase are this surface's
// own derivation of that axis, so no row can name a different owner, a
// non-existent closure point, or an owner at or above this surface.
func TestCoordinateIsDerivedFromTheAxisItDescribes(t *testing.T) {
	universe := universeID("universe/container")
	entry, ok := Coordinate("container", universe)
	if !ok || entry == nil {
		t.Fatal("a declared axis and set description yielded no coordinate world")
	}
	if entry.Key() != "coordinates/container" {
		t.Fatalf("coordinate world spelled %q, not the axis's own key under its coordinates", entry.Key())
	}
	if entry.ID() != schema.NewEntryID(schema.SurfaceKindDenominator, entry.Key()) {
		t.Fatal("coordinate world does not carry this surface's derivation of its key")
	}
	if entry.Owner() != (Owner{Surface: schema.SurfaceKindAxis, Entry: "container"}) {
		t.Fatal("coordinate world is not owned by the axis it describes")
	}
	if entry.Owner().Surface >= schema.SurfaceKindDenominator {
		t.Fatal("coordinate world names an owner at or above this surface")
	}
	if entry.Phase() != PhasePublication {
		t.Fatalf("coordinate world closes at phase %d, not at publication", entry.Phase())
	}
	if entry.Universe() != universe {
		t.Fatal("declared set description lost")
	}
}

// TestCoordinateRejectsUndeclaredSource states that the two inputs the
// derivation cannot supply itself are required: a world with no axis belongs
// to nothing, and a world with no set description quantifies over nothing.
func TestCoordinateRejectsUndeclaredSource(t *testing.T) {
	for name, derive := range map[string]func() (*Entry, bool){
		"axis":     func() (*Entry, bool) { return Coordinate("", universeID("universe/container")) },
		"universe": func() (*Entry, bool) { return Coordinate("container", identity.ContentID{}) },
	} {
		if entry, ok := derive(); ok || entry != nil {
			t.Fatalf("coordinate world with no %s admitted", name)
		}
	}
}

// TestTableDigestCoversDeclaredContent is the drift law of this surface: the
// digest is what a derived inventory is checked against, so two catalogs that
// name the same denominators and close their universes at different points are
// two tables. A phase is when a totality claim becomes answerable, so moving one
// moves the digest.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	declared, failure := sealTable(t, []*Entry{axisEntry(t, "container")})
	if failure.Available() {
		t.Fatalf("toy denominator rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	moved := axisEntry(t, "container")
	moved.phase = PhaseSeal
	shifted, failure := sealTable(t, []*Entry{moved})
	if failure.Available() {
		t.Fatalf("denominator with a shifted phase rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if declared.Digest() == shifted.Digest() {
		t.Fatal("a denominator's declared closure phase left the table digest unchanged")
	}
}

func TestUnifiedSurfaceSealsClosedWorldAndRelations(t *testing.T) {
	closed := axisEntry(t, "container")
	primary := mustRelation(t, relationSpec("relation/primary", RelationOwnerProgramSource, RelationFormAuthored))
	child := mustRelation(t, relationSpec("relation/child", RelationOwnerProgramSource, RelationFormSealDerived, primary.ID()))
	if failure := sealRelations(t, []*Entry{closed}, []*RelationEntry{primary, child}); failure.Available() {
		t.Fatalf("unified denominator rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if got := child.Parents(); len(got) != 1 || got[0] != primary.ID() {
		t.Fatal("relation parent identity was not retained")
	}
}

func TestRelationIdentityAndContentAreDeclaredByDenominatorSurface(t *testing.T) {
	left := mustRelation(t, relationSpec("relation/order", RelationOwnerProgramFlow, RelationFormAuthored))
	right := mustRelation(t, relationSpec("relation/order", RelationOwnerProgramFlow, RelationFormAuthored))
	if left.ID() != schema.NewEntryID(schema.SurfaceKindDenominator, left.Key()) || left.ID() != right.ID() {
		t.Fatal("relation identity is not the denominator derivation of its key")
	}
	first, failure := sealTable(t, []*Entry{axisEntry(t, "container")})
	if failure.Available() {
		t.Fatalf("baseline denominator rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	second, failure := sealContribution(t, NewSurface([]*Entry{axisEntry(t, "container")}, []*RelationEntry{left}))
	if failure.Available() {
		t.Fatalf("relation denominator rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if first.Digest() == second.Digest() {
		t.Fatal("relation declaration did not participate in the root digest")
	}
}

func TestRelationOwnerAndFormAreDeclared(t *testing.T) {
	for name, damage := range map[string]func(*RelationEntry){
		"owner": func(entry *RelationEntry) { entry.owner = RelationOwnerUnset },
		"form":  func(entry *RelationEntry) { entry.form = RelationFormUnset },
	} {
		relation := mustRelation(t, relationSpec(schema.Key("relation/declared/"+name), RelationOwnerProgramSource, RelationFormAuthored))
		damage(relation)
		failure := sealRelations(t, nil, []*RelationEntry{relation})
		want := LawRelationOwnerDeclared
		if name == "form" {
			want = LawRelationFormDeclared
		}
		if failure.Law != want || failure.Disposition != schema.DispositionIncomplete {
			t.Fatalf("relation without %s sealed: law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

func TestRelationParentsResolveAndAreUnique(t *testing.T) {
	parent := mustRelation(t, relationSpec("relation/parent", RelationOwnerProgramSource, RelationFormAuthored))
	missing := mustRelation(t, relationSpec("relation/missing", RelationOwnerProgramSource, RelationFormAuthored, schema.NewEntryID(schema.SurfaceKindDenominator, "relation/absent")))
	if failure := sealRelations(t, nil, []*RelationEntry{parent, missing}); failure.Law != LawRelationParentResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved relation parent sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	duplicate := mustRelation(t, relationSpec("relation/duplicate", RelationOwnerProgramSource, RelationFormAuthored, parent.ID(), parent.ID()))
	if failure := sealRelations(t, nil, []*RelationEntry{parent, duplicate}); failure.Law != LawRelationParentUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate relation parent sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

func TestRelationParentCycleLaws(t *testing.T) {
	left := mustRelation(t, relationSpec("relation/cycle/left", RelationOwnerProgramSource, RelationFormAuthored))
	right := mustRelation(t, relationSpec("relation/cycle/right", RelationOwnerProgramFlow, RelationFormAuthored))
	left.parents = []schema.EntryID{right.ID()}
	right.parents = []schema.EntryID{left.ID()}
	if failure := sealRelations(t, nil, []*RelationEntry{left, right}); failure.Law != LawRelationParentCycle || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("cross-owner relation SCC sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}

	localLeft := mustRelation(t, relationSpec("relation/local-cycle/left", RelationOwnerProgramSource, RelationFormAuthored))
	localRight := mustRelation(t, relationSpec("relation/local-cycle/right", RelationOwnerProgramSource, RelationFormAuthored))
	localLeft.parents = []schema.EntryID{localRight.ID()}
	localRight.parents = []schema.EntryID{localLeft.ID()}
	if failure := sealRelations(t, nil, []*RelationEntry{localLeft, localRight}); failure.Available() {
		t.Fatalf("same-owner relation recursion rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

func TestRelationOwnerCycleIsRejectedWithoutRelationSCC(t *testing.T) {
	first := mustRelation(t, relationSpec("relation/owner-cycle/first", RelationOwnerProgramSource, RelationFormAuthored))
	second := mustRelation(t, relationSpec("relation/owner-cycle/second", RelationOwnerProgramFlow, RelationFormAuthored))
	third := mustRelation(t, relationSpec("relation/owner-cycle/third", RelationOwnerProgramSource, RelationFormAuthored))
	first.parents = []schema.EntryID{second.ID()}
	second.parents = []schema.EntryID{third.ID()}
	if failure := sealRelations(t, nil, []*RelationEntry{first, second, third}); failure.Law != LawRelationOwnerCycle || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("cross-owner dependency cycle sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}
