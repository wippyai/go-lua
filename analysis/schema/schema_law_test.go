package schema

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/internal/framing"
)

// scratchEntry is one anonymous row of a stand-in surface. The root reads an
// entry's identity, its own admissibility verdict, and its declared content, so
// a row with a key and one declared property is all any root law needs.
type scratchEntry struct {
	key      Key
	property uint64
}

func (entry scratchEntry) Key() Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry scratchEntry) EntryContent(content *framing.Writer) error {
	return content.Uint(entry.property)
}

// scratchSurface is a stand-in contributor with no laws of its own. It exists
// to let the root's laws be stated over a complete catalog without dragging in
// any surface's own package.
type scratchSurface struct {
	kind    SurfaceKind
	entries []Entry
}

func (contribution scratchSurface) Kind() SurfaceKind { return contribution.kind }

func (contribution scratchSurface) Entries() []Entry { return contribution.entries }

func (contribution scratchSurface) Seal(View, Sealed) SealFailure { return SealFailure{} }

// unavailableEntry is one row that identifies itself and then answers that it
// is not a usable declaration. It separates the root's identity law from its
// admissibility law: the identity derives, and the row still may not be
// indexed.
type unavailableEntry struct{ key Key }

func (entry unavailableEntry) Key() Key { return entry.key }

func (entry unavailableEntry) EntryAvailable() bool { return false }

func (entry unavailableEntry) EntryContent(*framing.Writer) error { return nil }

// refusingEntry is one admissible row whose declared content cannot be written.
// It is what a surface whose canonical bytes do not form looks like to the root.
type refusingEntry struct{ key Key }

func (entry refusingEntry) Key() Key { return entry.key }

func (entry refusingEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry refusingEntry) EntryContent(*framing.Writer) error { return errRefusedContent }

var errRefusedContent = errors.New("scratch entry declares no canonical content")

// populated is one stand-in surface with a single row.
func populated(kind SurfaceKind) scratchSurface {
	return scratchSurface{kind: kind, entries: []Entry{scratchEntry{key: "scratch"}}}
}

// holding is one stand-in surface carrying exactly the rows a law is about.
func holding(kind SurfaceKind, entries ...Entry) scratchSurface {
	return scratchSurface{kind: kind, entries: entries}
}

// sealWith seals a complete catalog in which one surface is replaced by the
// given contribution and every other surface declares the canonical row.
func sealWith(replaced SurfaceKind, contribution Surface) (*Schema, SealFailure) {
	builder := NewBuilder()
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		if kind == replaced {
			builder.Register(contribution)
			continue
		}
		builder.Register(populated(kind))
	}
	return builder.Seal()
}

// empty is one stand-in surface that declares nothing.
func empty(kind SurfaceKind) scratchSurface { return scratchSurface{kind: kind} }

// declaring is one stand-in surface whose single row carries a declared
// property.
func declaring(kind SurfaceKind, property uint64) scratchSurface {
	return scratchSurface{kind: kind, entries: []Entry{scratchEntry{key: "scratch", property: property}}}
}

// sealCatalog seals a complete catalog in which one surface declares the given
// property and every other surface declares the canonical row.
func sealCatalog(t *testing.T, declared SurfaceKind, property uint64) *Schema {
	t.Helper()
	builder := NewBuilder()
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		if kind == declared {
			builder.Register(declaring(kind, property))
			continue
		}
		builder.Register(populated(kind))
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("catalog declaring %d on surface %d rejected: law=%d disposition=%s", property, declared, failure.Law, failure.Disposition)
	}
	return sealed
}

// TestTableDigestCoversDeclaredContent is the content half of the root's fold:
// the digest is the drift guard every derived inventory is checked against, so
// two catalogs that name the same entries and declare different data are two
// tables, not one. It holds for every surface of the catalog, because the fold
// is the root's law rather than any surface's.
func TestTableDigestCoversDeclaredContent(t *testing.T) {
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		declared := sealCatalog(t, kind, 1)
		shifted := sealCatalog(t, kind, 2)
		if declared.Digest() == shifted.Digest() {
			t.Fatalf("surface %d declared a different property and the table digest did not move", kind)
		}
	}
}

// TestTableDigestIsStable states the other half: content is a function of the
// declaration alone, so sealing one catalog twice yields one digest.
func TestTableDigestIsStable(t *testing.T) {
	if sealCatalog(t, SurfaceKindAxis, 7).Digest() != sealCatalog(t, SurfaceKindAxis, 7).Digest() {
		t.Fatal("one catalog sealed twice yields two digests")
	}
}

