package composite

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Every module-composition axis role the schema declares is admitted to the
// axis table. The declaration publishes the role; the table grants its column.
// A role declared without its table entry leaves the compile path holding a
// column no capability can be minted for, and the composition publication that
// writes it refuses after the schema binding has already sealed.
func TestModuleCompositionAxesAreAdmitted(t *testing.T) {
	entries, _, sealed := axisTemplates()
	if !sealed || len(entries) == 0 {
		t.Fatalf("the axis table did not seal")
	}
	admitted := make(map[schema.Key]bool, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		admitted[entry.Key()] = true
	}
	specs := modulecomposition.StructureSpecs()
	if len(specs) == 0 {
		t.Fatalf("module composition declared no axis roles")
	}
	for _, spec := range specs {
		if spec.Category != structure.CategorySemanticRole {
			continue
		}
		key := schema.Key(strings.TrimPrefix(spec.Spelling, "axis/"))
		if !admitted[key] {
			t.Fatalf("module-composition axis %q is declared but not admitted to the axis table", key)
		}
	}
}
