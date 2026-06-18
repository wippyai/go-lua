local function need(s: string): string return s end
local function f(v: any): string
    return need(v :: string)
end
return f("x")
