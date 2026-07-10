type Box = {
    value: string?,
}

local box: Box = {value = "ready"}
local alias = box
local key = "value"

if box.value then
    alias[key] = nil
    local after: string = box.value -- expect-error
end

return "ok"
