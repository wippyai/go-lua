package analysis

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The artifact query row address is the key a Result reads a published query
// column by, so the formula that derives it is part of the published contract:
// a Result holding an address minted under the old formula reads nothing under
// the new one. The construction cut moves who attaches these rows and when; it
// must not move the formula.
//
// artifactQueryFormulaFence is the tag artifact_query_plan.go frames the address
// under, and artifactQueryRoleFence is the closed role-name vocabulary that
// completes the preimage. Both are load-bearing preimage bytes, not labels.
const artifactQueryFormulaFence = "analysis/artifact-query/v1"

var artifactQueryRoleFence = []struct {
	role artifactQueryRole
	name string
	// address is the row the formula derives for the fixed mount/point pair
	// below. It pins the tag, the argument order, the role name, and the
	// length framing DeriveContentID applies, all in one literal.
	address string
}{
	{artifactQueryValueSummary, "value-summary", "fae0e88daf32331fe59d368e57758c17991d2e6a4de2edd40483cc84e6f1c1df"},
	{artifactQueryEffectExact, "effect-exact", "b214ec5905f50cdbfe42e3c61816f1ad248cbb3706a340b7067b552aa716804e"},
}

func artifactQueryFenceID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

// TestArtifactQueryRowAddressFormulaIsFenced pins the two row addresses the
// closed role vocabulary derives, and pins that the roles occupy distinct
// address spaces.
func TestArtifactQueryRowAddressFormulaIsFenced(t *testing.T) {
	mount, point := artifactQueryFenceID(0x31), artifactQueryFenceID(0x32)
	seen := make(map[identity.ContentID]string, len(artifactQueryRoleFence))
	for _, row := range artifactQueryRoleFence {
		id, ok := identity.DeriveContentID(artifactQueryFormulaFence, mount[:], point[:], []byte(row.name))
		if !ok || !id.Available() {
			t.Fatalf("the fenced artifact query address for %q no longer derives", row.name)
		}
		if got := hex.EncodeToString(id[:]); got != row.address {
			t.Errorf("artifact query row address for %q is %s, the fence pins %s", row.name, got, row.address)
		}
		if other, duplicate := seen[id]; duplicate {
			t.Errorf("roles %q and %q derive one address", row.name, other)
		}
		seen[id] = row.name
	}
	// The role vocabulary is closed and total: a third lane would publish a
	// column no fenced address names.
	if int(artifactQueryEffectExact) != len(artifactQueryRoleFence) {
		t.Errorf("the role vocabulary spans %d ordinals, the fence table holds %d rows", int(artifactQueryEffectExact), len(artifactQueryRoleFence))
	}
}

// TestArtifactQueryRowAddressIsPositional records that the pin above fences the
// whole preimage: mount and point are not interchangeable, the role name is
// required, and the tag is its own address space.
func TestArtifactQueryRowAddressIsPositional(t *testing.T) {
	mount, point := artifactQueryFenceID(0x31), artifactQueryFenceID(0x32)
	forward, forwardOK := identity.DeriveContentID(artifactQueryFormulaFence, mount[:], point[:], []byte("value-summary"))
	reverse, reverseOK := identity.DeriveContentID(artifactQueryFormulaFence, point[:], mount[:], []byte("value-summary"))
	roleless, rolelessOK := identity.DeriveContentID(artifactQueryFormulaFence, mount[:], point[:])
	foreign, foreignOK := identity.DeriveContentID(artifactQueryFormulaFence+"x", mount[:], point[:], []byte("value-summary"))
	if !forwardOK || !reverseOK || !rolelessOK || !foreignOK {
		t.Fatal("the fenced artifact query address no longer derives")
	}
	if forward == reverse {
		t.Fatal("mount and point are interchangeable in the row address")
	}
	if forward == roleless {
		t.Fatal("the role name does not reach the row address")
	}
	if forward == foreign {
		t.Fatal("the formula tag does not reach the row address")
	}
}

// TestArtifactQueryPlanRowsUseTheFencedFormula binds the pinned formula to the
// production plan: every row a real mounted plan issues must carry the address
// the fenced formula derives from that row's own coordinates. A literal pinned
// only in this file could drift from production; this law is what stops it.
func TestArtifactQueryPlanRowsUseTheFencedFormula(t *testing.T) {
	plan, status := Compile(fixtureLink(t, "core/control-for-loop"))
	if status != CompileComplete || plan == nil || plan.state == nil || len(plan.state.artifacts.mounts) == 0 {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	defer plan.Close()
	queryPlan, queryOK := newArtifactQueryPlan(plan.state.artifacts.mounts)
	if !queryOK || queryPlan == nil || len(queryPlan.rows) == 0 {
		t.Fatal("the mounted fixture issued no query plan rows")
	}
	names := make(map[artifactQueryRole]string, len(artifactQueryRoleFence))
	for _, row := range artifactQueryRoleFence {
		names[row.role] = row.name
	}
	for index, row := range queryPlan.rows {
		name, named := names[row.role]
		if !named {
			t.Fatalf("plan row %d carries role %d, which the fence table does not name", index, row.role)
		}
		want, wantOK := identity.DeriveContentID(artifactQueryFormulaFence, row.mount[:], row.point[:], []byte(name))
		if !wantOK || row.id != want {
			t.Fatalf("plan row %d address %v is not the fenced formula over its own (mount, point, %q)", index, row.id, name)
		}
	}
}
