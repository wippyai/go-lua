package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The structural half of this declaration's laws - its geometry, its identity,
// its agreement with the reducer call shape, and the refusal of every
// malformed edit of a term the cold ABI carries - is emitted from the
// declaration into generated_law_test.go.
//
// What stays here is the one thing the Program does not carry: the semantic
// role vocabulary this rule contributes to the structure surface. A role
// reaches the sealed table only through this contribution, so a rule that
// declares a role it does not contribute names an identity no composition can
// resolve.
func TestReturnEscapeContributesTheRolesItDeclares(t *testing.T) {
	specs := StructureSpecs()
	if len(specs) != 2 {
		t.Fatalf("structure spec count = %d, want this rule's own role and its operand role", len(specs))
	}
	contributed := make(map[schema.Key]struct{}, len(specs))
	for index, spec := range specs {
		if !spec.Key.Available() || spec.Category != structure.CategorySemanticRole {
			t.Fatalf("structure spec[%d] = %+v, want an available semantic role", index, spec)
		}
		contributed[spec.Key] = struct{}{}
	}
	entry := RuleEntry()
	for _, role := range append([]schema.Key{entry.Semantic}, entry.Roles...) {
		if _, present := contributed[role]; !present {
			t.Fatalf("the rule declares role %q and contributes no structure for it", string(role))
		}
	}
}
