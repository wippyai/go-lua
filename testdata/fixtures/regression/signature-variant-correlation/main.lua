local lib = require("lib")

local function pick(which: "num" | "str")
    if which == "num" then
        return { kind = "num", value = 1 }
    end
    return { kind = "str", text = "x" }
end

local a = pick("num")
local v: number = a.value

local b = pick("str")
local t: string = b.text

local function nonliteral(flag: boolean)
    local mixed = pick(flag and "num" or "str")
    local unguarded: number = mixed.value -- expect-error
    return unguarded
end

local imported_num = lib.pick("num")
local imported_v: number = imported_num.value

local imported_str = lib.pick("str")
local imported_t: string = imported_str.text

local wrong_payload: string = pick("num").text -- expect-error

return v, t, imported_v, imported_t, nonliteral
