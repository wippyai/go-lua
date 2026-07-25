-- A direct call that omits a required argument must be rejected; otherwise the
-- callee could trust a nil slot as its declared number parameter.
local function g(x: number, y: number): number return x end
local function f(): number
    return g(1) -- expect-error
end

return f
