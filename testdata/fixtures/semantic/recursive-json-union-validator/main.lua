type Json = nil | boolean | number | string | {Json} | {[string]: Json}

local function is_scalar(value: Json): boolean
    return value == nil or type(value) == "boolean" or type(value) == "number" or type(value) == "string"
end

local doc: Json = {
    title = "root",
    children = {
        {title = "leaf", value = 1},
        {title = "flag", value = true},
    },
}

if not is_scalar(doc) and type(doc) == "table" then
    local title = doc.title
    if type(title) == "string" then
        local text: string = title
    end
end

local bad: string = doc -- expect-error

return "ok"
