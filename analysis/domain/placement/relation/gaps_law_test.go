package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/placement/relation"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
)

// TestPlacementGapInventoryIsTotal makes an omitted declaration a test
// failure instead of silently treating it as an engine fallback.
func TestPlacementGapInventoryIsTotal(t *testing.T) {
	want := map[string]relation.GapState{
		"placement/allocationbirth":     relation.GapBound,
		"placement/capture":             relation.GapBound,
		"placement/containment":         relation.GapBound,
		"placement/formal":              relation.GapBound,
		"placement/freshbirth":          relation.GapBound,
		"placement/publicationescape":   relation.GapBound,
		"placement/returnescape":        relation.GapBound,
		"placement/store":               relation.GapBound,
		"placement/suspension":          relation.GapBound,
		"placement/suspension-evidence": relation.GapBound,
		"placement/transfer":            relation.GapBound,
	}
	seen := make(map[string]bool, len(want))
	for _, gap := range relation.Gaps() {
		state, known := want[gap.Family]
		if !known {
			t.Errorf("unexpected Placement signature %q", gap.Family)
			continue
		}
		if seen[gap.Family] {
			t.Errorf("duplicate Placement signature %q", gap.Family)
		}
		seen[gap.Family] = true
		if gap.State != state {
			t.Errorf("%s state = %v, want %v", gap.Family, gap.State, state)
		}
		if gap.Reason == "" {
			t.Errorf("%s has no explicit gap reason", gap.Family)
		}
	}
	for family := range want {
		if !seen[family] {
			t.Errorf("missing Placement signature %q", family)
		}
	}

	declared := make(map[string]bool, len(want))
	for _, family := range relbind.Declared().Families {
		if family.Axis != "placement" || family.Census == "" {
			continue
		}
		state, known := want[family.Census]
		if !known {
			t.Errorf("corpus declares unexpected Placement family %q", family.Census)
			continue
		}
		if declared[family.Census] {
			t.Errorf("corpus declares Placement family %q twice", family.Census)
		}
		declared[family.Census] = true
		if family.Emitted() != (state == relation.GapBound) {
			t.Errorf("corpus emitted=%v for %s, want bound=%v", family.Emitted(), family.Census, state == relation.GapBound)
		}
	}
	for family := range want {
		if !declared[family] {
			t.Errorf("corpus lacks Placement family %q", family)
		}
	}
}
