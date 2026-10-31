local function gen(): (string?, string?)
    return "x", nil
end

local function id<T>(x: T): T
    return x
end

local function run()
    local name, err = gen()
    if not name then
        return nil
    end
    local same = id(name)
    local s: string = same
    local ch = process.listen(name)
    local raw: channel.Channel<any> = ch
    local wrong: number = same -- expect-error: cannot assign string to number
    return s, raw
end

local function nested()
    local name = gen()
    if name then
        local same = id(name)
        local s: string = same
        return s
    end
    return nil
end

return run, nested
