-- A field of a captured record local keeps its resolved type inside a nested function.
local function nz(v: any): number return tonumber(v) or 0 end
local function need(n: number): number return n end

local function closure_plain(x: any): any
    local B = { a = nz(x) }
    local f = function(): number return need(B.a) end
    return f()
end

local function closure_literal(): any
    local B = { a = 5 }
    local f = function(): number return need(B.a) end
    return f()
end

return { closure_plain = closure_plain, closure_literal = closure_literal }
