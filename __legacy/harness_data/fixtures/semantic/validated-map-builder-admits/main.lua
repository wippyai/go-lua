local function collect_tags(raw: any): {[string]: string}
    local out: {[string]: string} = {}
    if type(raw) ~= "table" then
        return out
    end
    for key, value in pairs(raw) do
        if type(key) == "string" and type(value) == "string" then
            out[key] = value
        end
    end
    return out
end

local tags = collect_tags({owner = "ops", retry = 3})
local owner = tags.owner
if owner then
    local owner_name: string = owner
end

return "ok"
