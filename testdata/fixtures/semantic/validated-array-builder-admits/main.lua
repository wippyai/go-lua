local function collect_strings(raw: any): {string}
    local out: {string} = {}
    if type(raw) ~= "table" then
        return out
    end
    for _, item in ipairs(raw) do
        if type(item) == "string" then
            table.insert(out, item)
        end
    end
    return out
end

local values = collect_strings({"a", 2, "b"})
local first = values[1]
if first then
    local s: string = first
end

return "ok"
