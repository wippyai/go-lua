local function get(ok: boolean): (string?, boolean?)
    if ok then return "value", nil end
    return nil, false
end

local value, err = get(false)
if not err then
    local required: string = value -- expect-error: cannot assign
end
