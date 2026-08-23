package seal

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/internal/framing"
)

type scratchEntry struct {
	key      schema.Key
	property uint64
	constant bool
	refs     schema.EntryReferences
	reads    *int
}

func (entry *scratchEntry) Key() schema.Key { return entry.key }

func (entry *scratchEntry) EntryAvailable() bool {
	return entry != nil && entry.key.Available()
}

func (entry *scratchEntry) EntryContent(content *framing.Writer) error {
	if entry.constant {
		return nil
	}
	return content.Uint(entry.property)
}

func (entry *scratchEntry) References() schema.EntryReferences {
	if entry.reads != nil {
		*entry.reads++
	}
	return entry.refs
}

type scratchSurface struct {
	kind       schema.SurfaceKind
	entries    []schema.Entry
	refs       schema.EntryReferences
	reads      *int
	mutateSeal func(*scratchSurface)
}

func (surface *scratchSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface *scratchSurface) Entries() []schema.Entry { return surface.entries }

func (surface *scratchSurface) References() schema.EntryReferences {
	if surface.reads != nil {
		*surface.reads++
	}
	return surface.refs
}

func (surface *scratchSurface) Seal(View, Sealed) schema.SealFailure {
	if surface.mutateSeal != nil {
		surface.mutateSeal(surface)
	}
	return schema.SealFailure{}
}

type refusingEntry struct{ key schema.Key }

func (entry refusingEntry) Key() schema.Key { return entry.key }

func (entry refusingEntry) EntryAvailable() bool { return entry.key.Available() }

func (refusingEntry) EntryContent(*framing.Writer) error {
	return errors.New("refusing canonical content")
}

func populated(kind schema.SurfaceKind) *scratchSurface {
	return &scratchSurface{
		kind:    kind,
		entries: []schema.Entry{&scratchEntry{key: "scratch"}},
	}
}

func completeBuilder(replaced schema.SurfaceKind, replacement Surface) *Builder {
	builder := NewBuilder()
	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		if kind == replaced {
			builder.Register(replacement)
			continue
		}
		builder.Register(populated(kind))
	}
	return builder
}

func completeTable(t *testing.T, replaced schema.SurfaceKind, replacement Surface) *Schema {
	t.Helper()
	table, failure := completeBuilder(replaced, replacement).Seal()
	if failure.Available() || table == nil {
		t.Fatalf("complete catalog rejected: contributor=%d entry=%x law=%d disposition=%s", failure.Contributor, failure.Entry, failure.Law, failure.Disposition)
	}
	return table
}

func TestRootEntryIdentityAndContentLaws(t *testing.T) {
	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		left := completeTable(t, kind, &scratchSurface{kind: kind, entries: []schema.Entry{&scratchEntry{key: "scratch", property: 1}}})
		right := completeTable(t, kind, &scratchSurface{kind: kind, entries: []schema.Entry{&scratchEntry{key: "scratch", property: 2}}})
		if left.Digest() == right.Digest() {
			t.Fatalf("surface %d changed declared content without changing digest", kind)
		}
	}

	_, failure := completeBuilder(schema.SurfaceKindAxis, &scratchSurface{
		kind:    schema.SurfaceKindAxis,
		entries: []schema.Entry{&scratchEntry{key: "scratch"}, &scratchEntry{key: "scratch"}},
	}).Seal()
	if failure.Law != LawEntryUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate row law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

func TestDigestSensitivityWithConstantEntryContent(t *testing.T) {
	makeTable := func(target schema.Key) *Schema {
		builder := NewBuilder()
		for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
			switch kind {
			case schema.SurfaceKindStructure:
				builder.Register(&scratchSurface{kind: kind, entries: []schema.Entry{
					&scratchEntry{key: "scratch", constant: true}, &scratchEntry{key: "other", constant: true},
				}})
			case schema.SurfaceKindAxis:
				builder.Register(&scratchSurface{kind: kind, entries: []schema.Entry{&scratchEntry{
					key: "axis", constant: true, refs: schema.EntryReferences{{Surface: schema.SurfaceKindStructure, Key: target}},
				}}})
			default:
				builder.Register(populated(kind))
			}
		}
		table, failure := builder.Seal()
		if failure.Available() || table == nil {
			t.Fatalf("constant reference table rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
		}
		return table
	}
	left := makeTable("scratch")
	right := makeTable("other")
	if left.Digest() == right.Digest() {
		t.Fatal("constant EntryContent made two declarations with different references share a digest")
	}
}

