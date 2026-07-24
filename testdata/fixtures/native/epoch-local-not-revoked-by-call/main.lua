-- Precision: a narrowing on a local that no closure captures survives an opaque call. The
-- callee cannot reach the binding, so call.opaque is not in the revocation set and the JIT
-- must not re-guard after the call.

local function run(x: number?, notify: () -> ()): number
    if x == nil then
        return 0
    end
    notify()
    return x + 1
end

return run
