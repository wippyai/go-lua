package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallMultiReturn_JSONDecodeTypedTarget(t *testing.T) {
	json := io.NewManifest("json")
	json.SetExport(typ.NewRecord().Field("decode", typ.Func().
		Param("str", typ.String).
		OptParam("target", typ.NewMeta(typ.Any)).
		Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()).Build())
	req := io.NewManifest("req")
	req.SetExport(typ.NewRecord().Field("body", typ.Func().
		Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()).Build())
	result := testutil.Check(`
local json = require("json")
local req = require("req")
json.decode(req.body())
`, testutil.WithStdlib(), testutil.WithManifest("json", json), testutil.WithManifest("req", req))
	messages := testutil.ErrorMessages(result.Diagnostics)
	if len(messages) != 1 || messages[0] != "argument 2: expected typeof(any), got Error?" {
		t.Fatalf("expected the expanded body error at decode target, got %v", messages)
	}
}

func TestCallMultiReturnArguments(t *testing.T) {
	tests := []struct {
		name string
		code string
		want string
	}{
		{
			name: "json decode receives body error",
			code: `
type Error = {message: string}
local json = {}
function json.decode(str: string, target: string?): (any, Error?) return nil, nil end
local req = {}
function req.body(): (string, Error?) return "{}", nil end
json.decode(req.body())
`,
			want: "argument 2:",
		},
		{
			name: "optional parameter receives error",
			code: `
type Error = {message: string}
local function body(): (string, Error?) return "", nil end
local function decode(str: string, target: number?) end
decode(body())
`,
			want: "argument 2:",
		},
		{
			name: "variadic print accepts spread",
			code: `
local function pair(): (string, number) return "x", 1 end
print(pair())
string.format("%s %s", pair())
`,
		},
		{
			name: "typed variadic tail checks spread",
			code: `
local function take(a: string, ...: number) end
local function pair(): (string, string) return "x", "wrong" end
take(pair())
`,
			want: "argument 2:",
		},
		{
			name: "table insert selects three argument overload",
			code: `
local t: {string} = {}
local function pair(): (integer, string) return 1, "x" end
table.insert(t, pair())
`,
		},
		{
			name: "table insert checks position from spread",
			code: `
local t: {string} = {}
local function pair(): (string, string) return "wrong", "x" end
table.insert(t, pair())
`,
			want: "argument 2:",
		},
		{
			name: "any callee has unknown arity",
			code: `
local f: any = nil
local function pair(): (string, number) return "x", 1 end
f(pair())
`,
		},
		{
			name: "any returning call has unknown arity",
			code: `
local f: any = nil
local function need(a: string, b: number) end
need(f())
`,
		},
		{
			name: "any method returning call has unknown arity",
			code: `
local obj: any = nil
local function need(a: string, b: number) end
need(obj:run())
`,
		},
		{
			name: "declared single unknown return still leaves required argument missing",
			code: `
local function decode<T>(raw: string): T return raw :: T end
local function need(a: string, b: number) end
need(decode("x"))
`,
			want: "not enough arguments",
		},
		{
			name: "method call receives spread",
			code: `
local obj = {}
function obj:take(a: string, b: number) end
local function pair(): (string, string) return "x", "wrong" end
obj:take(pair())
`,
			want: "argument 2:",
		},
		{
			name: "nested trailing calls expand",
			code: `
type Error = {message: string}
local function h(): (string, number) return "x", 1 end
local function g(a: string, b: number): (string, Error?) return a, nil end
local function f(a: string, b: number?) end
f(g(h()))
`,
			want: "argument 2:",
		},
		{
			name: "non-final nested call is single valued",
			code: `
local function h(): (string, number) return "x", 1 end
local function g(a: string, b: number): (string, boolean) return a, true end
local function f(a: string, b: number) end
f(g(h()), 1)
`,
		},
		{
			name: "parenthesized call has one value",
			code: `
type Error = {message: string}
local function g(): (string, Error?) return "x", nil end
local function f(a: string, b: number?) end
f((g()))
`,
		},
		{
			name: "typed vararg has unknown count",
			code: `
local function need(a: string, b: number) end
local function forward(...: any) need(...) end
`,
		},
		{
			name: "typed vararg values are checked at the receiving parameter",
			code: `
local function need(a: string) end
local function forward(...: number) need(...) end
`,
			want: "argument 1:",
		},
		{
			name: "expanded values past declared parameters are dropped",
			code: `
local function take(a: string, b: number) end
local function three(): (string, number, boolean) return "x", 1, true end
take(three())
local upper = string.upper(("a b"):gsub(" ", "_"))
`,
		},
		{
			name: "expanded values landing on declared parameters are checked",
			code: `
local function take(a: string, b: string) end
local function pair(): (string, number) return "x", 1 end
take(pair())
`,
			want: "argument 2:",
		},
		{
			name: "explicit extra has same arity error",
			code: `
local function take(a: string, b: number) end
take("x", 1, true)
`,
			want: "too many arguments",
		},
		{
			name: "zero parameter function accepts expanded extra",
			code: `
local function take() end
local function pair(): (string, number) return "x", 1 end
take(pair())
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := testutil.Check(tt.code, testutil.WithStdlib())
			messages := testutil.ErrorMessages(result.Diagnostics)
			if tt.want == "" {
				if len(messages) != 0 {
					t.Fatalf("unexpected errors: %v", messages)
				}
				return
			}
			for _, message := range messages {
				if strings.Contains(message, tt.want) {
					return
				}
			}
			t.Fatalf("expected %q, got %v", tt.want, messages)
		})
	}
}
