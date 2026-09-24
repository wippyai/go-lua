type Map = { [string]: unknown }
local function entries(): { [string]: Map }
    return { other = { id = "without-view" } }
end
local expected: { [string]: Map } = entries()
expected["known"] = { view = "app:known" }
local key: string = "other"
local want = expected[key]
if want ~= nil then
    local function get(_id: string): unknown return {} end
    return get(want.view)
end
return nil
