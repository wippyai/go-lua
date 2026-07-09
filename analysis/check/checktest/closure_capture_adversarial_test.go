package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestClosureCaptureSeesReassignmentAfterCapture(t *testing.T) {
	src := `local function f(): string
    local x: string? = "ready"
    local get = function(): string?
        return x
    end
    x = nil
    local out: string = get()
    return out
end
`
	result := Check(src)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	requireEvidenceMessage(t, diag, "get(...) can be string or nil here")
	requireEvidenceMessage(t, diag, "out is declared as string")
	requireEvidenceMessage(t, diag, "no guard on this path proves get(...) is non-nil")
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign get(...) because it may be nil
 --> test.lua:7:25
  |
  |                ↓ declared type
7 |     local out: string = get()
  |                         ↑ assigned value

because:
  1. proven: get(...) can be string or nil here
  2. claimed: out is declared as string
  3. missing proof: no guard on this path proves get(...) is non-nil

help: Guard ` + "`get(...)`" + ` with a nil check, provide a default value, or change the target type to accept nil.`
	assertRenderedEqual(t, rendered, want)
}

func TestClosureCaptureSeesLoopReassignmentAfterCapture(t *testing.T) {
	src := `local function f(n: number): string
    local x: string? = "ready"
    local get = function(): string?
        return x
    end
    for i = 1, n do
        x = nil
    end
    local out: string = get()
    return out
end
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            9,
		Column:          25,
		Span:            diagnostic.Span{StartLine: 9, StartCol: 25, EndLine: 9, EndCol: 27},
		MessageContains: []string{"get(...)", "may be nil"},
		EvidenceMin:     3,
		EvidenceOrdered: []string{
			"get(...) can be string or nil here",
			"out is declared as string",
			"no guard on this path proves get(...) is non-nil",
		},
		LabelMin: 2,
		LabelContains: []string{
			"declared type",
			"assigned value",
		},
		HelpContains: []string{"Guard `get(...)`"},
		Sources:      diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"9 |     local out: string = get()",
			"1. proven: get(...) can be string or nil here",
			"2. claimed: out is declared as string",
			"3. missing proof: no guard on this path proves get(...) is non-nil",
		},
		RenderNotContains: []string{
			"^",
			"get returns string?",
		},
	})
}

func TestClosureCaptureSingleAssignmentKeepsRecordShapeInBody(t *testing.T) {
	result := Check(`type Buf = { n: number }
local buf: Buf = { n = 0 }
local function push(v: number): number
    buf.n = buf.n + v
    return buf.n
end
return push(1)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for single-assignment captured record shape", result.Diagnostics)
	}
}

func TestClosureCaptureSiblingWriteDropsDefinitionNarrowing(t *testing.T) {
	result := Check(`local function f(): string
    local x: string? = "ready"
    local function clear()
        x = nil
    end
    if x ~= nil then
        local function get(): string
            return x
        end
        return get()
    end
    return ""
end
return f()
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeReturnContractType)
	if !strings.Contains(diag.Message, "x") || !strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want return diagnostic for nilable captured x", diag)
	}
}

