type TextEvent = {
    kind: "text",
    value: string,
}

type CountEvent = {
    kind: "count",
    value: number,
}

type Event = TextEvent | CountEvent

local event: Event = {kind = "count", value = 4}

if event.kind ~= "text" then
    local doubled: number = event.value * 2
else
    local text: string = event.value
end

return "ok"
