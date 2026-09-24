type Map = { [string]: unknown }
local expected: { [string]: Map } = {}
local alias = expected
alias["other"] = { id = "without-view" }
expected["known"] = { view = "app:known" }
local key: string = "other"
local want = expected[key]
if want ~= nil then
    local function get(_id: string): unknown return {} end
    return get(want.view)
end
return nil
