type Map = { [string]: unknown }
local function run(key: string)
    local expected: { [string]: Map } = {}
    expected[tostring(1)] = { view = "app:known" }
    local function mutate()
        expected[tostring(2)] = { id = "without-view" }
    end
    mutate()
    local want = expected[key]
    if want ~= nil then
        local function get(_id: string): unknown return {} end
        return get(want.view)
    end
    return nil
end
return run
