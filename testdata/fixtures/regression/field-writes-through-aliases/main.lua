-- Fields a function may write on a table reached through a parameter or a
-- captured variable become optional fields of that table once the table
-- reaches the function.
local function mark(t)
    t.done = true
end

local marked = { n = 0 }
mark(marked)
local done: boolean? = marked.done

local registry = {}
local function register(api: any)
    table.insert(registry, api)
end

local counters = { hits = 0 }
register({
    hit = function()
        counters.hits = counters.hits + 1
        counters.label = "hit"
    end
})
local label: string? = counters.label
local hits: integer = counters.hits

local function make_writer(target)
    return function(v: string)
        target.last = v
    end
end

local function install(target)
    register(make_writer(target))
end

local sink = { size = 0 }
install(sink)
local last: string? = sink.last
local size: integer = sink.size

local function make_recorder(target)
    return {
        record = function(v: any)
            target.seen = v
            target.seen = nil
        end,
    }
end

local log = { n = 0 }
make_recorder(log)
local first = log.seen[1]

local untouched = { n = 0 }
mark(sink)
local missing = untouched.done -- expect-error: does not exist

return { done, label, hits, last, size, first, missing }
