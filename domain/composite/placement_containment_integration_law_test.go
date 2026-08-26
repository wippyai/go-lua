package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// TestPlacementContainmentOwnsTheMountedPointLane pins containment's distinct
// closure geometry while preserving the declaration positions of the Link
// rules that follow it.
func TestPlacementContainmentOwnsTheMountedPointLane(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	const (
		containmentKey         schema.Key = "placement-containment"
		containmentSlot                   = 25
		suspensionKey          schema.Key = "placement-suspension"
		suspensionSlot                    = 27
		suspensionEvidenceKey  schema.Key = "placement-suspension-evidence"
		suspensionEvidenceSlot            = 28
	)
	key, keyOK := RuleKeyAt(compilation, containmentSlot-1)
	if !keyOK || key != containmentKey {
		t.Fatalf("slot %d = %q, want %q", containmentSlot, key, containmentKey)
	}
	entry, entryOK := templateForKey(state, containmentKey)
	if !entryOK || entry == nil {
		t.Fatalf("containment key has no sealed template")
	}
	if entry.Lane() != rule.LaneMountedPoint || MountedRuleKey(compilation, containmentKey) {
		t.Fatalf("containment lane = %v, mounted = %v; want mounted-point and not artifact-mounted", entry.Lane(), MountedRuleKey(compilation, containmentKey))
	}
	pointKeys := mountedPointKeys(state)
	if len(pointKeys) != 1 || pointKeys[0] != containmentKey {
		t.Fatalf("mounted-point inventory = %v, want [%q]", pointKeys, containmentKey)
	}
	// The suspension pair is artifact-mounted, not Link-owned: its judgment is
	// decided by the Call fact solved at the boundary the liveness span is
	// anchored at, and a Link rule reads every Factor at the mount-neutral
	// bootstrap point, where no mounted rule has written one. The evidence
	// producer follows the class producer it witnesses, so the pair stays
	// consecutive; their absolute table positions are pinned below.
	for _, key := range []schema.Key{suspensionKey, suspensionEvidenceKey} {
		entry, entryOK := templateForKey(state, key)
		if !entryOK || entry == nil || entry.Lane() != rule.LaneMounted || !MountedRuleKey(compilation, key) {
			t.Fatalf("suspension entry %q missing or not artifact-mounted: present=%t entry=%v mounted=%t", key, entryOK, entry, MountedRuleKey(compilation, key))
		}
		if entry.IssuanceCount() != 1 {
			t.Fatalf("suspension entry %q issuances=%d, want one subject-liveness subscription", key, entry.IssuanceCount())
		}
		issuance, issuanceOK := entry.IssuanceAt(0)
		if !issuanceOK || issuance.Occurrence != "occurrence/subject-liveness" {
			t.Fatalf("suspension entry %q issuance=%+v, want the subject-liveness occurrence", key, issuance)
		}
	}
	for _, key := range LinkKeys(compilation) {
		if key == suspensionKey || key == suspensionEvidenceKey {
			t.Fatalf("Link inventory still carries the mounted suspension key %q", key)
		}
	}
	key, keyOK = RuleKeyAt(compilation, suspensionSlot-1)
	if !keyOK || key != suspensionKey {
		t.Fatalf("slot %d = %q, want %q", suspensionSlot, key, suspensionKey)
	}
	key, keyOK = RuleKeyAt(compilation, suspensionEvidenceSlot-1)
	if !keyOK || key != suspensionEvidenceKey {
		t.Fatalf("slot %d = %q, want %q", suspensionEvidenceSlot, key, suspensionEvidenceKey)
	}
	semantic, semanticOK := RuleSemantic(compilation, containmentKey)
	want, wantOK := vocabulary.Key("rule/placement/containment")
	if !semanticOK || !wantOK || semantic != want {
		t.Fatalf("containment semantic = %v, want %v", semantic, want)
	}
}
