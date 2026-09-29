local KIND = { CREATE = "CREATE", UPDATE = "UPDATE" }

-- A closure captures a table filled at constant keys. The keys resolve only
-- once build sees its own captures, so the first iteration records the writes
-- under keys of unknown type; the resolved fields replace that shape.
local function build()
    local handlers = {}
    handlers[KIND.CREATE] = 1
    handlers[KIND.UPDATE] = function(n: number): number return n * 2 end
    local function run(): number
        return handlers.UPDATE(handlers.CREATE)
    end
    return handlers, run
end

local handlers, run = build()
print(handlers.CREATE, run())
