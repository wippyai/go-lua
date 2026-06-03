package canonical_test

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"strings"
	"testing"
)

func TestCrossModuleRegisteredMapReadStaysOptionalAtCall(t *testing.T) {
	protocol := testutil.CheckAndExport(`
type Handler = (string) -> string
local M = {}
M.Handler = Handler
return M
`, "protocol", testutil.WithStdlib())
	if protocol.HasError() {
		t.Fatalf("protocol export failed: %v", testutil.ErrorMessages(protocol.Errors))
	}

	tools := testutil.CheckAndExport(`
local M = {}
function M.handle(value: string): string
    return value
end
return M
`, "tools", testutil.WithStdlib())
	if tools.HasError() {
		t.Fatalf("tools export failed: %v", testutil.ErrorMessages(tools.Errors))
	}

	engine := testutil.CheckAndExport(`
local protocol = require("protocol")

type App = {
    handlers: {[string]: protocol.Handler},
    register: (self: App, name: string, handler: protocol.Handler) -> App,
}

local App = {}
local M = {}

function M.new(): App
    local self: App = {
        handlers = {},
        register = App.register,
    }
    return self
end

function App:register(name: string, handler: protocol.Handler): App
    self.handlers[name] = handler
    return self
end

return M
`, "engine", testutil.WithStdlib(), testutil.WithModule("protocol", protocol))
	if engine.HasError() {
		t.Fatalf("engine export failed: %v", testutil.ErrorMessages(engine.Errors))
	}

	result := testutil.Check(`
local engine = require("engine")
local tools = require("tools")

local app = engine.new():register("search", tools.handle)
local handler = app.handlers["search"]
return handler("payload")
`, testutil.WithStdlib(),
		testutil.WithModule("protocol", protocol),
		testutil.WithModule("tools", tools),
		testutil.WithModule("engine", engine))

	msgs := testutil.ErrorMessages(result.Diagnostics)
	for _, msg := range msgs {
		if strings.Contains(msg, "cannot call optional value without nil check") {
			return
		}
	}
	t.Fatalf("expected optional-call diagnostic for exported registered map read, got %v", msgs)
}
