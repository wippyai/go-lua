package canonical_test

import (
	"testing"
)

func TestCapturedOptionalNarrowingRegression(t *testing.T) {
	cases := map[string]struct {
		src          string
		wantClean    bool
		wantContains string
	}{
		"closure-captured-optional-call": {
			wantClean: true,
			src: `
type Decorator = (string) -> string
local function build(decorator: Decorator?): (string) -> string
    return function(note: string): string
        if decorator then
            note = decorator(note)
        end
        return note
    end
end
return build
`,
		},
		"module-captured-early-return": {
			wantClean: true,
			src: `
type Services = { logger: string }
local M = {}
local _services: Services? = nil
function M.init(): Services
    local s: Services = { logger = "x" }
    _services = s
    return s
end
function M.get(): Services
    if not _services then
        return M.init()
    end
    return _services
end
return M
`,
		},
		"soundness-unguarded-captured-optional-call": {
			wantContains: "cannot call optional value without nil check",
			src: `
type Decorator = (string) -> string
local function build(decorator: Decorator?): (string) -> string
    return function(note: string): string
        return decorator(note)
    end
end
return build
`,
		},
		"soundness-local-from-map-read-optional-call": {
			wantContains: "cannot call optional value without nil check",
			src: `
type Handler = (string) -> string
type App = { handlers: {[string]: Handler} }
local function run(app: App, key: string): string
    local handler = app.handlers[key]
    return handler("payload")
end
return run
`,
		},
		"soundness-local-from-method-registered-map-read-stays-optional": {
			wantContains: "cannot call optional value without nil check",
			src: `
type Handler = (string) -> string
type App = {
    handlers: {[string]: Handler},
    register: (self: App, name: string, handler: Handler) -> App,
}
local App = {}
function App:register(name: string, handler: Handler): App
    self.handlers[name] = handler
    return self
end
local function make_handler(value: string): string
    return value
end
local app: App = {
    handlers = {},
    register = App.register,
}
local chained = app:register("search", make_handler)
local handler = chained.handlers["search"]
return handler("payload")
`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.wantClean {
				requireCanonicalClean(t, tc.src)
				return
			}
			requireCanonicalDiagnosticContains(t, tc.src, tc.wantContains)
		})
	}
}
