package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZMetaOopProbe reproduces metatable-oop counter.lua:52 in isolation:
// a method-typed handler param passed where EventHandler is expected.
func TestZZMetaOopProbe(t *testing.T) {
	cases := map[string]string{
		// Single-module analogue: EventHandler defined locally, handler param
		// references the SAME local EventEmitter type.
		"single-module-same-type": `
type EventHandler = (self: EventEmitter, event: string, data: any) -> ()
type EventEmitter = {
    _handlers: {[string]: {EventHandler}},
    on: (self: EventEmitter, event: string, handler: EventHandler) -> EventEmitter,
    emit: (self: EventEmitter, event: string, data: any) -> (),
}
local EventEmitter = {}
EventEmitter.__index = EventEmitter
function EventEmitter:on(event: string, handler: EventHandler): EventEmitter
    return self
end
local function caller(e: EventEmitter, h: (self: EventEmitter, event: string, data: any) -> ())
    e:on("change", h)
end
return caller
`,
	}
	for name, src := range cases {
		mod := testutil.CheckAndExport(src, "mo_"+name)
		t.Logf("=== %s: %d errors ===", name, len(mod.Errors))
		for _, d := range mod.Errors {
			t.Logf("   %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
		}
	}
}
