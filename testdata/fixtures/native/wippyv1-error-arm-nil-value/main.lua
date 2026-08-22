-- Contract: every fallible member of the Wippy v1 host surface answers nil for
-- its value on the arm that reports an error. The runtime states it directly:
-- runtime/lua/modules/store/store.go pushes nil alongside invalidError("store
-- is released"), runtime/lua/modules/process/module.go answers
-- pushProcessError(l, lua.LNil, ...), and the json, expr and http modules each
-- push lua.LNil before their error. So a caller that reads the value without
-- first testing the error reads a nil, and a caller that tests the error first
-- does not.
--
-- Each member below is read twice, once on each side of that test. The
-- unguarded read is the production harm; the guarded read is the idiom the
-- whole surface is shaped around and must stay silent.

local store = require("store")
local process = require("process")
local json = require("json")
local expr = require("expr")
local http = require("http")

-- store.get answers nil for an empty or unresolvable resource id.
local raw_handle = store.get("")
local raw_released: boolean = raw_handle:release() -- expect-error
local released: boolean = false
local handle, open_err = store.get("app.test.store:memory")
if open_err == nil then
    released = handle:release()
end

-- Every Store accessor answers nil once the handle is released.
local raw_present: boolean = handle:has("alpha") -- expect-error
local present: boolean = false
local found, has_err = handle:has("alpha")
if has_err == nil then
    present = found
end

-- process.send answers nil when the destination is gone or the topic is
-- rejected.
local raw_delivered: boolean = process.send("worker", "job", "alpha") -- expect-error
local delivered: boolean = false
local sent, send_err = process.send("worker", "job", "alpha")
if send_err == nil then
    delivered = sent
end

-- json.decode answers nil for text that is not a document.
local raw_document: string = json.decode("{") -- expect-error
local document: string = ""
local decoded, decode_err = json.decode("{}")
if decode_err == nil then
    document = tostring(decoded)
end

-- expr.eval answers nil for source it cannot evaluate.
local raw_value: string = expr.eval("(") -- expect-error
local value: string = ""
local evaluated, eval_err = expr.eval("label")
if eval_err == nil then
    value = tostring(evaluated)
end

-- http.request answers nil outside a request context.
local raw_request = http.request()
local raw_path: string = raw_request:path() -- expect-error
local path: string = ""
local request, request_err = http.request()
if request_err == nil then
    local declared, path_err = request:path()
    if path_err == nil then
        path = declared
    end
end

if not (released and present and delivered and raw_released and raw_present and raw_delivered) then
    return "incomplete"
end
return document .. value .. path .. raw_document .. raw_value .. raw_path
