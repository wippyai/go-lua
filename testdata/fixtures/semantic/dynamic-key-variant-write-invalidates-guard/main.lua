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
local key = "active"

if slots.active.value.kind == "file" then
    slots[key].value = {kind = "timer", seconds = 20}
    local stale_path: string = slots.active.value.path -- expect-error
end
