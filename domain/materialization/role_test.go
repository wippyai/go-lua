package materialization

import "testing"

func TestRoleIsClosedAndOnlyRecentAdvances(t *testing.T) {
	for _, role := range []Role{Exact, Recent, Summary} {
		if !role.Valid() {
			t.Fatalf("valid role %d rejected", role)
		}
	}
	for _, role := range []Role{Invalid, Role(4)} {
		if role.Valid() {
			t.Fatalf("invalid role %d admitted", role)
		}
	}
	if next, ok := RecentToSummary(Recent); !ok || next != Summary {
		t.Fatal("recent did not advance to summary")
	}
	for _, role := range []Role{Invalid, Exact, Summary} {
		if _, ok := RecentToSummary(role); ok {
			t.Fatalf("non-recent role %d advanced", role)
		}
	}
}

// TestCatalogIsTheDenseEnumerationOfEveryValidRole states the density law a
// consumer's exhaustive iteration rests on: the catalog is every role the
// admission predicate accepts, each once, in ordinal order from the first.
// A role added to the type and not to the catalog is a rejected build here
// rather than an alternative a consumer silently never visits.
func TestCatalogIsTheDenseEnumerationOfEveryValidRole(t *testing.T) {
	var admitted []Role
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		if role := Role(candidate); role.Valid() {
			admitted = append(admitted, role)
		}
	}
	catalog := Roles()
	if len(admitted) != RoleCount || len(catalog) != RoleCount {
		t.Fatalf("catalog holds %d roles and the type admits %d, declared count is %d", len(catalog), len(admitted), RoleCount)
	}
	for position, role := range catalog {
		if role != admitted[position] {
			t.Fatalf("catalog position %d is role %d, but the type's ordinal %d is role %d", position, role, position, admitted[position])
		}
		if int(role) != position+1 {
			t.Fatalf("catalog position %d holds role %d, so the ordinals are not dense from one", position, role)
		}
	}
}

// TestCatalogEnumerationAllocatesNothing states that iterating the catalog
// costs no allocation, so a consumer visiting every role on a hot path reaches
// for the declared catalog rather than an inline slice literal.
func TestCatalogEnumerationAllocatesNothing(t *testing.T) {
	if allocations := testing.AllocsPerRun(100, func() {
		for _, role := range Roles() {
			if !role.Valid() {
				t.Fatal("catalog holds an inadmissible role")
			}
		}
	}); allocations != 0 {
		t.Fatalf("catalog enumeration allocated %v times per run", allocations)
	}
}
