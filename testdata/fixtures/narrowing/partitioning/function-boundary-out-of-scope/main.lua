local function load(): string
    return "loaded"
end

local function use(value: string): string
    return value
end

local function run(ok: boolean): string?
    local x: string?
    if ok then
        x = load()
    end

    local function later(): string?
        if ok then
            return use(x) -- expect-error
        end
        return nil
    end

    return later()
end

return run
