local function first(): string
    return "a"
end

local function second(): string
    return "b"
end

local function run(p: boolean, q: boolean, r: boolean): string
    local x: string? = nil
    if p then
        x = first()
    end
    if q then
        x = second()
    end
    if r then
        x = "c"
    end
    return x:upper()
end

return run
