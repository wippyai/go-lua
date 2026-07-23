local variants = require("variants")

local function make(ok: boolean)
    if ok then
        return { kind = "ok", value = 42 }
    end
    return { kind = "err", message = "boom" }
end

local r = make(true)
if r.kind == "ok" then
    local v: number = r.value
else
    local m: string = r.message
end

local unguarded = make(false)
local missing: number = unguarded.value -- expect-error

local function make_multi(mode: string)
    if mode == "number" then
        return { kind = "ok", value = 1 }
    elseif mode == "string" then
        return { kind = "ok", value = "cached" }
    end
    return { kind = "err", message = "missing" }
end

local multi = make_multi("number")
if multi.kind == "ok" then
    local v: number | string = multi.value
else
    local m: string = multi.message
end

local imported = variants.make(true)
if imported.kind == "ok" then
    local v: number = imported.value
else
    local m: string = imported.message
end
