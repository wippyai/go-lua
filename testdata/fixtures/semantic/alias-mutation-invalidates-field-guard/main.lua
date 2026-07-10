type Box = {
    value: string?,
}

local box: Box = {value = "ready"}
local alias = box

if box.value then
    alias.value = nil
    local after: string = box.value -- expect-error
end

return "ok"
