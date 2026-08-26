package catalog

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// declarationLawRole is the semantic role this declaration table is written
// under. The structural surface declares the role and the axis names it, so
// the sealed table carries one vocabulary the rule plan resolves against.
var declarationLawRole = vocabulary.RoleKey("law/role")

type declarationLawEntry struct {
	key schema.Key
}

func (entry declarationLawEntry) Key() schema.Key { return entry.key }

func (entry declarationLawEntry) EntryAvailable() bool { return entry.key.Available() }

func (entry declarationLawEntry) EntryContent(_ *framing.Writer) error { return nil }

func (entry declarationLawEntry) Storage() axis.Storage { return axis.StorageEngine }

func (entry declarationLawEntry) OutputCount() int { return 1 }

func (entry declarationLawEntry) OutputAt(index int) (axis.Output, bool) {
	if index != 0 {
		return axis.Output{}, false
	}
	return axis.Output{Key: "law/output", Writer: "law/axis"}, true
}

func (entry declarationLawEntry) Coverage() axis.Coverage { return axis.CoverageTotal }

func (entry declarationLawEntry) Semantic() schema.Key { return declarationLawRole }

func (entry declarationLawEntry) Catalog() member.Catalog { return member.Catalog{} }

func (entry declarationLawEntry) Signature() axis.Signature { return axis.Signature{} }

type declarationLawSurface struct {
	kind    schema.SurfaceKind
	entries []schema.Entry
}

func (surface declarationLawSurface) Kind() schema.SurfaceKind { return surface.kind }

func (surface declarationLawSurface) Entries() []schema.Entry { return surface.entries }

func (surface declarationLawSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func newDeclarationLawInput(t *testing.T) *Declarations {
	t.Helper()
	role, roleOK := structure.New(structure.Spec{
		Key:      declarationLawRole,
		Category: structure.CategorySemanticRole,
		Ordinal:  1,
		Spelling: "law/role",
		Accepted: true,
	})
	if !roleOK {
		t.Fatal("semantic role declaration was not admitted")
	}
	declarations := NewDeclarations()
	for kind := schema.SurfaceKindStructure; kind.Available(); kind++ {
		var entries []schema.Entry
		switch kind {
		case schema.SurfaceKindStructure:
			entries = []schema.Entry{role}
		case schema.SurfaceKindAxis:
			entries = []schema.Entry{declarationLawEntry{key: "law/axis"}}
		}
		declarations.Register(declarationLawSurface{kind: kind, entries: entries})
	}
	return declarations
}

func TestDeclarationsSealOneExplicitCompilation(t *testing.T) {
	declarations := newDeclarationLawInput(t)
	compilation, failure := declarations.Seal()
	if failure.Available() || !compilation.Available() {
		t.Fatalf("explicit declaration seal failed: %#v", failure)
	}
	if compilation.Schema() == nil || !compilation.Digest().Available() {
		t.Fatal("sealed compilation did not retain its immutable schema identity")
	}
	publication, ok := compilation.Publication()
	if !ok || publication.Columns() != 1 {
		t.Fatalf("publication = %#v/%v, want one declared column", publication, ok)
	}
	if _, ok := ProjectAxis[uint32, uint64](publication, "law/output"); !ok {
		t.Fatal("declared output did not project from the same compilation")
	}
	_, secondFailure := declarations.Seal()
	if !secondFailure.Available() {
		t.Fatal("declarations sealed twice")
	}
	if declarations.Register(declarationLawSurface{kind: schema.SurfaceKindStructure}) {
		t.Fatal("sealed declarations accepted a later surface")
	}
}

func TestDeclarationsRejectIncompleteInputBeforeCompilation(t *testing.T) {
	declarations := NewDeclarations()
	compilation, failure := declarations.Seal()
	if compilation.Available() || failure.Disposition != schema.DispositionIncomplete {
		t.Fatalf("empty declarations = %#v/%#v, want incomplete", compilation, failure)
	}
}

func TestDeclarationsPreserveIndependentEnvironmentIdentity(t *testing.T) {
	left, right := newDeclarationLawInput(t), newDeclarationLawInput(t)
	leftCompilation, leftFailure := left.Seal()
	rightCompilation, rightFailure := right.Seal()
	if leftFailure.Available() || rightFailure.Available() || leftCompilation.Digest() != rightCompilation.Digest() {
		t.Fatal("equal explicit declaration inputs did not produce equal identities")
	}
}
