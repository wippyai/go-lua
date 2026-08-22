package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

// TestPlacementTransferIsASealedMountedPlacementWriter is the composition
// authority law for the Target-transfer consumer.  Its key, owner, lane,
// semantic role, and call-effect issuance all come from the one sealed rule
// table; no parallel compatibility registration is allowed to make this
// consumer appear bound.
func TestPlacementTransferIsASealedMountedPlacementWriter(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	key := schema.Key("placement-transfer")
	entry, entryOK := templateForKey(compilation.catalog, key)
	if !entryOK || entry == nil {
		t.Fatal("placement-transfer is absent from the sealed rule table")
	}
	if entry.Lane() != rule.LaneMounted || !MountedRuleKey(compilation, key) {
		t.Fatalf("placement-transfer lane = %v, mounted = %t; want mounted", entry.Lane(), MountedRuleKey(compilation, key))
	}
	if entry.Writes() != schema.Key("placement") || entry.Owner() != schema.Key("placement") {
		t.Fatalf("placement-transfer writes/owner = %q/%q, want placement/placement", entry.Writes(), entry.Owner())
	}
	semantic, semanticOK := RuleSemantic(compilation, key)
	if !semanticOK || !semantic.Available() {
		t.Fatal("placement-transfer has no sealed semantic role")
	}
	if owner, ownerOK := RuleOwner(compilation, key); !ownerOK || owner != entry.Owner() {
		t.Fatalf("placement-transfer owner projection = %q/%t, want %q/true", owner, ownerOK, entry.Owner())
	}
	if entry.IssuanceCount() != 1 {
		t.Fatalf("placement-transfer issuance count = %d, want one call-effect issuance", entry.IssuanceCount())
	}
	issuance, issuanceOK := entry.IssuanceAt(0)
	if !issuanceOK || issuance.Occurrence != schema.Key("occurrence/call") ||
		issuance.Requirement != schema.Key("program-requirement/unrestricted") ||
		issuance.Form != schema.Key("program-form/call-effect") {
		t.Fatalf("placement-transfer issuance = %#v, want exact call-effect cut", issuance)
	}
	if entry.RoleCount() != 1 {
		t.Fatalf("placement-transfer role count = %d, want one operand role", entry.RoleCount())
	}
	role, roleOK := entry.RoleAt(0)
	if !roleOK || role != schema.Key("semantic/operand/placement/transfer") {
		t.Fatalf("placement-transfer operand role = %q/%t, want semantic/operand/placement/transfer", role, roleOK)
	}
}
