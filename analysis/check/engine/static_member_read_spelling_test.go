package engine_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// TestBareMemberReadRefutedByDeclaredArmUsesEitherSpelling proves that a member
// absent from the narrowed arm of a declared union is refuted whether the read
// is spelled with a field or with a quoted key. Both name the same member, so
// both publish the same missing-member fact.
func TestBareMemberReadRefutedByDeclaredArmUsesEitherSpelling(t *testing.T) {
	sources := map[string]string{
		"field": `
type Text = {kind: "text", value: string}
type Group = {kind: "group", children: {string}}

local function inspect(node: Text | Group): ()
    if node.kind == "text" then
        local children = node.children
    end
end
inspect({kind = "text", value = "a"})`,
		"quoted key": `
type Text = {kind: "text", value: string}
type Group = {kind: "group", children: {string}}

local function inspect(node: Text | Group): ()
    if node.kind == "text" then
        local children = node["children"]
    end
end
inspect({kind = "text", value = "a"})`,
	}
	for name, source := range sources {
		t.Run(name, func(t *testing.T) {
			result, err := engine.Check(source)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			for _, diagnostic := range result.PublishedDiagnostics {
				if diagnostic.Code == "type.member.missing" && diagnostic.Span.StartLine == 7 {
					return
				}
			}
			t.Fatalf("narrowed arm accepted an absent member: %#v", result.PublishedDiagnostics)
		})
	}
}

// TestDeclaredFormalAssignmentSurvivesStaticMemberRead proves that a body which
// both reads a declared member and claims a declared formal keeps publishing
// its assignment refutation: the static member read boundary must not displace
// the wider declared boundary that already carries the body.
func TestDeclaredFormalAssignmentSurvivesStaticMemberRead(t *testing.T) {
	result, err := engine.Check(`
type A = {tag: "a", value: string}
type B = {tag: "b", value: number}

local function check(r: A | B)
    if r.tag == "a" then
    else
        local s: string = r.value
    end
end`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Code == "type.assignment" && diagnostic.Span.StartLine == 8 {
			return
		}
	}
	t.Fatalf("declared boundary lost its assignment refutation: %#v", result.PublishedDiagnostics)
}
