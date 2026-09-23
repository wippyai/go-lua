package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// fsLikeManifest defines module types whose names collide with a Lua builtin
// (type) and with names programs use for their own values (Scanner).
func fsLikeManifest() *io.Manifest {
	m := io.NewManifest("fs")
	m.DefineType("type", typ.NewRecord().Field("DIR", typ.String).Field("FILE", typ.String).Build())
	m.DefineType("Scanner", typ.NewInterface("fs.Scanner", []typ.Method{
		{Name: "next", Type: typ.Func().Param("self", typ.Self).Returns(typ.Boolean).Build()},
	}))
	m.SetExport(typ.NewRecord().Field("read", typ.Func().Param("path", typ.String).Returns(typ.String).Build()).Build())
	return m
}

func checkWithFS(t *testing.T, source string) {
	t.Helper()
	res := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("fs", fsLikeManifest()))
	for _, d := range res.Errors {
		t.Errorf("line %d: %s", d.Position.Line, d.Message)
	}
}

// Module types are bound for type positions only: the builtin type() stays
// the typeof function even when a module defines a type named type.
func TestModuleTypeNamedLikeBuiltinDoesNotRetypeTypeCall(t *testing.T) {
	checkWithFS(t, `
local function canonical(value: unknown): string
    local kind = type(value)
    if kind == "string" then return "s" .. value end
    return kind
end

local function direct(value: unknown): string
    if type(value) == "string" then return "s" .. value end
    return ""
end

return { canonical = canonical, direct = direct }
`)
}

// A table named like a module type keeps its own methods and fields.
func TestModuleTypeNameDoesNotRetypeProgramValues(t *testing.T) {
	checkWithFS(t, `
local Scanner = { text = "abc", pos = 0 }

function Scanner.new(text: string)
    return { text = text, pos = 0 }
end

function Scanner:peek(): string
    return string.sub(self.text, self.pos + 1, self.pos + 1)
end

local s = Scanner.new("abc")
local text: string = s.text
local c: string = Scanner:peek()
return text .. c
`)
}

// The type annotation still resolves to the module type.
func TestModuleTypeResolvesInTypePositions(t *testing.T) {
	res := testutil.Check(`
local function step(s: Scanner): boolean
    return s:next()
end
local function wrong(s: Scanner): string
    return s:next()
end
return { step = step, wrong = wrong }
`, testutil.WithStdlib(), testutil.WithManifest("fs", fsLikeManifest()))
	if len(res.Errors) != 1 {
		t.Fatalf("want one error for returning boolean as string, got %v", res.Errors)
	}
}
