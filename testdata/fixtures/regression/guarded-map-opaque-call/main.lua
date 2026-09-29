type Map = { [string]: unknown }
local opaque: any = require("opaque")
local expected: { [string]: Map } = {}
expected["known"] = { view = "app:known" }
opaque.mutate(expected)
local key: string = "other"
local want = expected[key]
if want ~= nil then
    local function get(_id: string): unknown return {} end
    return get(want.view) -- expect-hint: argument 1: implicit unknown flows into declared string
end
return nil
