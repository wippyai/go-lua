package composite

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestIssuanceRequirementIsAdmissionVocabularyNotIdentity is the identity fence
// of the declared-admissibility column. The requirement decides which compiled
// rows a rule is placed on; it names no engine slot, so it cannot reach the
// cold composition every persisted artifact and mounted graph is keyed under.
//
// The fence is stated over the two ways a spelling could reach that identity: a
// requirement member that also stood as a semantic role would key an engine
// slot, and a requirement member declared in a second category would be read as
// one of the vocabularies the placement ordinals are pinned to.
func TestIssuanceRequirementIsAdmissionVocabularyNotIdentity(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	table, tableOK := structure.NewTable(view)
	roles, rolesOK := SemanticRoles()
	if !viewOK || !tableOK || !rolesOK {
		t.Fatal("sealed table published no structural vocabulary")
	}
	count := table.Count(structure.CategoryIssuanceRequirement)
	if count == 0 {
		t.Fatal("the requirement vocabulary declares no member")
	}
	declared := make(map[schema.Key]struct{}, count)
	for ordinal := uint16(1); int(ordinal) <= count; ordinal++ {
		entry, entryOK := table.At(structure.CategoryIssuanceRequirement, ordinal)
		if !entryOK || !entry.Key().Available() {
			t.Fatalf("requirement member %d is unreachable at its declared ordinal", ordinal)
		}
		if _, semantic := roles.Key(entry.Key()); semantic {
			t.Fatalf("requirement %q also resolves as a semantic role, so an engine slot could be keyed by an admission term", entry.Key())
		}
		declared[entry.Key()] = struct{}{}
	}
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*structure.Entry)
		if !rowOK || !entryOK {
			t.Fatalf("structural row %d is not a vocabulary member", position)
		}
		if _, requirement := declared[entry.Key()]; !requirement {
			continue
		}
		if entry.Category() != structure.CategoryIssuanceRequirement {
			t.Fatalf("requirement %q is also declared in category %d", entry.Key(), entry.Category())
		}
	}
}

// TestIssuanceRequirementOrdinalsPinTheArtifactVocabulary states the other half
// of the projection: the compiler switches on artifact ordinals, so the
// declared member at each ordinal is the shape that ordinal spells. A member
// reordered here would silently retarget every placement declared against it.
func TestIssuanceRequirementOrdinalsPinTheArtifactVocabulary(t *testing.T) {
	sealed, failure := Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	table, tableOK := structure.NewTable(view)
	if !viewOK || !tableOK {
		t.Fatal("sealed table published no structural vocabulary")
	}
	pinned := []struct {
		key      schema.Key
		spelling string
		ordinal  programartifact.IssuanceRequirement
	}{
		{"requirement/unrestricted", "unrestricted", 1},
		{"requirement/call-plain-unary", "call-plain-unary", 2},
	}
	if table.Count(structure.CategoryIssuanceRequirement) != len(pinned) {
		t.Fatalf("the requirement vocabulary declares %d members, %d are pinned", table.Count(structure.CategoryIssuanceRequirement), len(pinned))
	}
	for _, row := range pinned {
		entry, entryOK := table.At(structure.CategoryIssuanceRequirement, uint16(row.ordinal))
		if !entryOK || entry.Key() != row.key || entry.Spelling() != row.spelling {
			t.Fatalf("ordinal %d is not the declared shape %q", row.ordinal, row.key)
		}
	}
}
