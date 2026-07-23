local function use(value: string): string
    return value
end

local function run(ok: boolean): string?
    local tag: "idle" | "ready" = "idle"
    local payload: string?

    if ok then
        tag = "ready"
        payload = "body"
    end

    if tag == "ready" then
        return use(payload)
    end
    return nil
end

return run
