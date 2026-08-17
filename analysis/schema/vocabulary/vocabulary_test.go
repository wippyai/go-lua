package vocabulary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// TestRoleDerivationIsReplayableAndDistinct states the identity algebra this
// package owns: one role derives one identity, the derivation replays exactly,
// and a role with no spelling derives nothing.
func TestRoleDerivationIsReplayableAndDistinct(t *testing.T) {
	first, firstOK := Key("factor/value")
	second, secondOK := Key("factor/value")
	other, otherOK := Key("factor/call")
	if !firstOK || !secondOK || !otherOK {
		t.Fatal("declared semantic role derived no identity")
	}
	if first != second {
		t.Fatal("one semantic role did not replay exactly")
	}
	if first == other {
		t.Fatal("distinct semantic roles collided")
	}
	if _, ok := Key(""); ok {
		t.Fatal("empty semantic role was accepted")
	}
}

// TestRolesResolveDeclaredRowsOnly states the resolution: a declared row
// resolves to the identity its spelling derives, and a name no row declares
// resolves to nothing.
func TestRolesResolveDeclaredRowsOnly(t *testing.T) {
	entries, entriesOK := structure.Collect(RuleRoleSpecs("value/source"))
	if !entriesOK {
		t.Fatal("semantic role rows were not admitted")
	}
	roles, rolesOK := NewRoles(entries)
	if !rolesOK || roles.Count() != 3 {
		t.Fatalf("resolved %d roles, want 3", roles.Count())
	}
	semantics, semanticsOK := roles.Rule("value/source")
	if !semanticsOK {
		t.Fatal("declared rule role tuple did not resolve")
	}
	expected, expectedOK := Key("rule/value/source")
	if !expectedOK || semantics.Rule != expected {
		t.Fatal("resolved identity is not the one the declared spelling derives")
	}
	if _, ok := roles.Key(RoleKey("factor/value")); ok {
		t.Fatal("an undeclared role resolved")
	}
	if _, ok := roles.Transformed("value/source"); ok {
		t.Fatal("a rule that declares no transform form resolved one")
	}
}

// TestRestrictAdmitsOnlyTheNamedRoles states the narrowing an entry's hooks
// receive: a role the entry declared resolves, one it did not is not reachable,
// and naming an undeclared role leaves the restriction unavailable.
func TestRestrictAdmitsOnlyTheNamedRoles(t *testing.T) {
	specs := append(RuleRoleSpecs("value/source"), RoleSpecs("factor/value")...)
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("semantic role rows were not admitted")
	}
	roles, rolesOK := NewRoles(entries)
	if !rolesOK {
		t.Fatal("semantic role vocabulary did not resolve")
	}
	narrowed, narrowedOK := roles.Restrict(RoleKey("rule/value/source"))
	if !narrowedOK || narrowed.Count() != 1 {
		t.Fatal("restriction did not admit exactly the named role")
	}
	if _, ok := narrowed.Key(RoleKey("factor/value")); ok {
		t.Fatal("a role outside the restriction resolved")
	}
	if _, ok := roles.Restrict(RoleKey("factor/absent")); ok {
		t.Fatal("a restriction naming an undeclared role was admitted")
	}
}
