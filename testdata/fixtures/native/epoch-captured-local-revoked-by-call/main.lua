-- The same shape as the uncaptured precision case, but the local is captured by a closure
-- handed to the opaque callee: the narrowing is now revoked, and its revocation set names
-- both call.opaque and write.upvalue.
type Thunk = () -> ()

local function run(x: number?, notify: (Thunk) -> ()): number
    if x == nil then
        return 0
    end
    local before = x + 1
    local function clear()
        x = nil
    end
    notify(clear)
    return before + (x or 0)
end

return run
