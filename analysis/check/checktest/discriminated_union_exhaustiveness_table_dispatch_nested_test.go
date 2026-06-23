package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDiscriminatedUnionExhaustivenessReportsNestedDispatchTableKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedDispatchTableWithDynamicKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
        [key] = function(action: Action): string return action.kind end,
    },
}

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning for nested dispatch table with dynamic key", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsNestedDispatchTableStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
registry.handlers.cancel = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested static dispatch fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsNestedDispatchTableAfterDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
local key = "cancel"
registry.handlers[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = registry.handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after nested dynamic dispatch mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsDispatchTableAfterKnownNonCaseDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "audit"
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}
handlers[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "handlers.cancel"},
		EvidenceOrdered: []string{
			"`handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `handlers.begin`, `handlers.commit`",
			"missing dispatch keys: `handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsKnownCaseDynamicDispatchFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local key = "cancel"
local handlers = {
    begin = function(action: Action): string return action.kind end,
    commit = function(action: Action): string return action.kind end,
}
handlers[key] = function(action: Action): string return action.kind end

local action: Action = { kind = "begin", id = "evt-1" }
local handler = handlers[action.kind]
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after known dynamic dispatch fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessReportsCapturedNestedDispatchTableKey(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedNestedDispatchTableWithParentStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
registry.handlers.cancel = function(action: Action): string return action.kind end

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested parent static fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessAcceptsCapturedNestedDispatchTableWithChildStaticFill(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action): string
    registry.handlers.cancel = function(item: Action): string return item.kind end
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested child static fill", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedNestedDispatchTableAfterChildDynamicMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}

local function route(action: Action, key: string): string
    registry.handlers[key] = function(item: Action): string return item.kind end
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after captured nested dynamic mutation", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessSkipsCapturedNestedDispatchTableAfterParentUnknownCallMutation(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
patch_handlers(registry.handlers)

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithGlobals("patch_handlers"), WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	if hasDiagnosticCode(result.Diagnostics, diagnostics.CodeDiscriminatedUnionExhaustive) {
		t.Fatalf("diagnostics = %#v, want no exhaustive-union warning after parent unknown call can mutate captured nested dispatch table", result.Diagnostics)
	}
}

func TestDiscriminatedUnionExhaustivenessKeepsCapturedNestedDispatchTableAfterParentReadOnlyCall(t *testing.T) {
	result := Check(`
type Begin = { kind: "begin", id: string }
type Commit = { kind: "commit", payment_id: string }
type Cancel = { kind: "cancel", reason: string }
type Action = Begin | Commit | Cancel

local function inspect_handlers(handlers): ()
end

local registry = {
    handlers = {
        begin = function(action: Action): string return action.kind end,
        commit = function(action: Action): string return action.kind end,
    },
}
inspect_handlers(registry.handlers)

local function route(action: Action): string
    local handler = registry.handlers[action.kind]
    return ""
end
`, WithDiagnosticRule(
		diagnostics.CodeDiscriminatedUnionExhaustive,
		diagnostic.Enable(),
	))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDiscriminatedUnionExhaustive,
		Severity:        diagnostic.SeverityWarning,
		DiagnosticCount: 1,
		MessageContains: []string{"dispatch table is not exhaustive", "registry.handlers.cancel"},
		EvidenceOrdered: []string{
			"`registry.handlers` is indexed by discriminant `action.kind`",
			"dispatch table provides keys: `registry.handlers.begin`, `registry.handlers.commit`",
			"missing dispatch keys: `registry.handlers.cancel` for `action.kind == \"cancel\"`",
		},
	})
}
