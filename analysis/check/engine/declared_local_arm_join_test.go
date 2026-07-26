package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/lint"
	diagnostic "github.com/wippyai/go-lua/analysis/diagnostic"
)

// treeUnionProject checks a two-module program whose entry declares a local of
// an imported discriminated union type without an initializer and assigns it on
// both edges of a branch. It returns the entry diagnostics.
func treeUnionProject(t *testing.T, main string) []diagnostic.Diagnostic {
	t.Helper()
	protocol := `type TextNode = {kind: "text", value: string}
type GroupNode = {kind: "group", children: {TreeNode}}
type TreeNode = TextNode | GroupNode
type Decoder<T> = {decode: (any) -> T}

local M = {}

function M.tree_type(): Decoder<TreeNode>
    return {
        decode = function(raw: any): TreeNode
            return {kind = "text", value = tostring(raw)}
        end,
    }
end

function M.decode(raw: string, decoder: Decoder<TreeNode>): TreeNode
    return decoder.decode(raw)
end

return M`
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries: []lint.Entry{
			{Path: "protocol.lua", ModulePath: "protocol", Source: protocol},
			{Path: "main.lua", ModulePath: "main", Imports: []string{"protocol"}, Source: main},
		},
		Targets: []string{"main"},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result.Diagnostics
}

func diagnosticAtLine(diagnostics []diagnostic.Diagnostic, line int) (diagnostic.Diagnostic, bool) {
	for _, item := range diagnostics {
		if item.Position.Line == line {
			return item, true
		}
	}
	return diagnostic.Diagnostic{}, false
}

// TestArmAssignedDeclaredLocalNarrowsPastTheJoin states the rule the two arm
// writes establish: each edge proved a type for the local, so the point they
// reconverge at carries their union and the discriminant guard past it selects
// one arm. The selected arm's members are then decided exactly, which is what
// refutes the number annotation on a string member.
func TestArmAssignedDeclaredLocalNarrowsPastTheJoin(t *testing.T) {
	diagnostics := treeUnionProject(t, `local protocol = require("protocol")

local function render(payload: string): string
    local tree: protocol.TreeNode
    if payload == "text" then
        tree = {kind = "text", value = payload}
    else
        tree = protocol.decode(payload, protocol.tree_type())
    end
    if tree.kind == "group" then
        local first = tree.children[1]
        if first and first.kind == "text" then
            local value: string = first.value
            local bad: number = first.value
            return value .. tostring(bad)
        end
    end
    return payload
end

return render`)
	item, found := diagnosticAtLine(diagnostics, 14)
	if !found {
		t.Fatalf("the group arm's child member was not decided past the join: %#v", diagnostics)
	}
	if item.Code != "type.assignment" {
		t.Fatalf("member refutation code = %q, want type.assignment", item.Code)
	}
	if _, unexpected := diagnosticAtLine(diagnostics, 13); unexpected {
		t.Fatalf("the string annotation on a string member was refuted: %#v", diagnostics)
	}
}

// TestArmAssignedDeclaredLocalRefutesTheOtherArmsMember is the second half of
// the same proof: the arm the guard selected states its whole member set, so a
// member only the other arm declares is absent rather than unknown.
func TestArmAssignedDeclaredLocalRefutesTheOtherArmsMember(t *testing.T) {
	diagnostics := treeUnionProject(t, `local protocol = require("protocol")

local function render(payload: string): string
    local tree: protocol.TreeNode
    if payload == "text" then
        tree = {kind = "text", value = payload}
    else
        tree = protocol.decode(payload, protocol.tree_type())
    end
    if tree.kind == "text" then
        local children = tree.children
        return tostring(children)
    end
    return payload
end

return render`)
	item, found := diagnosticAtLine(diagnostics, 11)
	if !found {
		t.Fatalf("the text arm did not refute the group arm's member: %#v", diagnostics)
	}
	if item.Code != "type.member.missing" {
		t.Fatalf("absent member code = %q, want type.member.missing", item.Code)
	}
}

// TestUnassignedDeclaredLocalProvesNoSurface keeps the lane falsifiable: the
// declaration alone is a user assertion, so a local no edge ever wrote reaches
// its read with no member surface and no member refutation at all.
func TestUnassignedDeclaredLocalProvesNoSurface(t *testing.T) {
	diagnostics := treeUnionProject(t, `local protocol = require("protocol")

local function render(payload: string): string
    local tree: protocol.TreeNode
    if tree.kind == "text" then
        local children = tree.children
        return tostring(children)
    end
    return payload
end

return render`)
	if item, found := diagnosticAtLine(diagnostics, 6); found {
		t.Fatalf("an unwritten declaration refuted a member: %#v", item)
	}
}
