local function need(s: string): string return s end
local function f(v: any): string
    -- A cast adopts its target type for inference, but it does not prove a
    -- parameter contract: the any value cannot satisfy the string parameter.
    return need(v :: string) -- expect-error: is any, not string
end
return f("x")
