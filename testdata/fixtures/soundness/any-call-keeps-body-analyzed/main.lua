-- Callability is a concrete requirement, so applying a value that came through
-- an any boundary is refuted on its own. That refutation must not silence the
-- rest of the body: the member assignment below still refutes at the same time.

local function apply_through_any(x: any): number
    local b: string = x.field -- expect-error
    x(1) -- expect-error
    return 1
end

-- A runtime type test proves the callable kind for the same value.
local function validated_call(x: any): number
    if type(x) == "function" then
        x(1)
    end
    return 1
end

-- A declared callable formal is proven callable without any test.
local function declared_call(f: fun(n: number)): number
    f(1)
    return 1
end

return apply_through_any, validated_call, declared_call
