type FileValue = {
    kind: "file",
    detail: {path: string},
}

type TimerValue = {
    kind: "timer",
    detail: {seconds: number},
}

type Value = FileValue | TimerValue

type Slot = {
    value: Value?,
    note: string?,
}

type Slots = {[string]: Slot}

local slots: Slots = {
    active = {
        value = {kind = "file", detail = {path = "/tmp/active"}},
    },
}

if slots.active and slots.active.value and slots.active.value.kind == "file" then
    local path: string = slots.active.value.detail.path
    local seconds: number = slots.active.value.detail.seconds -- expect-error
    local note: string = slots.active.note -- expect-error
    local standby: Slot = slots.standby -- expect-error
end

return "ok"
