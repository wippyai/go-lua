package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// TestEverySurfaceIdentityIsOneDeclaredRole is the cross-surface ownership
// law. The structural table is the sole role inventory; this test verifies
// that every executable surface consumes that inventory and that no two
// writers claim one role. It intentionally keeps no copied role list or
// digest table.
func TestEverySurfaceIdentityIsOneDeclaredRole(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	sealed, failure := Table(compilation)
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	roles, rolesOK := SemanticRoles(compilation)
	if !rolesOK {
		t.Fatal("sealed table resolved no semantic role vocabulary")
	}
	claimed := make(map[schema.Key]schema.Key)
	claim := func(t *testing.T, owner, role schema.Key) {
		t.Helper()
		if _, resolved := roles.Key(role); !resolved {
			t.Fatalf("entry %q is declared under role %q, which no row declares", owner, role)
		}
		if prior, duplicate := claimed[role]; duplicate {
			t.Fatalf("entries %q and %q are both declared under role %q", prior, owner, role)
		}
		claimed[role] = owner
	}
	for _, entry := range state.axes {
		claim(t, entry.Key(), entry.Semantic())
		for index := 0; index < entry.RoleCount(); index++ {
			role, roleOK := entry.RoleAt(index)
			if !roleOK {
				t.Fatalf("axis %q holds no role at %d", entry.Key(), index)
			}
			if _, resolved := roles.Key(role); !resolved {
				t.Fatalf("axis %q consumes role %q, which no row declares", entry.Key(), role)
			}
		}
	}
	for _, entry := range state.templates {
		claim(t, entry.Key(), entry.Semantic())
		for index := 0; index < entry.RoleCount(); index++ {
			role, roleOK := entry.RoleAt(index)
			if !roleOK {
				t.Fatalf("rule %q holds no role at %d", entry.Key(), index)
			}
			if _, resolved := roles.Key(role); !resolved {
				t.Fatalf("rule %q consumes role %q, which no row declares", entry.Key(), role)
			}
		}
	}
	if len(claimed) == 0 {
		t.Fatal("no surface entry is declared under a role")
	}
}