func TestCoverageAndPhaseLaws(t *testing.T) {
	builder := NewBuilder()
	builder.Register(populated(schema.SurfaceKindRule))
	builder.Register(populated(schema.SurfaceKindAxis))
	if table, failure := builder.Seal(); table != nil || failure.Law != LawSurfacePhase || failure.Disposition != schema.DispositionMalformed {
		t.Fatalf("out-of-order catalog result table=%v law=%d disposition=%s", table, failure.Law, failure.Disposition)
	}

	builder = NewBuilder()
	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		if kind != schema.SurfaceKindDenominator {
			builder.Register(populated(kind))
		}
	}
	if table, failure := builder.Seal(); table != nil || failure.Law != LawSurfaceCoverage || failure.Contributor != schema.SurfaceKindDenominator {
		t.Fatalf("missing surface result table=%v contributor=%d law=%d", table, failure.Contributor, failure.Law)
	}
}

func TestReferencesAllowUpwardCompositeToDenominator(t *testing.T) {
	composite := &scratchSurface{
		kind:    schema.SurfaceKindComposite,
		entries: []schema.Entry{&scratchEntry{key: "composite"}},
		refs:    schema.EntryReferences{{Surface: schema.SurfaceKindDenominator, Key: "scratch"}},
	}
	if table, failure := completeBuilder(schema.SurfaceKindComposite, composite).Seal(); failure.Available() || table == nil {
		t.Fatalf("upward composite reference was not resolved by complete pass: law=%d contributor=%d disposition=%s", failure.Law, failure.Contributor, failure.Disposition)
	}
}

func TestReferencesDistinguishMissingAndMalformed(t *testing.T) {
	cases := []struct {
		name        string
		reference   schema.EntryReference
		disposition schema.Disposition
	}{
		{name: "missing lower entry", reference: schema.EntryReference{Surface: schema.SurfaceKindStructure, Key: "absent"}, disposition: schema.DispositionIncomplete},
		{name: "missing surface key", reference: schema.EntryReference{Key: "scratch"}, disposition: schema.DispositionMalformed},
		{name: "invalid surface", reference: schema.EntryReference{Surface: schema.SurfaceKindInvalid, Key: "scratch"}, disposition: schema.DispositionMalformed},
		{name: "empty reference", reference: schema.EntryReference{}, disposition: schema.DispositionMalformed},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			referencing := &scratchSurface{
				kind:    schema.SurfaceKindAxis,
				entries: []schema.Entry{&scratchEntry{key: "axis"}},
				refs:    schema.EntryReferences{test.reference},
			}
			_, failure := completeBuilder(schema.SurfaceKindAxis, referencing).Seal()
			if failure.Law != LawReference || failure.Disposition != test.disposition {
				t.Fatalf("reference law=%d disposition=%s want %d/%s", failure.Law, failure.Disposition, LawReference, test.disposition)
			}
		})
	}
}

func TestSourceEntrySliceMutationAfterSealCannotRewriteView(t *testing.T) {
	first := &scratchEntry{key: "first"}
	second := &scratchEntry{key: "second"}
	entries := []schema.Entry{first}
	surface := &scratchSurface{kind: schema.SurfaceKindAxis, entries: entries}
	table := completeTable(t, schema.SurfaceKindAxis, surface)
	entries[0] = second
	view, ok := table.Surface(schema.SurfaceKindAxis)
	if !ok {
		t.Fatal("sealed axis view unavailable")
	}
	got, ok := view.At(0)
	if !ok || got != first {
		t.Fatalf("sealed view observed source slice mutation: got=%v want=%v", got, first)
	}
}

