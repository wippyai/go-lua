package engine_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/lint"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// checkModules runs the project checker over modules given in dependency
// order. The last module is the target, matching the fixture corpus layout.
func checkModules(t *testing.T, order []string, sources map[string]string) []diag.Diagnostic {
	t.Helper()
	entries := make([]lint.Entry, 0, len(order))
	for _, name := range order {
		entries = append(entries, lint.Entry{Path: name + ".lua", ModulePath: name, Source: sources[name]})
	}
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries: entries,
		Targets: []string{order[len(order)-1]},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result.Diagnostics
}

func hasDiagnostic(diagnostics []diag.Diagnostic, line int, code, want string) bool {
	for _, item := range diagnostics {
		if item.Position.Line == line && string(item.Code) == code && strings.Contains(item.Message, want) {
			return true
		}
	}
	return false
}

func diagnosticLines(diagnostics []diag.Diagnostic) string {
	lines := make([]string, 0, len(diagnostics))
	for _, item := range diagnostics {
		lines = append(lines, fmt.Sprintf("%s [%s] %s", item.Position.String(), item.Code, item.Message))
	}
	return strings.Join(lines, "\n")
}

const witnessProtocolModule = `
type Type<T> = { decode: (any) -> T }
type Record = { id: string, amount: number }

local M = {}

function M.record_type(): Type<Record>
    return {
        decode = function(raw: any): Record
            return { id = tostring(raw), amount = 1 }
        end,
    }
end

return M
`

const witnessDecoderModule = `
type Type<T> = { decode: (any) -> T }

local M = {}

function M.decode<T>(data: string, witness: Type<T>): T
    return witness.decode(data)
end

return M
`

// TestImportedCaptureBoundaryChecksUncalledBody proves an uncalled body whose
// only captures are module bindings is evaluated at its allocation boundary:
// the generic result binds from the witness argument, so the annotated local
// is refuted by the imported declaration alone.
func TestImportedCaptureBoundaryChecksUncalledBody(t *testing.T) {
	diagnostics := checkModules(t, []string{"protocol", "decoder", "main"}, map[string]string{
		"protocol": witnessProtocolModule,
		"decoder":  witnessDecoderModule,
		"main": `local protocol = require("protocol")
local decoder = require("decoder")

local function handle(payload: string): string
    local record = decoder.decode(payload, protocol.record_type())
    local id: string = record.id
    local bad: number = record.id
    return id .. tostring(bad)
end

return handle
`,
	})
	if !hasDiagnostic(diagnostics, 7, "type.assignment", "not number") {
		t.Fatalf("expected the witness-bound member read to refute the number annotation, got:\n%s", diagnosticLines(diagnostics))
	}
	if hasDiagnostic(diagnostics, 6, "type.assignment", "") {
		t.Fatalf("the matching string annotation must hold, got:\n%s", diagnosticLines(diagnostics))
	}
}

// TestImportedCaptureBoundaryLeavesMutableCaptureDormant proves the boundary is
// fail-closed. A module binding the enclosing body reassigns is no longer a
// stable authority, so the child receives no entry and stays dormant rather
// than being seeded with the binding's first value.
func TestImportedCaptureBoundaryLeavesMutableCaptureDormant(t *testing.T) {
	diagnostics := checkModules(t, []string{"protocol", "decoder", "main"}, map[string]string{
		"protocol": witnessProtocolModule,
		"decoder":  witnessDecoderModule,
		"main": `local protocol = require("protocol")
local decoder = require("decoder")

local function handle(payload: string): string
    local record = decoder.decode(payload, protocol.record_type())
    local bad: number = record.id
    return payload .. tostring(bad)
end

decoder = protocol

return handle
`,
	})
	if hasDiagnostic(diagnostics, 6, "type.assignment", "") {
		t.Fatalf("a reassigned module binding is no authority and must leave the body dormant, got:\n%s", diagnosticLines(diagnostics))
	}
}

// TestImportedAuthorityIsScopedToItsBody proves an import authority answers only
// for the body that published it. The module body's require result and the child
// body's call result occupy the same term name, so an authority keyed by name
// alone would answer the child's assignment with the module's export type and
// refute a correct annotation.
func TestImportedAuthorityIsScopedToItsBody(t *testing.T) {
	diagnostics := checkModules(t, []string{"boxes", "main"}, map[string]string{
		"boxes": `
type Boxed = { tag: string, body: string }

local M = {}

function M.wrap(payload: string): Boxed
    return { tag = "boxed", body = payload }
end

return M
`,
		"main": `local boxes = require("boxes")

local function open(payload: string): string
    local box: {tag: string, body: string} = boxes.wrap(payload)
    return box.body
end

return open
`,
	})
	if hasDiagnostic(diagnostics, 4, "type.assignment", "") {
		t.Fatalf("the wrapped record satisfies its annotation, got:\n%s", diagnosticLines(diagnostics))
	}
}
