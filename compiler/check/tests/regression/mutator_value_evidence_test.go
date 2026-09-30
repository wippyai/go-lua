package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func withChannel() testutil.Option {
	return testutil.WithManifest("channel", testutil.ChannelManifest())
}

// Values passed to container mutators carry the inferred types of the locals
// they read.
func TestChannelSendOfCapturedCounterKeepsDeclaredElement(t *testing.T) {
	source := `
local channel = require("channel")
type Channel = channel.Channel
type Activation = {serial: integer, error: string?, title: string?}
local function run(): Channel<Activation>?
    local activations = channel.new(1) :: Channel<Activation>
    local serial = 0
    local function finish(): Channel<Activation>?
        serial = serial + 1
        return activations
    end
    activations:send({serial = serial, error = "setup failed"})
    activations:send({serial = serial, title = "app"})
    return finish()
end
return run
`
	checkBothModes(t, source, "", withChannel())
}

func TestChannelSendOfLocalReportsSolvedElement(t *testing.T) {
	source := `
local channel = require("channel")
type Channel = channel.Channel
local function run(): Channel<{n: string}>
    local n = 0
    local ch = channel.new(1)
    ch:send({n = n})
    return ch
end
return run
`
	checkBothModes(t, source, "cannot return channel.Channel<{n: integer}>, expected channel.Channel<{n: string}>", withChannel())
}

func TestTableInsertOfLocalReportsSolvedElement(t *testing.T) {
	source := `
local function run(): {{n: string}}
    local n = 0
    local t = {}
    table.insert(t, {n = n})
    return t
end
return run
`
	checkBothModes(t, source, "cannot return {n: 0}[], expected {n: string}[]")
}

func TestChannelSendOfMismatchedLocalFieldIsReported(t *testing.T) {
	source := `
local channel = require("channel")
type Channel = channel.Channel
local function run(): Channel<{serial: integer}>
    local s = "x"
    local ch = channel.new(1) :: Channel<{serial: integer}>
    ch:send({serial = s})
    return ch
end
return run
`
	checkBothModes(t, source, "cannot return channel.Channel<{serial: integer | string}>", withChannel())
}

func TestMutatorDynamicIndexValue(t *testing.T) {
	for _, tt := range []struct{ name, write, result string }{
		{"insert", "table.insert(out, messages[i])", "{integer}"},
		{"index", "out[i] = messages[i]", "{integer}"},
		{"send", "out:send(messages[i])", "Channel<integer>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			init := "{}"
			if tt.name == "send" {
				init = "channel.new(1)"
			}
			code := `local channel = require("channel")
type Channel = channel.Channel
local function run(): ` + tt.result + `
    local function load(): {integer} return {1, 2, 3} end
    local messages = load()
    local out = ` + init + `
    for i = #messages, 1, -1 do
        ` + tt.write + `
    end
    return out
end
return run`
			checkBothModes(t, code, "", withChannel())
		})
	}
}

func TestMutatorNestedLiteralValue(t *testing.T) {
	for _, tt := range []struct{ name, write, result string }{
		{"insert", "table.insert(out, {a = {b = n}})", "{{a: {b: integer}}}"},
		{"index", "out[i] = {a = {b = n}}", "{{a: {b: integer}}}"},
		{"send", "out:send({a = {b = n}})", "Channel<{a: {b: integer}}>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			init := "{}"
			if tt.name == "send" {
				init = "channel.new(1)"
			}
			code := `local channel = require("channel")
type Channel = channel.Channel
local function run(): ` + tt.result + `
    local n = 1
    local out = ` + init + `
    for i = 1, 2 do
        ` + tt.write + `
    end
    return out
end
return run`
			checkBothModes(t, code, "", withChannel())
		})
	}
}

func TestMutatorFallbackPreservesKnownFields(t *testing.T) {
	checkBothModes(t, `local channel = require("channel")
type Channel = channel.Channel
local function run(): Channel<{serial: integer, title: string}>
    local n = 1
    local out = channel.new(1)
    out:send({serial = n, title = "known"})
    return out
end
return run`, "", withChannel())
}

func TestMutatorFlowEvidenceOverridesFallback(t *testing.T) {
	checkBothModes(t, `local channel = require("channel")
type Channel = channel.Channel
local function run(value: any): Channel<{serial: string}>
    local out = channel.new(1)
    if type(value) == "string" then
        out:send({serial = value})
    end
    return out
end
return run`, "", withChannel())
}

func TestMutatorValueEvidenceRejectsMismatches(t *testing.T) {
	for _, tt := range []struct{ name, value, result, setup string }{
		{"dynamic insert", "messages[i]", "{integer}", ""},
		{"nested insert", "{a = {b = s}}", "{{a: {b: integer}}}", ""},
		{"fallback insert", "{a = {b = s}, title = 'known'}", "{{a: {b: integer}, title: string}}", ""},
		{"dynamic send", "messages[i]", "Channel<integer>", "send"},
		{"nested send", "{a = {b = s}}", "Channel<{a: {b: integer}}>", "send"},
		{"fallback send", "{a = {b = s}, title = 'known'}", "Channel<{a: {b: integer}, title: string}>", "send"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			init, write := "{}", "table.insert(out, "+tt.value+")"
			if tt.setup == "send" {
				init, write = "channel.new(1)", "out:send("+tt.value+")"
			}
			checkBothModes(t, `local channel = require("channel")
type Channel = channel.Channel
local function run(): `+tt.result+`
    local function load(): {string} return {"x"} end
    local messages = load()
    local s = "x"
    local out = `+init+`
    for i = 1, #messages do
        `+write+`
    end
    return out
end
return run`, "cannot return", withChannel())
		})
	}
}

func TestMutatorNestedSequenceValue(t *testing.T) {
	checkBothModes(t, `
local function run(): {integer}
    local n = 1
    local out = {}
    table.insert(out, {a = {{b = n}, {b = n}}})
    return {out[1].a[1].b, out[1].a[2].b}
end
return run`, "")
}

func TestMutatorReversedOptionalCallResult(t *testing.T) {
	checkBothModes(t, `
local function query(): ({ {[string]: any} }?, string?) return {}, nil end
local function run()
    local messages, err = query()
    if err then return nil, err end
    local reversed = {}
    for i = #messages, 1, -1 do
        table.insert(reversed, messages[i])
    end
    messages = reversed
    if #messages > 0 then
        return messages[1].message_id
    end
end
return run`, "")
}

func TestMutatorLiteralKeyReadKeepsFieldType(t *testing.T) {
	for _, tt := range []struct{ name, write, result, init string }{
		{"index", "out[i] = input[key]", "{integer}", "{}"},
		{"insert", "table.insert(out, input[key])", "{integer}", "{}"},
		{"send", "out:send({serial = input[key]})", "Channel<{serial: integer}>", "channel.new(1)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkBothModes(t, `local channel = require("channel")
type Channel = channel.Channel
local function run(key: "count"): `+tt.result+`
    local input = {count = 1, title = "x"}
    local out = `+tt.init+`
    for i = 1, 2 do
        `+tt.write+`
    end
    return out
end
return run`, "", withChannel())
		})
	}
}