func TestClosureCaptureEscapingMutableCaptureDropsPointNarrowing(t *testing.T) {
	result := Check(`local function make(): () -> number
    local box: { n: number? } = { n = 1 }
    if box.n ~= nil then
        local read = function(): number
            return box.n
        end
        box.n = nil
        return read
    end
    return function(): number
        return 0
    end
end
return make()
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeReturnContractType)
	if !strings.Contains(diag.Message, "box.n") || !strings.Contains(diag.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want return diagnostic for escaped nilable box.n", diag)
	}
}

func TestClosureCapturePreservesNestedLiteralFieldsForUnwrittenTable(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "plain closure body call argument",
			src: `local function expect_string(v: string): string
    return v
end

local AGENT_CONFIG = {
    memory = { contract_id = "wippy.agent:memory" },
}

local function load_memory(): string
    return expect_string(AGENT_CONFIG.memory.contract_id)
end

return load_memory()
`,
		},
		{
			name: "captured by sibling closure before call argument read",
			src: `local function expect_string(v: string): string
    return v
end

local AGENT_CONFIG = {
    memory = { contract_id = "wippy.agent:memory" },
}

local function observe_config(): any
    return AGENT_CONFIG.memory
end

local function load_memory(): string
    observe_config()
    return expect_string(AGENT_CONFIG.memory.contract_id)
end

return load_memory()
`,
		},
		{
			name: "returned closure call argument",
			src: `local function expect_string(v: string): string
    return v
end

local function make_loader(): () -> string
    local AGENT_CONFIG = {
        memory = { contract_id = "wippy.agent:memory" },
    }

    return function(): string
        return expect_string(AGENT_CONFIG.memory.contract_id)
    end
end

local load_memory = make_loader()
return load_memory()
`,
		},
		{
			name: "returned closure after sibling capture",
			src: `local function expect_string(v: string): string
    return v
end

local function make_loader(): () -> string
    local AGENT_CONFIG = {
        memory = { contract_id = "wippy.agent:memory" },
    }

    local function observe_config(): any
        return AGENT_CONFIG.memory
    end

    return function(): string
        observe_config()
        return expect_string(AGENT_CONFIG.memory.contract_id)
    end
end

local load_memory = make_loader()
return load_memory()
`,
		},
		{
			name: "read after escaping sibling callback capture",
			src: `local function expect_string(v: string): string
    return v
end

local scheduler = {}
function scheduler.spawn(fn: () -> any): ()
end

local AGENT_CONFIG = {
    memory = { contract_id = "wippy.agent:memory" },
}

scheduler.spawn(function(): any
    return AGENT_CONFIG.memory
end)

local function load_memory(): string
    return expect_string(AGENT_CONFIG.memory.contract_id)
end

return load_memory()
`,
		},
		{
			name: "returned closure after escaping sibling callback capture",
			src: `local function expect_string(v: string): string
    return v
end

local scheduler = {}
function scheduler.spawn(fn: () -> any): ()
end

local function make_loader(): () -> string
    local AGENT_CONFIG = {
        memory = { contract_id = "wippy.agent:memory" },
    }

    scheduler.spawn(function(): any
        return AGENT_CONFIG.memory
    end)

    return function(): string
        return expect_string(AGENT_CONFIG.memory.contract_id)
    end
end

local load_memory = make_loader()
return load_memory()
`,
		},
		{
			name: "returned closure with widened intermediate table member",
			src: `local function expect_string(v: string): string
    return v
end

local function make_loader(): () -> string
    local memory: table = { contract_id = "wippy.agent:memory" }
    local AGENT_CONFIG = {
        memory = memory,
    }

    return function(): string
        return expect_string(AGENT_CONFIG.memory.contract_id)
    end
end

local load_memory = make_loader()
return load_memory()
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Check(tc.src)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want none for nested literal field call argument", result.Diagnostics)
			}
		})
	}

	t.Run("module exported closure call argument", func(t *testing.T) {
		contracts := CheckAndExport(`local M = {}

function M.get(contract_id: string): any
    return contract_id
end

return M
`, "contracts")
		if len(contracts.Errors) != 0 {
			t.Fatalf("contracts diagnostics = %#v, want none", contracts.Errors)
		}

		agent := CheckAndExport(`local contract_module = require("contracts")

local AGENT_CONFIG = {
    memory = { contract_id = "wippy.agent:memory" },
}

local M = {}

function M.load_memory(): any
    return contract_module.get(AGENT_CONFIG.memory.contract_id)
end

return M
`, "agent", WithModule("contracts", contracts))
		if len(agent.Errors) != 0 {
			t.Fatalf("agent diagnostics = %#v, want none for exported nested literal field call argument", agent.Errors)
		}
	})
}

func TestClosureCaptureNestedLiteralFieldInvalidatesAfterWrite(t *testing.T) {
	result := Check(`local function expect_string(v: string): string
    return v
end

local function make_loader(): () -> string
    local memory: table = { contract_id = "wippy.agent:memory" }
    local AGENT_CONFIG = {
        memory = memory,
    }

    return function(): string
        AGENT_CONFIG.memory.contract_id = 42
        return expect_string(AGENT_CONFIG.memory.contract_id)
    end
end

local load_memory = make_loader()
return load_memory()
`)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	if !strings.Contains(diag.Message, "AGENT_CONFIG.memory.contract_id") ||
		(!strings.Contains(diag.Message, "not string") &&
			!strings.Contains(diag.Message, "comes from any/unknown")) {
		t.Fatalf("diagnostic = %#v, want nested member write to invalidate string proof", diag)
	}
}

func TestClosureCaptureCounterAccumulatorStaysClean(t *testing.T) {
	result := Check(`type Buf = { n: number }
local buf: Buf = { n = 0 }
local function push(v: number): number
    buf.n = buf.n + v
    return buf.n
end
push(1)
return buf.n
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for captured counter accumulator", result.Diagnostics)
	}
}
