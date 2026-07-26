package engine_test

import (
	"strings"
	"testing"
)

// TestNonDominatingMemberClosureCallJoinsBothResults is the defining case for
// per-edge callee resolution. The guarded write installs a second capability at
// M.dep.get; at the point after the branch both are live, so the call result is
// the union of what each returns and the member read off it may be nil.
func TestNonDominatingMemberClosureCallJoinsBothResults(t *testing.T) {
	diagnostics := checkSource(t, `local function run(flag: boolean)
    local M = { dep = { get = function() return nil end } }
    if flag then
        M.dep = { get = function() return { answer = "ok" } end }
    end
    local res = M.dep.get()
    local answer: string = res.answer
    return answer
end

return run
`)
	if len(diagnostics) != 1 {
		t.Fatalf("expected exactly the nil-assignment refutation, got:\n%s", diagnosticSummaries(diagnostics))
	}
	if got := diagnostics[0].Message; got != "cannot assign res.answer because it may be nil" {
		t.Fatalf("message = %q, want the may-be-nil refutation", got)
	}
	explanation := diagnostics[0].Explanation.String()
	for _, want := range []string{`res.answer can be "ok" or nil here`, "res may be nil before reading .answer", "no guard on this path proves res.answer is non-nil"} {
		if !strings.Contains(explanation, want) {
			t.Fatalf("explanation is missing %q:\n%s", want, explanation)
		}
	}
}

// TestNonDominatingMemberClosureReachesCallThroughWrapper keeps the join
// available where the branch write and the dispatch are in different bodies:
// the wrapper's captured environment differs per edge, so the same wrapper call
// carries both outcomes.
func TestNonDominatingMemberClosureReachesCallThroughWrapper(t *testing.T) {
	diagnostics := checkSource(t, `local function run(flag: boolean)
    local M = { dep = { get = function() return nil end } }

    function M.run()
        return M.dep.get()
    end

    if flag then
        M.dep = { get = function() return { answer = "ok" } end }
    end

    local res = M.run()
    local answer: string = res.answer
    return answer
end

return run
`)
	if len(diagnostics) != 1 {
		t.Fatalf("expected exactly the nil-assignment refutation, got:\n%s", diagnosticSummaries(diagnostics))
	}
	if got := diagnostics[0].Message; got != "cannot assign res.answer because it may be nil" {
		t.Fatalf("message = %q, want the may-be-nil refutation", got)
	}
}

// TestDominatingMemberClosureCallStaysExact is the precision guardrail: a write
// that dominates the call leaves one live capability, so its result keeps its
// exact type and the assignment is discharged.
func TestDominatingMemberClosureCallStaysExact(t *testing.T) {
	diagnostics := checkSource(t, `local function run()
    local M = { dep = { get = function() return nil end } }
    M.dep = { get = function() return { answer = "ok" } end }
    local res = M.dep.get()
    local answer: string = res.answer
    return answer
end

return run
`)
	if len(diagnostics) != 0 {
		t.Fatalf("a dominating write leaves one capability, got:\n%s", diagnosticSummaries(diagnostics))
	}
}

// TestBothEdgesAgreeingOnTheResultDischargesTheAssignment keeps the split from
// widening a point where both alternatives prove the same thing: two different
// capabilities that return the same shape leave the member non-nil.
func TestBothEdgesAgreeingOnTheResultDischargesTheAssignment(t *testing.T) {
	diagnostics := checkSource(t, `local function run(flag: boolean)
    local M = { dep = { get = function() return { answer = "first" } end } }
    if flag then
        M.dep = { get = function() return { answer = "second" } end }
    end
    local res = M.dep.get()
    local answer: string = res.answer
    return answer
end

return run
`)
	if len(diagnostics) != 0 {
		t.Fatalf("both edges prove a non-nil answer, got:\n%s", diagnosticSummaries(diagnostics))
	}
}
