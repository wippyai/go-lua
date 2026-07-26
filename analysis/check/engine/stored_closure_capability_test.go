package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

func checkDiagnosticMessages(t *testing.T, source string) []string {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	messages := make([]string, 0, len(result.PublishedDiagnostics))
	for _, diagnostic := range result.PublishedDiagnostics {
		messages = append(messages, diagnostic.Code+": "+diagnostic.Message)
	}
	return messages
}

func containsMessage(messages []string, fragment string) bool {
	for _, message := range messages {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// TestCheckMemberRoutedRecursionResolvesItsDeclaredResult pins the re-entry
// guard on the shape that closes a cycle no published capability spells out:
// the body loads its own callee from a member cell of the table it captures.
// The application must terminate through the recursion coordinator and keep the
// callee's declared result, which a wrong claim on that result still refutes.
func TestCheckMemberRoutedRecursionResolvesItsDeclaredResult(t *testing.T) {
	const source = `
local m = {}
function m.step(n: number): number
    if n <= 0 then
        return 0
    end
    return m.step(n - 1)
end
local total: %s = m.step(3)
return total
`
	if messages := checkDiagnosticMessages(t, strings.Replace(source, "%s", "number", 1)); len(messages) != 0 {
		t.Fatalf("member-routed recursion reported %v on its own declared result", messages)
	}
	messages := checkDiagnosticMessages(t, strings.Replace(source, "%s", "string", 1))
	if !containsMessage(messages, "call result 1 is number, not string") {
		t.Fatalf("diagnostics = %v, want the callee's number result refuting the string claim", messages)
	}
}

// TestCheckMemberRoutedMutualRecursionResolvesItsDeclaredResult pins the same
// guard on a cycle of length two: each body loads the other from the captured
// table, so neither closure handle names the edge.
func TestCheckMemberRoutedMutualRecursionResolvesItsDeclaredResult(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
local m = {}
function m.even(n: number): boolean
    if n <= 0 then
        return true
    end
    return m.odd(n - 1)
end
function m.odd(n: number): boolean
    if n <= 0 then
        return false
    end
    return m.even(n - 1)
end
local flag: string = m.even(4)
return flag
`)
	if !containsMessage(messages, "call result 1 is boolean, not string") {
		t.Fatalf("diagnostics = %v, want the mutual callee's boolean result refuting the string claim", messages)
	}
}

// TestCheckStaticMemberStoredClosureEscapesWithItsContainer pins the capability
// a static member write carries: the written term names the closure body, so the
// slot the write publishes holds it and an opaque sink that receives the
// container can run it. The captured field's narrowing therefore ends at the
// escape.
func TestCheckStaticMemberStoredClosureEscapesWithItsContainer(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
type Config = { host: string? }
local function escaping(): string
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers.fn = function()
        cfg.host = nil
    end
    sink(handlers)
    local h: string = cfg.host
    return h
end
return escaping
`)
	if !containsMessage(messages, "cannot assign cfg.host") {
		t.Fatalf("diagnostics = %v, want the escaped callback's write to end the field narrowing", messages)
	}
}

// TestCheckContainedStaticMemberStoredClosureKeepsTheFieldProof is the contrast
// arm: the container never reaches a call site, so no reachable callback can run
// and the constructor's value stands.
func TestCheckContainedStaticMemberStoredClosureKeepsTheFieldProof(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
type Config = { host: string? }
local function contained(): string
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers.fn = function()
        cfg.host = nil
    end
    local h: string = cfg.host
    return h .. tostring(handlers ~= nil)
end
return contained
`)
	if len(messages) != 0 {
		t.Fatalf("diagnostics = %v, want the field proof to survive a container nothing reaches", messages)
	}
}

// TestCheckExactDynamicKeyStoredClosureEscapesWithItsContainer pins the same
// capability for a store whose key resolves to a member suffix: the slot has a
// name, so it publishes an ordinary cell, and that cell carries the callable the
// written term names.
func TestCheckExactDynamicKeyStoredClosureEscapesWithItsContainer(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
type Config = { host: string? }
local function escaping(): string
    local slot = "fn"
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers[slot] = function()
        cfg.host = nil
    end
    sink(handlers)
    local h: string = cfg.host
    return h
end
return escaping
`)
	if !containsMessage(messages, "cannot assign cfg.host") {
		t.Fatalf("diagnostics = %v, want the escaped callback's write to end the field narrowing", messages)
	}
}

// TestCheckUnresolvedKeyStoredClosureEscapesWithItsContainer pins the inventory
// row: the store names no slot, so the container's unresolved-key inventory is
// the only record that the callable is inside it, and the opaque-callback walk
// must consume that record rather than skip the container.
func TestCheckUnresolvedKeyStoredClosureEscapesWithItsContainer(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
type Config = { host: string? }
local function escaping(): string
    local key: string = tostring(1)
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers[key] = function()
        cfg.host = nil
    end
    sink(handlers)
    local h: string = cfg.host
    return h
end
return escaping
`)
	if !containsMessage(messages, "cannot assign cfg.host") {
		t.Fatalf("diagnostics = %v, want the callable at the unnamed slot to escape with its container", messages)
	}
}

// TestCheckContainedUnresolvedKeyStoredClosureKeepsTheFieldProof is the
// contrast arm: an unnamed slot is still a slot of a container no call site
// reaches, so nothing can run the callable it holds.
func TestCheckContainedUnresolvedKeyStoredClosureKeepsTheFieldProof(t *testing.T) {
	messages := checkDiagnosticMessages(t, `
type Config = { host: string? }
local function contained(): string
    local key: string = tostring(1)
    local cfg: Config = { host = "x" }
    local handlers = {}
    handlers[key] = function()
        cfg.host = nil
    end
    local h: string = cfg.host
    return h .. tostring(handlers ~= nil)
end
return contained
`)
	if len(messages) != 0 {
		t.Fatalf("diagnostics = %v, want the field proof to survive a container nothing reaches", messages)
	}
}
