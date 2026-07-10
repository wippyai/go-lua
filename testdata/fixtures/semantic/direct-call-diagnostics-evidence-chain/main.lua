local function encode(id: string, retries: number): string
    return id
end

encode(42, "bad")
encode("ok")

local target: number = 1
target()

return "ok"
