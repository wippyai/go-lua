local function fetch(ok: boolean): (string?, string?)
    if ok then
        return "payload", nil
    end
    return nil, "failed"
end

local function run(ok: boolean): string
    local value, err = fetch(ok)
    if err == nil then
        local s: string = value
        return s
    end
    return err
end

return run
