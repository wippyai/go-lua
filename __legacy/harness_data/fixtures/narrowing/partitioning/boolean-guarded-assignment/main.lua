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

    local untouched = 1
    untouched = untouched + 1

    if ok then
        return use(x)
    end
    return nil
end

return run
