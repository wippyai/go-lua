type FileSlot = {
    kind: "file",
    path: string,
}

type TimerSlot = {
    kind: "timer",
    seconds: number,
}

type Slot = {
    value: FileSlot | TimerSlot,
}

type Slots = {[string]: Slot}

local slots: Slots = {
    active = {
        value = {kind = "file", path = "/tmp/active"},
    },
}

if slots.active.value.kind == "file" then
    local before: string = slots["active"].value.path
    slots["active"].value = {kind = "timer", seconds = 10}
    local stale_path: string = slots.active.value.path -- expect-error
    local stale_seconds: number = before -- expect-error
end

return "ok"
