package schema

import "testing"

// scratchEntry is one anonymous row of a stand-in surface. The root reads only
// an entry's identity and its own admissibility verdict, so a row with a key is
// all any root law needs.
type scratchEntry struct{ key Key }

func (entry scratchEntry) Key() Key { return entry.key }

func (entry scratchEntry) EntryAvailable() bool { return entry.key.Available() }

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

// populated is one stand-in surface with a single row.
func populated(kind SurfaceKind) scratchSurface {
	return scratchSurface{kind: kind, entries: []Entry{scratchEntry{key: "scratch"}}}
}

// empty is one stand-in surface that declares nothing.
func empty(kind SurfaceKind) scratchSurface { return scratchSurface{kind: kind} }

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
