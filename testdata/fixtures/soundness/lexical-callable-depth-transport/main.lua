-- WIR binds a captured symbol where it is read, so a body that only constructs
-- a closure over a chunk-level `local function` holds no binding for it, and a
-- body that receives such a callable holds no binding for that callable's own
-- environment. Both capabilities have to reach the descendant's entry: without
-- them the depth-2 call site dispatches through the opaque boundary and the
-- callee's parameter contract never runs.

local function scaled(factor: number): number
    return factor * 2
end

-- The intermediate body constructs the closure that reads `scaled`. It never
-- reads the symbol itself, so `scaled` is a capture of the descendant alone.
local function through_nested_closure(): number
    local function apply_scale(): number
        return scaled("two") -- expect-error: argument 1 is "two", not number
    end
    return apply_scale()
end

-- `forwarding` runs only from `through_captured_callable`, whose entry receives
-- the capability but binds nothing `forwarding` itself reads.
local function forwarding(): number
    return scaled("three") -- expect-error: argument 1 is "three", not number
end

local function through_captured_callable(): number
    return forwarding()
end

-- Correct use through the same two boundaries keeps the call site clean.
local function correct(): number
    local function apply_scale(): number
        return scaled(2)
    end
    return apply_scale()
end

local a: number = through_nested_closure()
local b: number = through_captured_callable()
local c: number = correct()
return a, b, c
