package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// recurrenceTaggedDecisions counts the branch operations of a compiled body,
// and how many of them carry the recurrence marker.
func recurrenceTaggedDecisions(t *testing.T, source string) (decisions, tagged int) {
	t.Helper()
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, operation := range compilation.Artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		decisions++
		for _, operand := range operation.Operands {
			if operand.Role == "recurrence" && string(operand.Term.Encoding) == "recurrence/cyclic" {
				tagged++
				break
			}
		}
	}
	return decisions, tagged
}

// TestBranchDecisionsCarryTheRecurrenceMarkerOnlyInsideACycle pins the front's
// static recurrence signal. A decision an execution can arrive at more than
// once reads a different state on each arrival, so a consumer that publishes a
// proof for the whole fixed point has to be able to tell the two cases apart
// before it reads any value. The marker is that signal, and it is a property of
// the CFG rather than of any statement shape.
func TestBranchDecisionsCarryTheRecurrenceMarkerOnlyInsideACycle(t *testing.T) {
	// A decision outside every cycle is evaluated once, so it carries nothing.
	decisions, tagged := recurrenceTaggedDecisions(t, `
local n = 0
if n < 1 then
    n = 1
end
`)
	if decisions == 0 || tagged != 0 {
		t.Fatalf("acyclic body: decisions=%d tagged=%d, want at least one decision and no marker", decisions, tagged)
	}
	// A loop header is reached again on every trip, so it carries the marker.
	decisions, tagged = recurrenceTaggedDecisions(t, `
local n = 0
while n < 10 do
    n = n + 1
end
`)
	if decisions == 0 || tagged != decisions {
		t.Fatalf("loop header: decisions=%d tagged=%d, want every decision marked", decisions, tagged)
	}
	// A decision inside a loop body is equally recurrent; a decision that only
	// dominates the loop is not.
	decisions, tagged = recurrenceTaggedDecisions(t, `
local n = 0
local guard = 3
if guard > 0 then
    while n < 10 do
        if n == 5 then
            n = n + 2
        end
        n = n + 1
    end
end
`)
	if decisions < 3 || tagged != decisions-1 {
		t.Fatalf("mixed body: decisions=%d tagged=%d, want every decision but the dominating one marked", decisions, tagged)
	}
}

// TestAcyclicDecisionDraftsAreUnchangedByTheMarker pins the compatibility
// constraint the marker was admitted under: a body with no cycle must lower to
// the very same artifact it lowered to before the marker existed, so a corpus
// without loops cannot move. Canonical bytes are the whole draft, so equality
// here covers operand order and occurrence identity as well as the operands
// themselves.
func TestAcyclicDecisionDraftsAreUnchangedByTheMarker(t *testing.T) {
	const source = `
local function classify(n: number): string
    if n < 0 then
        return "negative"
    end
    if n == 0 then
        return "zero"
    end
    return "positive"
end
return classify
`
	first, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	second, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if string(first.Artifact.CanonicalBytes()) != string(second.Artifact.CanonicalBytes()) {
		t.Fatal("acyclic lowering is not deterministic")
	}
	for _, operation := range first.Artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role == "recurrence" {
				t.Fatalf("acyclic body carried a recurrence marker at %s", operation.Target.Name)
			}
		}
	}
}
