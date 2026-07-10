local function stream_id(value: stream.Stream): string
    return value.id
end

local opened: stream.Stream = stream.open("events")
local id: string = stream_id(opened)
local wrong_id: number = opened.id

return id
