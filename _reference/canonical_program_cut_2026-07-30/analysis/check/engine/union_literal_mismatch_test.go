package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// TestUnionLiteralMismatchNamesTheRefutingMember proves that a refuted literal
// assigned to a union target reports the member that refutes it whenever the
// literal's named surface fits exactly one arm.
func TestUnionLiteralMismatchNamesTheRefutingMember(t *testing.T) {
	result, err := engine.Check(`
type Accepted = {id: string, attempt: number, source: string?}
type Rejected = {id: string, reason: string}
type Decision = Accepted | Rejected

local wrong: Decision = {
    id = "job",
    attempt = "two",
}`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" {
			continue
		}
		if diagnostic.Span.StartLine == 8 && strings.Contains(diagnostic.Message, "wrong.attempt") {
			return
		}
	}
	t.Fatalf("union literal mismatch did not name its refuting member: %#v", result.PublishedDiagnostics)
}

// TestUnionLiteralMismatchKeepsWholeValueWhenArmsAreAmbiguous proves that arm
// selection stays undecided when several arms admit the literal's named
// surface: the whole-value refutation remains the published contract.
func TestUnionLiteralMismatchKeepsWholeValueWhenArmsAreAmbiguous(t *testing.T) {
	result, err := engine.Check(`
type Left = {id: string, weight: number}
type Right = {id: string, weight: boolean}
type Either = Left | Right

local wrong: Either = {
    id = "job",
    weight = "two",
}`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	found := false
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code != "type.assignment" {
			continue
		}
		if strings.Contains(diagnostic.Message, "wrong.weight") {
			t.Fatalf("ambiguous arms selected one arm: %#v", diagnostic)
		}
		found = found || diagnostic.Span.StartLine == 6
	}
	if !found {
		t.Fatalf("ambiguous union literal lost its refutation: %#v", result.PublishedDiagnostics)
	}
}