func TestViewOrdinalUsesSealedEntryIndex(t *testing.T) {
	first := &scratchEntry{key: "first"}
	second := &scratchEntry{key: "second"}
	table := completeTable(t, schema.SurfaceKindAxis, &scratchSurface{
		kind:    schema.SurfaceKindAxis,
		entries: []schema.Entry{first, second},
	})
	view, ok := table.Surface(schema.SurfaceKindAxis)
	if !ok {
		t.Fatal("sealed axis view unavailable")
	}
	firstOrdinal, firstOK := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, first.Key()))
	secondOrdinal, secondOK := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, second.Key()))
	if !firstOK || !secondOK || firstOrdinal != 0 || secondOrdinal != 1 {
		t.Fatalf("sealed ordinals = %d/%t, %d/%t", firstOrdinal, firstOK, secondOrdinal, secondOK)
	}
	if _, found := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, "missing")); found {
		t.Fatal("missing entry acquired an ordinal")
	}
}

func TestReferenceSnapshotIsExactOnceAndPreSeal(t *testing.T) {
	reads := 0
	entry := &scratchEntry{
		key:   "axis",
		refs:  schema.EntryReferences{{Surface: schema.SurfaceKindStructure, Key: "scratch"}},
		reads: &reads,
	}
	surface := &scratchSurface{
		kind:    schema.SurfaceKindAxis,
		entries: []schema.Entry{entry},
		mutateSeal: func(surface *scratchSurface) {
			// This is the adversarial TOCTOU mutation: the source reference is
			// changed after the snapshot and before the final full-table pass.
			entry.refs[0] = schema.EntryReference{Surface: schema.SurfaceKindInvalid, Key: "mutated"}
			surface.refs = schema.EntryReferences{{Surface: schema.SurfaceKindInvalid, Key: "mutated"}}
		},
		refs:  schema.EntryReferences{{Surface: schema.SurfaceKindStructure, Key: "scratch"}},
		reads: &reads,
	}
	if table, failure := completeBuilder(schema.SurfaceKindAxis, surface).Seal(); failure.Available() || table == nil {
		t.Fatalf("pre-seal reference snapshot was changed by adversarial mutation: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if reads != 2 {
		t.Fatalf("reference providers called %d times; want exactly once for surface and entry", reads)
	}
}

func TestCompleteResolverReachesAllSealedSurfaces(t *testing.T) {
	table := completeTable(t, schema.SurfaceKindObservation, populated(schema.SurfaceKindObservation))
	resolver := table.Resolver()
	if !resolver.Complete() {
		t.Fatal("schema resolver did not become complete")
	}
	for kind := schema.SurfaceKindInvalid + 1; kind < schema.SurfaceKindLimit; kind++ {
		if _, ok := resolver.Surface(kind); !ok {
			t.Fatalf("complete resolver cannot reach surface %d", kind)
		}
		if _, disposition := resolver.Resolve(kind, "scratch"); disposition != schema.DispositionAccepted {
			t.Fatalf("complete resolver failed surface %d with %s", kind, disposition)
		}
	}
}

func TestViewOrdinalUsesTheSealedDeclarationOrder(t *testing.T) {
	view := View{
		kind:    schema.SurfaceKindAxis,
		entries: []schema.Entry{&scratchEntry{key: "first"}, &scratchEntry{key: "second"}},
		index: map[schema.EntryID]int{
			schema.NewEntryID(schema.SurfaceKindAxis, "first"):  0,
			schema.NewEntryID(schema.SurfaceKindAxis, "second"): 1,
		},
	}
	if ordinal, ok := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, "second")); !ok || ordinal != 1 {
		t.Fatalf("second ordinal = %d/%t, want 1/true", ordinal, ok)
	}
	if _, ok := view.Ordinal(schema.NewEntryID(schema.SurfaceKindAxis, "missing")); ok {
		t.Fatal("missing entry acquired a dense ordinal")
	}
}
