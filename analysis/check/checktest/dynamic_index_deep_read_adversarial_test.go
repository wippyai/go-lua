package checktest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

func TestGuardedMapEntryDeepFieldReadKeepsDeclaredType(t *testing.T) {
	result := Check(`
type Meta = { route: string }
type Child = { meta: Meta }
type Item = { child: Child }
type Batch = { items: {[string]: Item} }

local batch: Batch = {
    items = {
        ["route-1"] = {
            child = { meta = { route = "route-1" } },
        },
    },
}

if batch.items["route-1"] then
    local bad_route: number = batch.items["route-1"].child.meta.route
end
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one deep field type mismatch", result.Diagnostics)
	}
	if got := result.Diagnostics[0].Code.String(); got != "type.assignment" {
		t.Fatalf("diagnostic code = %s, want type.assignment", got)
	}
}

func TestGuardedDeclaredMapEntryDeepFieldReadKeepsDeclaredType(t *testing.T) {
	result := Check(`
type Meta = { route: string }
type Child = { meta: Meta }
type Item = { child: Child }
type Batch = { items: {[string]: Item} }

local batch: Batch = { items = {} }

if batch.items["route-1"] then
    local bad_route: number = batch.items["route-1"].child.meta.route
end
`)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, rhs at bad_route = %s, want one deep field type mismatch",
			result.Diagnostics,
			localAssignmentSourceDebugAtLine(t, result, 10),
		)
	}
	if got := result.Diagnostics[0].Code.String(); got != "type.assignment" {
		t.Fatalf("diagnostic code = %s, want type.assignment", got)
	}
}

func TestPairsOverLocallyPopulatedMapCarriesInsertedRecordShape(t *testing.T) {
	result := Check(`
local state = {
    active_sessions = {},
}

state.active_sessions["s1"] = {
    created_at = 1,
    last_activity = 2,
}

local function need_number(value: number): ()
end

for _, session_info in pairs(state.active_sessions) do
    need_number(session_info.created_at)
    local last_activity = session_info.last_activity or session_info.created_at
    need_number(last_activity)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local dynamic map write shape to flow through pairs", result.Diagnostics)
	}
}

func TestCrossModuleGuardedMapEntryDeepFieldReadKeepsDeclaredType(t *testing.T) {
	protocolMod := CheckAndExport(`
type Meta = { route: string }
type Child = { meta: Meta }
type Item = { child: Child }
type Batch = { items: {[string]: Item} }

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item
M.Batch = Batch
return M
`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocolMod.Errors)
	}
	builderMod := CheckAndExport(`
local protocol = require("protocol")

local M = {}

function M.build(): protocol.Batch
    return {
        items = {
            ["route-1"] = {
                child = { meta = { route = "route-1" } },
            },
        },
    }
end

return M
`, "builder", WithModule("protocol", protocolMod))
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v, want none", builderMod.Errors)
	}
	if sig, ok := builderMod.Manifest.FunctionSignatures["builder.build"]; !ok || sig.Type == nil || len(sig.Type.Returns) != 1 {
		t.Fatalf("builder signatures = %#v, want builder.build with one declared return", builderMod.Manifest.FunctionSignatures)
	}

	result := Check(`
local builder = require("builder")
local batch = builder.build()

if batch.items["route-1"] then
    local bad_route: number = batch.items["route-1"].child.meta.route
end
`, WithModule("builder", builderMod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one deep field type mismatch", result.Diagnostics)
	}
	if got := result.Diagnostics[0].Code.String(); got != "type.assignment" {
		t.Fatalf("diagnostic code = %s, want type.assignment", got)
	}
}

