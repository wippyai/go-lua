local function use(value: string): string
    return value
end

local function run(ok: boolean): string?
    local state: { tag: "idle" | "ready" } = { tag = "idle" }
    local payload: string?

    if ok then
        state.tag = "ready"
        payload = "body"
    else
        state.tag = "idle"
    end

    if state.tag == "ready" then
        return use(payload)
    end
    return nil
end

return run
