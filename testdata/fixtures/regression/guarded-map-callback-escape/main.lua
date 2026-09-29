type Map = { [string]: unknown }
local function run(mutate: (t: { [string]: Map }) -> (), key: string)
    local expected: { [string]: Map } = {}
    expected["known"] = { view = "app:known" }
    mutate(expected)
    local want = expected[key]
    if want ~= nil then
        local function get(_id: string): unknown return {} end
        return get(want.view)
    end
    return nil
end
return run