func TestCrossModuleCallbackBuiltMapEntryDeepFieldReadKeepsDeclaredType(t *testing.T) {
	protocolMod := CheckAndExport(`
type Meta = {
    route: string,
    shard: string,
}

type Child = {
    id: string,
    meta: Meta,
}

type Item = {
    id: string,
    tags: {[string]: string},
    child: Child,
}

type Batch = {
    items: {[string]: Item},
    count: number,
}

local M = {}
M.Meta = Meta
M.Child = Child
M.Item = Item
M.Batch = Batch
return M
`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocolMod.Errors)
	}
	builderMod := CheckAndExport(`
local protocol = require("protocol")

local M = {}

function M.build(ids: {string}, fill: (protocol.Item, string, number) -> ()): protocol.Batch
    local batch: protocol.Batch = {items = {}, count = 0}
    for _, id in ipairs(ids) do
        batch.count = batch.count + 1
        local item: protocol.Item = {
            id = id,
            tags = {},
            child = {
                id = id,
                meta = {route = "", shard = ""},
            },
        }
        item.tags["phase"] = "constructing"
        fill(item, id, batch.count)
        item.tags["phase"] = "ready"
        batch.items[id] = item
    end
    return batch
end

return M
`, "builder", WithModule("protocol", protocolMod))
	if len(builderMod.Errors) != 0 {
		t.Fatalf("builder diagnostics = %#v, want none", builderMod.Errors)
	}

	result := Check(`
local builder = require("builder")

local batch = builder.build({"route-1", "route-2"}, function(item, id: string, index: number)
    item.child.meta.route = id
    if index == 1 then
        item.child.meta.shard = "primary"
    else
        item.child.meta.shard = "backup"
    end
    item.tags["callback"] = "filled"
end)

if batch.items["route-1"] then
    local bad_route: number = batch.items["route-1"].child.meta.route
end
`, WithModule("builder", builderMod))
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, rhs at bad_route = %s, want one deep field type mismatch",
			result.Diagnostics,
			localAssignmentSourceDebugAtLine(t, result, 15),
		)
	}
	if got := result.Diagnostics[0].Code.String(); got != "type.assignment" {
		t.Fatalf("diagnostic code = %s, want type.assignment", got)
	}
	if message := result.Diagnostics[0].Message; !strings.Contains(message, "string") ||
		!strings.Contains(message, "not number") ||
		strings.Contains(message, "may be nil") {
		t.Fatalf("diagnostic message = %q, want string-not-number without nilability", message)
	}
}

func localAssignmentSourceDebugAtLine(t *testing.T, result Result, line int) string {
	t.Helper()
	if result.checked == nil || result.checked.RootResult() == nil || result.checked.RootResult().Graph() == nil {
		return "<no checked result>"
	}
	root := result.checked.RootResult()
	for _, point := range root.Graph().RPO() {
		fact, ok := root.LocalAssignment(point)
		if !ok || fact.Expr == nil || fact.Expr.Line() != line {
			continue
		}
		value, ok := root.ExpressionValueBeforeBoundary(point, fact.Expr)
		source := fact.Source
		if !ok {
			return fmt.Sprintf("type=<no expression value> source=%d", source.Kind)
		}
		tp, ok := typevalue.TypeOf(root.Registry(), value)
		if !ok || tp == nil {
			return fmt.Sprintf("type=<no type> source=%d", source.Kind)
		}
		identityText := "no-id"
		if id, ok := valueIdentity(root.Registry(), value); ok {
			identityText = id.String()
		}
		return fmt.Sprintf("type=%s identity=%s source=%d has_annotation=%v", tp.String(), identityText, source.Kind, fact.Type != nil)
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || fact.Value == nil || fact.Value.Line() != line {
			continue
		}
		value, ok := root.ExpressionValueBeforeBoundary(point, fact.Value)
		if !ok {
			return "<ordinary assignment has no expression value>"
		}
		tp, ok := typevalue.TypeOf(root.Registry(), value)
		if !ok || tp == nil {
			return "<ordinary assignment has no type>"
		}
		return fmt.Sprintf("ordinary_type=%s", tp.String())
	}
	var localLines []int
	for _, point := range root.Graph().RPO() {
		fact, ok := root.LocalAssignment(point)
		if ok && fact.Expr != nil {
			localLines = append(localLines, fact.Expr.Line())
		}
	}
	var ordinaryLines []int
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if ok && fact.Value != nil {
			ordinaryLines = append(ordinaryLines, fact.Value.Line())
		}
	}
	return fmt.Sprintf("<assignment not found; local_lines=%v ordinary_lines=%v>", localLines, ordinaryLines)
}
