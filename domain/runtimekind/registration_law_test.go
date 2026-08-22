package runtimekind_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/runtimekind"
)

// The Lua runtime family vocabulary is declared once and projected everywhere
// else. These laws state that the sealed table is this domain's own closed
// vocabulary, family for family: a family that exists as a Kind and not as a
// declared row, or a row whose ordinal names a different family than the one
// it declares, is a rejected build rather than a silent mistranslation.

func sealedFamilies(t *testing.T) structure.Table {
	t.Helper()
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed compilation unavailable")
	}
	sealed, failure := composite.Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	return table
}

// TestSealedVocabularyIsTheClosedRuntimeKindSet states the declaration is the
// domain's whole vocabulary and nothing besides it: every valid Kind is
// declared at its own ordinal, and the sealed catalog holds no member the
// domain does not spell.
func TestSealedVocabularyIsTheClosedRuntimeKindSet(t *testing.T) {
	table := sealedFamilies(t)
	declared := table.Count(structure.CategoryRuntimeKind)
	if want := int(runtimekind.Count) - 1; declared != want {
		t.Fatalf("sealed runtime family vocabulary declares %d members, but the closed Kind vocabulary has %d", declared, want)
	}
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, ok := table.At(structure.CategoryRuntimeKind, uint16(kind))
		if !ok {
			t.Fatalf("runtime family %d names no member of the sealed vocabulary", kind)
		}
		family, familyOK := runtimekind.KindFor(entry)
		if !familyOK || family != kind {
			t.Fatalf("sealed member %q at ordinal %d declares family %d, not %d", entry.Key(), entry.Ordinal(), family, kind)
		}
		if !entry.Accepted() {
			t.Fatalf("sealed member %q is held back from the projection its vocabulary feeds", entry.Key())
		}
		if entry.Spelling() != kind.Spelling() || string(entry.Key()) != kind.Spelling() {
			t.Fatalf("sealed member %d spells key=%q rendering=%q, owner declared %q", kind, entry.Key(), entry.Spelling(), kind.Spelling())
		}
	}
}

// TestDeclaredFamilyNamesAreTheOneSpelling states that a row's key is the
// family's name, so a consumer that needs the name of a family reads it from
// the sealed table rather than from a list of its own.
func TestDeclaredFamilyNamesAreTheOneSpelling(t *testing.T) {
	table := sealedFamilies(t)
	seen := make(map[schema.Key]runtimekind.Kind, table.Count(structure.CategoryRuntimeKind))
	for kind := runtimekind.Invalid + 1; kind < runtimekind.Count; kind++ {
		entry, ok := table.At(structure.CategoryRuntimeKind, uint16(kind))
		if !ok {
			t.Fatalf("runtime family %d names no member of the sealed vocabulary", kind)
		}
		if !entry.Key().Available() {
			t.Fatalf("runtime family %d is declared without a name", kind)
		}
		if prior, duplicate := seen[entry.Key()]; duplicate {
			t.Fatalf("families %d and %d are both declared as %q", prior, kind, entry.Key())
		}
		seen[entry.Key()] = kind
	}
}

// TestForeignRowsAreNotRuntimeFamilies states the projection is exact in the
// other direction: a member of another structural vocabulary does not answer
// as a runtime family, so a consumer cannot read one catalog through the
// other.
func TestForeignRowsAreNotRuntimeFamilies(t *testing.T) {
	table := sealedFamilies(t)
	for _, category := range []structure.Category{structure.CategoryArm, structure.CategoryEvent, structure.CategoryOutcome} {
		for ordinal := 1; ordinal <= table.Count(category); ordinal++ {
			entry, ok := table.At(category, uint16(ordinal))
			if !ok {
				t.Fatalf("structural vocabulary %d holds no member at ordinal %d", category, ordinal)
			}
			if family, familyOK := runtimekind.KindFor(entry); familyOK {
				t.Fatalf("sealed member %q of vocabulary %d answered as runtime family %d", entry.Key(), category, family)
			}
		}
	}
}
