local mid = require("mid")

local function local_path(): string
    local box: mid.Boxed = mid.make("local")
    return box.body
end

local function mixed_path(cond: boolean)
    local box: mid.Boxed = mid.make("mixed")
    if cond then
        process.send("worker", "topic", box)
    else
        local body: string = box.body
        print(body)
    end
end

local r: string = local_path()
mixed_path(true)
mixed_path(false)
print(r)
