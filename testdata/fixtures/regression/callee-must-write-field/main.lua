local function fill(box)
    box.result = { value = "ready" }
end

local function maybe_fill(box, flag: boolean)
    if flag then
        box.optional = { value = "possible" }
    end
end

local function fill_on_both_paths(box, flag: boolean)
    if flag then
        box.both = { value = "left" }
    else
        box.both = { value = "right" }
    end
end

local function fill_then_clear(box)
    box.cleared = { value = "gone" }
    box.cleared = nil
end

local function read(flag: boolean)
    local box = {}
    fill(box)
    local value: string = box.result.value
    fill_on_both_paths(box, flag)
    local either: string = box.both.value
    maybe_fill(box, false)
    local missing: string = box.optional.value
    fill_then_clear(box)
    local cleared: string = box.cleared.value -- expect-error: cannot assign
    local declared: { value: { text: string }? } = {}
    local text: string = declared.value.text -- expect-error: cannot assign string? to string
    return value .. either
end

return read(true)
