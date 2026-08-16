package denominator

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

// scratchEntry is a stand-in row for a sibling surface. A denominator resolves
// an owner by deriving that surface's identity for the key it was handed and
// asking the sealed view, so a scratch inventory proves the same laws the
// analyzer's own records do.
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

func universeID(description string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(description)))
}

// sealEntries seals one denominator inventory into a complete declaration
// table. The catalog is walked rather than listed, so the surfaces the
// declaration root settles on do not change what these laws assert, and the two
// surfaces a denominator names as an owner carry real inventories.
func sealEntries(t *testing.T, entries []*Entry) schema.SealFailure {
	t.Helper()
	builder := schema.NewBuilder()
	for kind := schema.SurfaceKind(1); kind.Available(); kind++ {
		switch kind {
		case schema.SurfaceKindDenominator:
			builder.Register(NewSurface(entries))
		case schema.SurfaceKindAxis:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"container", "member"}})
		case schema.SurfaceKindComposite:
			builder.Register(scratchSurface{kind: kind, keys: []schema.Key{"containment"}})
		default:
			builder.Register(scratchSurface{kind: kind})
		}
	}
	_, failure := builder.Seal()
	return failure
}

func axisSpec(key schema.Key) Spec {
	return Spec{
		Key:      key,
		Owner:    Owner{Surface: schema.SurfaceKindAxis, Entry: "container"},
		Universe: universeID("universe/" + string(key)),
		Phase:    PhasePublication,
	}
}

func compositeSpec(key schema.Key) Spec {
	return Spec{
		Key:      key,
		Owner:    Owner{Surface: schema.SurfaceKindComposite, Entry: "containment"},
		Universe: universeID("universe/" + string(key)),
		Phase:    PhaseSeal,
	}
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok || entry == nil {
		t.Fatalf("scratch denominator %q rejected by construction", spec.Key)
	}
	return entry
}

// TestDenominatorSurfaceSealsCompleteInventory is the baseline: a complete
// denominator inventory is admitted, indexed, and sealed with no verdict.
func TestDenominatorSurfaceSealsCompleteInventory(t *testing.T) {
	entries := []*Entry{
		mustEntry(t, axisSpec("container-coordinates")),
		mustEntry(t, compositeSpec("containment-members")),
	}
	if failure := sealEntries(t, entries); failure.Available() {
		t.Fatalf("complete denominator inventory rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	entry := entries[0]
	if entry.Owner().Surface != schema.SurfaceKindAxis || entry.Owner().Entry != "container" {
		t.Fatal("declared owner lost")
	}
	if entry.Universe() != universeID("universe/container-coordinates") || entry.Phase() != PhasePublication {
		t.Fatal("declared universe or closure phase lost")
	}
}

// TestDenominatorIdentityIsThisSurfaceDerivation states that a denominator
// carries this surface's own derivation of its key.
func TestDenominatorIdentityIsThisSurfaceDerivation(t *testing.T) {
	entry := mustEntry(t, axisSpec("container-coordinates"))
	entry.id = schema.NewEntryID(schema.SurfaceKindAxis, entry.key)
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawDenominatorIdentity || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("foreign entry identity sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorKeyIsUnique states that two closed worlds cannot share one
// authored identity.
func TestDenominatorKeyIsUnique(t *testing.T) {
	first := mustEntry(t, axisSpec("container-coordinates"))
	second := mustEntry(t, compositeSpec("container-coordinates"))
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
		entry := mustEntry(t, axisSpec("container-coordinates"))
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
	entry := mustEntry(t, axisSpec("container-coordinates"))
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
	entry := mustEntry(t, axisSpec("container-coordinates"))
	entry.owner.Entry = "absent"
	failure := sealEntries(t, []*Entry{entry})
	if failure.Law != LawOwnerResolves || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("unresolved owner sealed: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	// The owner is resolved in the surface it names, not in any surface: an
	// axis key is not a composite key.
	crossed := mustEntry(t, axisSpec("container-coordinates"))
	crossed.owner.Surface = schema.SurfaceKindComposite
	if failure = sealEntries(t, []*Entry{crossed}); failure.Law != LawOwnerResolves ||
		failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("owner resolved against the wrong surface: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestDenominatorUniverseIsDeclared states that the set description is named.
func TestDenominatorUniverseIsDeclared(t *testing.T) {
	entry := mustEntry(t, axisSpec("container-coordinates"))
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
	first := mustEntry(t, axisSpec("container-coordinates"))
	second := mustEntry(t, compositeSpec("containment-members"))
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
	entry := mustEntry(t, axisSpec("container-coordinates"))
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

// TestNewRejectsIncompleteSpec states the constructor half: a spec that
// violates a law yields no entry at all.
func TestNewRejectsIncompleteSpec(t *testing.T) {
	cases := map[string]func(*Spec){
		"key":           func(spec *Spec) { spec.Key = "" },
		"owner surface": func(spec *Spec) { spec.Owner.Surface = schema.SurfaceKindInvalid },
		"owner entry":   func(spec *Spec) { spec.Owner.Entry = "" },
		"owner phase":   func(spec *Spec) { spec.Owner.Surface = schema.SurfaceKindQuery },
		"self owner":    func(spec *Spec) { spec.Owner.Surface = schema.SurfaceKindDenominator },
		"universe":      func(spec *Spec) { spec.Universe = identity.ContentID{} },
		"phase":         func(spec *Spec) { spec.Phase = PhaseInvalid },
	}
	for name, damage := range cases {
		spec := axisSpec("container-coordinates")
		damage(&spec)
		if entry, ok := New(spec); ok || entry != nil {
			t.Fatalf("spec with a rejected %s admitted", name)
		}
	}
}
