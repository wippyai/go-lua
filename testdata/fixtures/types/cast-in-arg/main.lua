local function need(s: string): string return s end
local function f(v: any): string
    -- Scalar casts are runtime validations; if this expression returns, v is string.
    return need(v :: string)
end
return f("x")
