-- Negative control for the loop in app:component_packaging_test.
type Map = { [string]: unknown }
local opaque: any = require("opaque")
local function run(entries: { string })
    local expected: { [string]: Map } = {}
    expected["known"] = { view = "app:known" }
    for _, value in ipairs(entries) do
        opaque.mutate(expected)
        local want = expected[value]
        if want ~= nil then
            local function get(_id: string): unknown return {} end
            return get(want.view)
        end
    end
    return nil
end
return run
