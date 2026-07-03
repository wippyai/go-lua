package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDirectCallArgumentAcceptsRecursiveAliasExpansion(t *testing.T) {
	result := Check(`
type A = { b: B?, tag: "a" }
type B = { c: C?, tag: "b" }
type C = { a: A?, tag: "c" }

local function walk(a: A?): number
    if a == nil then return 0 end
    if a.b == nil then return 1 end
    if a.b.c == nil then return 2 end
    return 3 + walk(a.b.c.a)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want recursive alias optional argument accepted", result.Diagnostics)
	}
}

func TestJudgmentDirectCallArgumentAcceptsRecursiveAliasExpansion(t *testing.T) {
	result := Check(`
type A = { b: B?, tag: "a" }
type B = { c: C?, tag: "b" }
type C = { a: A?, tag: "c" }

local function walk(a: A?): number
    if a == nil then return 0 end
    if a.b == nil then return 1 end
    if a.b.c == nil then return 2 end
    return 3 + walk(a.b.c.a)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want judgment direct-call optional recursive alias accepted", result.Diagnostics)
	}
}

func TestRecursiveAliasMapIndexReceiverMethodReturn(t *testing.T) {
	result := Check(`
type Message = {
    _topic: string,
    topic: (self: Message) -> string,
}

local function make(): {[string]: Message}
    return {
        ["root"] = {
            _topic = "mapped",
            topic = function(self: Message): string
                return self._topic
            end,
        },
    }
end

local root = make()["root"]
if root then
    local topic: string = root:topic()
	end
`)
	if len(result.Diagnostics) != 0 {
		debug := "<no checked result>"
		if result.checked != nil && result.checked.RootResult() != nil {
			debug = callOutcomeDebug(result.checked.RootResult())
		}
		t.Fatalf("diagnostics = %#v, want guarded recursive alias map-index receiver method to return string; calls: %s", result.Diagnostics, debug)
	}
}

func TestDirectCallArgumentRejectsWrongRecursiveAliasFamily(t *testing.T) {
	src := strings.TrimLeft(`
type A = { b: B?, tag: "a" }
type B = { c: C?, tag: "b" }
type C = { a: A?, tag: "c" }

local function walk(a: A?): number
    return 0
end

local b: B = { c = nil, tag = "b" }
walk(b)
`, "\n")
	result := Check(src)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallArgType)
	requireLabelMessage(t, diag, "argument value")
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	for _, fragment := range []string{
		"error[type.call.direct.argument_type]: argument 1 (b) is {c:",
		" --> test.lua:10:6",
		"10 | walk(b)",
		"|      ↑ argument value",
		"1. proven: argument 1 (b) has type {c:",
		"2. claimed: walk parameter 1 expects A#",
		"3. missing proof: no proof on this path shows b satisfies the parameter type",
		"help: Pass `b` as a value compatible with the parameter type, or change the callee signature if that argument is valid.",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered diagnostic missing %q:\n%s", fragment, rendered)
		}
	}
}
