-- In the x == false branch, x is false, not string. The branch assignment
-- must be rejected so false cannot be trusted as a string by compiled code.
local function f(x: string | false): string
    if x == false then
        local s: string = x -- expect-error
        return s
    end
    return "y"
end

return f
