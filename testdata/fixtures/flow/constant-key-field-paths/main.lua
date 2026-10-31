local keys = require("keys")
local K = keys.KIND

-- Writes at constant keys define fields; reads through the same keys see them.
local handlers = {}
handlers[K.CREATE] = function(n: number): number return n + 1 end
handlers[K.UPDATE] = function(n: number): number return n * 2 end
handlers[K.DELETE] = function(n: number): number
    return handlers[K.UPDATE](n) - 1
end
local create = handlers[K.CREATE]
local created: number = create(1)
local deleted: number = handlers.DELETE(2)

-- A nested write through a constant key reaches the field path.
local slots = {}
slots[K.CREATE] = {}
slots[K.CREATE].count = 3
local count: number = slots[K.CREATE].count

-- A presence test through a constant key narrows the same slot.
local cache: {[string]: {n: number}} = {}
local function cached(): number
    if not cache[K.UPDATE] then
        cache[K.UPDATE] = { n = 1 }
    end
    return cache[K.UPDATE].n
end
local function peek(): number
    if cache[K.DELETE] then
        return cache[K.DELETE].n
    end
    return 0
end

print(created, deleted, count, cached(), peek())
