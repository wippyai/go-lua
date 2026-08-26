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
	links := LinkKeys(compilation)
	if len(links) < 2 {
		t.Fatalf("Link inventory has %d entries, want the two-rule Placement tail", len(links))
	}
	// The evidence producer follows the class producer it witnesses, so the
	// pair is consecutive in Link order. Their absolute table positions are
	// pinned by the two slot checks below; a later Link rule appended after
	// them changes neither relation.
	wantPair := []schema.Key{suspensionKey, suspensionEvidenceKey}
	pair := -1
	for index := range links {
		if links[index] == wantPair[0] {
			pair = index
			break
		}
	}
	if pair < 0 || pair+len(wantPair) > len(links) {
		t.Fatalf("Link inventory %v does not carry the Placement pair %v", links, wantPair)
	}
	for index, want := range wantPair {
		if got := links[pair+index]; got != want {
			t.Fatalf("Link pair slot %d = %q, want %q", index, got, want)
		}
	}
	suspensionEntry, suspensionOK := templateForKey(state, suspensionKey)
	if !suspensionOK || suspensionEntry == nil || suspensionEntry.Lane() != rule.LaneLink || MountedRuleKey(compilation, suspensionKey) {
		t.Fatalf("suspension entry missing or not Link-owned: present=%t entry=%v mounted=%t", suspensionOK, suspensionEntry, MountedRuleKey(compilation, suspensionKey))
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
