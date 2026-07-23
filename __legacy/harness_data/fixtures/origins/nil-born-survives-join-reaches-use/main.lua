local function compute(): string
    return "hello"
end

local function run(flag: boolean): string
    local x: string? = nil
    if flag then
        x = compute()
    end
    return x:upper()
end

return run
