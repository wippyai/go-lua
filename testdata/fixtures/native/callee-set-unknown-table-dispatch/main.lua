-- Contract: a call through a table lookup keyed by a runtime string has no
-- statically provable callee; the callee set is Unknown and must not be narrowed
-- to the literal handler set.

type Handlers = { [string]: () -> number }

local handlers: Handlers = {
    up = function(): number return 1 end,
    down = function(): number return -1 end,
}

local function dispatch(key: string): number
    local handler = handlers[key]
    if handler == nil then
        return 0
    end
    return handler()
end

return dispatch
