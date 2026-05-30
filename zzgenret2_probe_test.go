package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZGenRet2Probe narrows when the generic return degrades to the bare
// Validation<T> (uninstantiated) vs Validation<{record}>. Mirrors the real
// fixture argument shape (any-narrowed fields + prior generic-call results).
func TestZZGenRet2Probe(t *testing.T) {
	cases := map[string]string{
		"plain-literal-arg-with-guards": `
type Config = { id: string, retries: number, labels: {string}, metadata: {[string]: string} }
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function invalid<T>(message: string): Validation<T>
    return {ok = false, error = message}
end
local function decode(raw: any): Validation<Config>
    return ok({
        id = "x",
        retries = 1,
        labels = {} :: {string},
        metadata = {} :: {[string]: string},
    })
end
return decode
`,
		"miss-115-any-field-truthy": `
type Config = { id: string, retries: number }
local raw_config: any = {id = "worker", retries = 3}
local unchecked_config: Config = raw_config -- expect-error at decl
if raw_config.id then
    local id: string = raw_config.id -- expect-error: any-field truthy is not string
end
return raw_config
`,
		"miss-115-pure-any-param": `
local function f(raw_config: any)
    if raw_config.id then
        local id: string = raw_config.id -- expect-error
    end
    local id2: string = raw_config.id -- expect-error (no guard)
end
return f
`,
		"value-into-fn-expecting-stringarr": `
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function read_labels(value): Validation<{string}>
    return ok({} :: {string})
end
local function takes(x: {string}): {string} return x end
local function use(raw: any): {string}
    local labels = read_labels(raw)
    if not labels.ok then return {} end
    return takes(labels.value)
end
return use
`,
		"table-field-from-generic-value": `
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function read_labels(value): Validation<{string}>
    return ok({} :: {string})
end
local function use(raw: any): {labels: {string}}
    local labels = read_labels(raw)
    if not labels.ok then return {labels = {}} end
    local tbl = {labels = labels.value}
    return tbl
end
return use
`,
		"value-field-of-generic-result": `
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function read_labels(value): Validation<{string}>
    return ok({} :: {string})
end
local function use(raw: any): {string}
    local labels = read_labels(raw)
    if not labels.ok then return {} end
    return labels.value
end
return use
`,
		"arg-prior-generic-value": `
type Config = { id: string, retries: number, labels: {string}, metadata: {[string]: string} }
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function read_labels(value): Validation<{string}>
    return ok({} :: {string})
end
local function decode(raw: any): Validation<Config>
    local labels = read_labels(raw)
    return ok({
        id = "x",
        retries = 1,
        labels = labels.value,
        metadata = {} :: {[string]: string},
    })
end
return decode
`,
		"arg-from-any-fields": `
type Config = { id: string, retries: number, labels: {string}, metadata: {[string]: string} }
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end
local function invalid<T>(message: string): Validation<T>
    return {ok = false, error = message}
end
local function read_labels(value): Validation<{string}>
    return ok({} :: {string})
end
local function read_metadata(value): Validation<{[string]: string}>
    return ok({} :: {[string]: string})
end
local function decode(raw: any): Validation<Config>
    if type(raw) ~= "table" then return invalid("root") end
    if type(raw.id) ~= "string" then return invalid("id") end
    if type(raw.retries) ~= "number" then return invalid("retries") end
    local labels = read_labels(raw.labels)
    if not labels.ok then return invalid(labels.error) end
    local metadata = read_metadata(raw.metadata)
    if not metadata.ok then return invalid(metadata.error) end
    return ok({
        id = raw.id,
        retries = raw.retries,
        labels = labels.value,
        metadata = metadata.value,
    })
end
return decode
`,
	}
	opt := testutil.WithCheckOption(check.WithCanonicalFlow())
	for name, src := range cases {
		mod := testutil.CheckAndExport(src, "g2_"+name, opt)
		t.Logf("=== %s: %d errors ===", name, len(mod.Errors))
		for _, d := range mod.Errors {
			t.Logf("   %s:%d:%d %s", d.Position.File, d.Position.Line, d.Position.Column, d.Message)
		}
	}
}
