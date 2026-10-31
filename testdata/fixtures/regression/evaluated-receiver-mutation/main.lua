type Obj = { m: (self: Obj) -> boolean }
local function run(x: Obj?)
    local function clear(): integer
        x = nil
        return 0
    end
    x:m()
    clear()
    return x:m()
end
return run
