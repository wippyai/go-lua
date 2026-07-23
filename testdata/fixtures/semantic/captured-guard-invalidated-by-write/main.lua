local raw: any = {id = "cfg"}

local function read_id()
    return raw.id
end

if type(raw.id) == "string" then
    local before: string = read_id()
    raw.id = 7
    local after: string = read_id() -- expect-error
end

return "ok"
