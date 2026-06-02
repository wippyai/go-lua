package canonical_test

import "testing"

func TestCanonicalCleanPrecisionRegressions(t *testing.T) {
	cases := map[string]string{
		"enum-router-method-return": `
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH"
type StatusCode = 200 | 201 | 400 | 404 | 500
type Request = {
    method: HttpMethod,
    path: string,
}
type Response = { status: StatusCode }
type Route = {
    method: HttpMethod,
    path: string,
    handler: (req: Request) -> Response,
}
type Router = {
    _routes: {Route},
    handle: (self: Router, req: Request) -> Response,
}
local function mkok(): Response return { status = 200 } end
local function mkerr(): Response return { status = 404 } end
local function handle(self: Router, req: Request): Response
    for _, route in ipairs(self._routes) do
        if route.method == req.method and route.path == req.path then
            return route.handler(req)
        end
    end
    return mkerr()
end
local rt: Router = { _routes = {}, handle = handle }
local resp = rt:handle({ method = "GET", path = "/x" })
local s: number = resp.status
`,
		"output-label-discriminant-chain": `
type RenderOutput = { kind: "rendered", body: string, label: string? }
type IndexOutput = { kind: "indexed", count: integer }
type AuditOutput = { kind: "audited", note: string, retry_after: string? }
type Output = RenderOutput | IndexOutput | AuditOutput
local M = {}
function M.output_label(output: Output): string
    if output.kind == "rendered" then
        return output.body
    end
    if output.kind == "indexed" then
        return tostring(output.count)
    end
    return output.note
end
return M
`,
		"callback-nested-return": `
local function outer(f: (number) -> number)
    return f(10)
end
local result: number = outer(function(x: number): number
    return x * 2
end)
return result
`,
		"generic-identity-closes-from-product-call-context": `
local function identity<T>(x: T): T
    return x
end
local s: string = identity("test")
return s
`,
		"same-body-closures-keep-distinct-captured-envs": `
local function make(v)
    return function()
        return v
    end
end
local get_string = make("s")
local get_number = make(1)
local s: string = get_string()
local n: number = get_number()
return s, n
`,
		"service-locator-early-return": `
type Services = { name: string }
local _services: Services? = nil
local function init(): Services
    local s = { name = "x" }
    _services = s
    return s
end
local function get(): Services
    if not _services then
        return init()
    end
    return _services
end
return get
`,
		"while-loop-counter": `
local function count_items(items)
    local count = 0
    while items do
        count = count + 1
    end
    local exact: number = count
    return exact
end
return count_items
`,
		"branch-field-call-return-with-pending-args": `
type Response = { status_code: number, body: string? }

local http_client = {}

function http_client.get(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

function http_client.post(url: string, options: any?): (Response?, string?)
    return nil, "not implemented"
end

local client = {
    _http_client = http_client
}

function client.request(method, url, http_options)
    local response, err
    if method == "GET" then
        response, err = client._http_client.get(url, http_options)
    else
        response, err = client._http_client.post(url, http_options)
    end
    if not response then
        return nil, err
    end
    local body: string? = response.body
    return body, nil
end
`,
		"pairs-self-write-preserves-closed-fields": `
local item = {
    count = 1,
    name = "ready",
}

for key, value in pairs(item) do
    item[key] = value
end

local count: number = item.count
local name: string = item.name
return count, name
`,
		"captured-map-dynamic-write-element-survives": `
local state = {sessions = {}}

local function add(id: string, started: number)
    state.sessions[id] = {created_at = started, last_activity = started}
end

local function sweep()
    for _, s in pairs(state.sessions) do
        local t: number = s.last_activity
        local c: number = s.created_at
    end
end

add("a", 1.0)
sweep()
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}
