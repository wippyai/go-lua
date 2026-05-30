package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZGenRetProbe isolates the gradual-typing-adversarial generic-return root:
// a local generic ok<T>(value:T):Validation<T> returned where Validation<Config>
// is expected. Read-only probe.
func TestZZGenRetProbe(t *testing.T) {
	cases := map[string]string{
		"ok-record-arg": `
type Config = { id: string, retries: number }
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function invalid<T>(message: string): Validation<T>
    return {ok = false, error = message}
end
local function decode(raw: any): Validation<Config>
    if type(raw) ~= "table" then
        return invalid("root")
    end
    return ok({ id = "x", retries = 1 })
end
return decode
`,
		"ok-direct-config": `
type Config = { id: string, retries: number }
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function decode(): Validation<Config>
    return ok({ id = "x", retries = 1 })
end
return decode
`,
	}
	opt := testutil.WithCheckOption(check.WithCanonicalFlow())
	for name, src := range cases {
		mod := testutil.CheckAndExport(src, "g_"+name, opt)
		t.Logf("=== %s: %d errors ===", name, len(mod.Errors))
		for _, d := range mod.Errors {
			t.Logf("   %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
		}
	}
}
