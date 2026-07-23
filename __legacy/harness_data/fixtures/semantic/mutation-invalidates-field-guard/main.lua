type Box = {
    value: string?,
}

local box: Box = {value = "ready"}

if box.value then
    local before: string = box.value
    box.value = nil
    local after: string = box.value -- expect-error
end

return "ok"
