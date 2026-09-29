local function gen(): string?
    return "x"
end

local function id<T>(x: T): T
    return x
end

local function run()
    local name = gen()
    if name then
        name = nil
        local after = id(name)
        local s: string = after -- expect-error: cannot assign
        return s
    end
    return nil
end

return run
