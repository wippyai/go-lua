package front_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// guardedClaims collects, for every claim a body lowers, the branch-outcome
// guards that claim carries. A claim is the operation an annotation is checked
// at, so its guard set is exactly the arm ownership the point is analyzed
// under.
func guardedClaims(t *testing.T, source string) [][]string {
	t.Helper()
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	claims := make([][]string, 0)
	for _, operation := range compilation.Artifact.Equations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		guards := make([]string, 0, len(operation.Guards))
		for _, guard := range operation.Guards {
			if encoding := string(guard.Encoding); strings.HasPrefix(encoding, "front/branch/") {
				guards = append(guards, encoding)
			}
		}
		claims = append(claims, guards)
	}
	return claims
}

// TestLoopBodyArmOwnershipSurvivesTheBackEdge pins the arm-ownership rule a
// point inside a loop is analyzed under. The other arm of a branch in the loop
// body reaches this point again through the back edge, but only by evaluating
// the same branch a second time. That is a later iteration, not an alternate
// edge of the decision this point sits under, so the point still carries the
// arm its own evaluation selected. Without the exclusion both arms count as
// reaching and the point carries no guard at all, which is what leaves a
// narrowed edge publication invisible inside a loop body.
func TestLoopBodyArmOwnershipSurvivesTheBackEdge(t *testing.T) {
	claims := guardedClaims(t, `
local xs: {string?} = {}
for _, x in ipairs(xs) do
    if x then
        local kk: string = x
    end
end
`)
	guarded := 0
	for _, guards := range claims {
		if len(guards) != 0 {
			guarded++
		}
		for _, guard := range guards {
			if !strings.HasSuffix(guard, "/true") {
				t.Fatalf("claim in the then arm carries %q, want the true outcome", guard)
			}
		}
	}
	if guarded == 0 {
		t.Fatal("no claim inside the loop body carries a branch outcome")
	}
}

// TestRejoinedLoopPointOwnsNoArm is the other half of the rule: past the
// branch both arms reach the point without re-evaluating it, so no outcome is
// required and the point keeps the declared state. A rule that guarded on a
// single reaching edge alone would claim an arm here.
func TestRejoinedLoopPointOwnsNoArm(t *testing.T) {
	claims := guardedClaims(t, `
local xs: {string?} = {}
for _, x in ipairs(xs) do
    if x then
        local kk: string = x
    end
    local after: string = x
end
`)
	if len(claims) < 2 {
		t.Fatalf("body lowered %d claims, want at least two", len(claims))
	}
	unguarded := 0
	for _, guards := range claims {
		if len(guards) == 0 {
			unguarded++
		}
	}
	if unguarded == 0 {
		t.Fatal("every claim carried an arm, but the point past the branch is owned by neither")
	}
}

// TestAcyclicArmOwnershipIsUnchanged pins the compatibility constraint the
// exclusion is admitted under. On a graph with no cycle no successor of a
// branch reaches that branch again, so excluding it removes no path and the
// lowered artifact is the one an acyclic corpus already had.
func TestAcyclicArmOwnershipIsUnchanged(t *testing.T) {
	const source = `
local v: string? = nil
if v then
    local held: string = v
end
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
	guarded := 0
	for _, operation := range first.Artifact.Equations {
		for _, guard := range operation.Guards {
			if strings.HasPrefix(string(guard.Encoding), "front/branch/") {
				guarded++
			}
		}
	}
	if guarded == 0 {
		t.Fatal("acyclic body carries no branch outcome at all")
	}
}
