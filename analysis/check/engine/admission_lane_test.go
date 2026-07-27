package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// TestAdmissionLanePrecedencePinned transcribes the effective priority of the
// legacy exclusion chain before that chain is removed. The overwrite lanes are
// ordered most-specific first; the fallback lanes retain each successive
// exclusion in the old chain. The sealed-capture lane also owns the chain's
// parameter-free/capture-free closed root policy.
func TestAdmissionLanePrecedencePinned(t *testing.T) {
	want := []string{
		"gradual-logical-call",
		"declared-local-union-read",
		"declared-indexed-read",
		"static-assignment",
		"typed-channel-send",
		"declared",
		"explicit-any",
		"static-captured-return",
		"static-arithmetic",
		"static-member-read",
		"declared-formal-call",
		"imported-capture",
		"sealed-capture",
		"contextual-callback",
	}
	if len(admissionLanes) != len(want) {
		t.Fatalf("admission lane count = %d, want %d", len(admissionLanes), len(want))
	}
	seen := make(map[string]bool, len(admissionLanes))
	for index, lane := range admissionLanes {
		if lane.Name != want[index] {
			t.Fatalf("admission lane %d = %q, want %q", index, lane.Name, want[index])
		}
		if seen[lane.Name] {
			t.Fatalf("duplicate admission lane %q", lane.Name)
		}
		seen[lane.Name] = true
		if lane.Admit == nil || len(lane.Discharges) == 0 {
			t.Fatalf("admission lane %q has an incomplete descriptor", lane.Name)
		}
	}
}

// TestAdmissionConsumerRequiresBodyIndex is the consumer mutation proof. The
// explicit-any body is admitted from its declaration-owned seed projection;
// severing that projection at the allocation consumer must fail closed rather
// than letting any descriptor recompute the boundary from Compilation.
func TestAdmissionConsumerRequiresBodyIndex(t *testing.T) {
	compilation, err := front.Compile(`
local function validate(value: any): string
    local strict: string = value
    return strict
end
return validate`)
	if err != nil {
		t.Fatal(err)
	}
	if len(compilation.NestedCompilations()) != 1 {
		t.Fatalf("nested bodies = %d, want 1", len(compilation.NestedCompilations()))
	}
	child := compilation.NestedCompilations()[0]
	bodyIndex := indexAdmissionBody(child)
	ctx := admissionLaneContext{child: child, bodyIndex: bodyIndex}
	result := selectAdmissionLane(&ctx)
	decision := result.decision
	if !result.admitted() || decision.Lane == nil || decision.Lane.Name != "explicit-any" {
		name := ""
		if decision.Lane != nil {
			name = decision.Lane.Name
		}
		t.Fatalf("indexed admission = (%q, %v), want explicit-any", name, result.admitted())
	}

	ctx.bodyIndex = admissionBodyIndex{} // mutation: disconnect the consumer from its projection.
	if result := selectAdmissionLane(&ctx); result.admitted() {
		decision := result.decision
		name := ""
		if decision.Lane != nil {
			name = decision.Lane.Name
		}
		t.Fatalf("admission survived missing body index through lane %q", name)
	}
}
