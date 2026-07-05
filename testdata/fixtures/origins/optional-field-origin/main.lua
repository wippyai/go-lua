type Cfg = { name: string, hook: string? }

local function run(c: Cfg): number
    return c.hook:len()
end

return run