// TestEmptyRegisteredSurfaceSeals states the population half of the root law
// split: how many rows a surface declares is that surface's own question, so a
// registered surface with an empty inventory seals, is reachable, and answers
// as the empty surface it is.
func TestEmptyRegisteredSurfaceSeals(t *testing.T) {
	for vacant := SurfaceKindInvalid + 1; vacant < surfaceKindLimit; vacant++ {
		builder := NewBuilder()
		for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
			if kind == vacant {
				builder.Register(empty(kind))
				continue
			}
			builder.Register(populated(kind))
		}
		sealed, failure := builder.Seal()
		if failure.Available() || sealed == nil || !sealed.Available() {
			t.Fatalf("table with an empty surface %d rejected: law=%d disposition=%s", vacant, failure.Law, failure.Disposition)
		}
		view, viewOK := sealed.Surface(vacant)
		if !viewOK || view.Kind() != vacant || view.Count() != 0 {
			t.Fatalf("empty surface %d did not seal as a reachable surface holding no rows", vacant)
		}
		if _, present := view.At(0); present {
			t.Fatalf("empty surface %d answered for a row it does not hold", vacant)
		}
	}
}

// TestUnregisteredSurfaceFailsCoverage states the coverage half: the catalog is
// the analyzer's declaration of what a complete table is, so a kind that is
// declared and never wired is an incomplete table, named by the kind that is
// missing.
func TestUnregisteredSurfaceFailsCoverage(t *testing.T) {
	for missing := SurfaceKindInvalid + 1; missing < surfaceKindLimit; missing++ {
		builder := NewBuilder()
		for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
			if kind == missing {
				continue
			}
			builder.Register(populated(kind))
		}
		sealed, failure := builder.Seal()
		if sealed != nil {
			t.Fatalf("table missing surface %d sealed", missing)
		}
		if failure.Law != LawSurfaceCoverage || failure.Disposition != DispositionIncomplete {
			t.Fatalf("table missing surface %d rejected under law=%d disposition=%s", missing, failure.Law, failure.Disposition)
		}
		if failure.Contributor != missing {
			t.Fatalf("coverage verdict named surface %d, not the missing surface %d", failure.Contributor, missing)
		}
	}
}

// TestCatalogRejectsWhatIsNotASurface states the root's catalog law. The
// contributor of an entry is its identity, so a registration that names no
// contributor is not a surface of this table and the builder holds the verdict
// until it is asked for one.
func TestCatalogRejectsWhatIsNotASurface(t *testing.T) {
	cases := map[string]func(*Builder){
		"absent contribution": func(builder *Builder) { builder.Register(nil) },
		"uncatalogued kind":   func(builder *Builder) { builder.Register(populated(SurfaceKindInvalid)) },
		"past the catalog":    func(builder *Builder) { builder.Register(populated(surfaceKindLimit)) },
	}
	for name, register := range cases {
		builder := NewBuilder()
		register(builder)
		for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
			builder.Register(populated(kind))
		}
		sealed, failure := builder.Seal()
		if sealed != nil {
			t.Fatalf("a catalog with an %s sealed", name)
		}
		if failure.Law != LawSurfaceCatalog || failure.Disposition != DispositionMalformed {
			t.Fatalf("%s rejected under law=%d disposition=%s", name, failure.Law, failure.Disposition)
		}
	}
}

// TestEmptyCatalogIsIncomplete is the other half of the same law: a builder
// that was handed nothing has no table to seal, and says so rather than
// producing an empty one.
func TestEmptyCatalogIsIncomplete(t *testing.T) {
	sealed, failure := NewBuilder().Seal()
	if sealed != nil {
		t.Fatal("a catalog with no registered surface sealed")
	}
	if failure.Law != LawSurfaceCatalog || failure.Disposition != DispositionIncomplete {
		t.Fatalf("empty catalog rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	var absent *Builder
	if sealed, failure = absent.Seal(); sealed != nil || failure.Law != LawSurfaceCatalog ||
		failure.Disposition != DispositionIncomplete {
		t.Fatalf("absent builder sealed a table: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
}

// TestSurfaceRegistersOnce states that one contributor holds one position in
// the catalog. A second registration under a kind already installed would give
// the table two inventories for one surface, so it is rejected at registration.
func TestSurfaceRegistersOnce(t *testing.T) {
	builder := NewBuilder()
	builder.Register(populated(SurfaceKindAxis))
	builder.Register(populated(SurfaceKindAxis))
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a catalog holding one surface twice sealed")
	}
	if failure.Law != LawSurfaceUnique || failure.Disposition != DispositionDuplicate {
		t.Fatalf("duplicate surface rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != SurfaceKindAxis {
		t.Fatalf("uniqueness verdict named surface %d, not the duplicated one", failure.Contributor)
	}
}

// TestSurfaceRegistersInCatalogOrder states the phase law at the root: the
// catalog order is the bind phase order, so a surface registered after one
// declared above it would seal a consumer before its producer.
func TestSurfaceRegistersInCatalogOrder(t *testing.T) {
	builder := NewBuilder()
	builder.Register(populated(SurfaceKindRule))
	builder.Register(populated(SurfaceKindAxis))
	sealed, failure := builder.Seal()
	if sealed != nil {
		t.Fatal("a catalog registered out of phase order sealed")
	}
	if failure.Law != LawSurfacePhase || failure.Disposition != DispositionMalformed {
		t.Fatalf("out-of-order registration rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Contributor != SurfaceKindAxis {
		t.Fatalf("phase verdict named surface %d, not the surface that arrived late", failure.Contributor)
	}
}

// resolvingSurface records, for its own kind, whether the structural
// vocabulary was already in reach when this surface was sealed. It is the
// observation the catalog-position law is stated over.
type resolvingSurface struct {
	scratchSurface
	reached map[SurfaceKind]bool
}

func (contribution resolvingSurface) Seal(_ View, sealed Sealed) SealFailure {
	view, ok := sealed.Surface(SurfaceKindStructure)
	contribution.reached[contribution.kind] = ok && view.Available() && sealed.Registered(SurfaceKindStructure)
	return SealFailure{}
}

// TestStructureResolvesFromEverySurfaceAboveIt is the catalog-position law for
// the structural vocabulary. It hosts the closed identity vocabularies the rest
// of the catalog names members of, and cross-surface resolution runs downward
// only, so it holds the first ordinal: every other surface is sealed with it
// already in reach, and it is sealed with nothing in reach at all.
func TestStructureResolvesFromEverySurfaceAboveIt(t *testing.T) {
	if SurfaceKindStructure != SurfaceKindInvalid+1 {
		t.Fatalf("the structural vocabulary declares ordinal %d, not the catalog's first", SurfaceKindStructure)
	}
	reached := make(map[SurfaceKind]bool, surfaceKindLimit)
	builder := NewBuilder()
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		builder.Register(resolvingSurface{scratchSurface: populated(kind), reached: reached})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("complete catalog rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	for kind := SurfaceKindInvalid + 1; kind < surfaceKindLimit; kind++ {
		if kind == SurfaceKindStructure {
			if reached[kind] {
				t.Fatal("the structural vocabulary resolved against itself while it was still being sealed")
			}
			continue
		}
		if !reached[kind] {
			t.Fatalf("surface %d seals without the structural vocabulary in reach", kind)
		}
	}
}

// TestRowIsPresentIdentifiedAdmissibleAndWritable states the four laws every
// entry of every surface is subject to, in the order the root indexes them. A
// row must exist, derive an identity, answer that it is a usable declaration,
// and write its canonical content; each failure is named by its own law rather
// than folded into one malformed-table verdict.
func TestRowIsPresentIdentifiedAdmissibleAndWritable(t *testing.T) {
	cases := []struct {
		name        string
		row         Entry
		law         LawID
		disposition Disposition
		named       bool
	}{
		{"absent row", nil, LawEntryPresent, DispositionMalformed, false},
		{"unidentified row", scratchEntry{}, LawEntryIdentity, DispositionMalformed, false},
		{"inadmissible row", unavailableEntry{key: "scratch"}, LawEntryAdmissible, DispositionMalformed, true},
		{"unwritable row", refusingEntry{key: "scratch"}, LawEntryContent, DispositionMalformed, true},
	}
	for _, test := range cases {
		sealed, failure := sealWith(SurfaceKindAxis, holding(SurfaceKindAxis, test.row))
		if sealed != nil {
			t.Fatalf("a catalog holding an %s sealed", test.name)
		}
		if failure.Law != test.law || failure.Disposition != test.disposition {
			t.Fatalf("%s rejected under law=%d disposition=%s", test.name, failure.Law, failure.Disposition)
		}
		if failure.Contributor != SurfaceKindAxis {
			t.Fatalf("%s verdict named surface %d, not the surface holding it", test.name, failure.Contributor)
		}
		named := failure.Entry.Available()
		if named != test.named {
			t.Fatalf("%s verdict entry named=%t, want %t", test.name, named, test.named)
		}
		if test.named && failure.Entry != NewEntryID(SurfaceKindAxis, test.row.Key()) {
			t.Fatalf("%s verdict named an identity this surface did not derive", test.name)
		}
	}
}

// TestRowIsUniqueWithinItsSurface states the last of the root's entry laws: an
// identity is derived from the authored key, so two rows spelled the same way
// are one entry declared twice.
func TestRowIsUniqueWithinItsSurface(t *testing.T) {
	sealed, failure := sealWith(SurfaceKindAxis, holding(SurfaceKindAxis,
		scratchEntry{key: "scratch"}, scratchEntry{key: "scratch", property: 1}))
	if sealed != nil {
		t.Fatal("a surface declaring one key twice sealed")
	}
	if failure.Law != LawEntryUnique || failure.Disposition != DispositionDuplicate {
		t.Fatalf("duplicate row rejected under law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	if failure.Entry != NewEntryID(SurfaceKindAxis, "scratch") {
		t.Fatal("duplicate verdict did not name the entry declared twice")
	}
}
