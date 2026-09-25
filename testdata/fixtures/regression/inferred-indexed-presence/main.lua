-- Reduced from app:request_test in kickside/stat-analysis/test.
local log = {}
local function install_thread_stub()
    log = {}
    local function append(args: any)
        local t = log[args.thread_id] or {}
        log[args.thread_id] = t
        t[#t + 1] = { type = args.event.type, seq = #t + 1, body = args.event.body }
        return { seq = #t }, nil
    end
    return append
end
local append = install_thread_stub()
append({ thread_id = "t", event = { type = "query", body = { prompt = "describe the data", depth = 1 } } })
local events = log["t"]
local event_type: string = events[1].type
local prompt: string = events[1].body.prompt
local depth: number = events[1].body.depth

-- Reduced from app:association_pull_test in kickside/providers/hubspot/test.
local fake = { lists = {} }
function fake.list_objects(_conn: any, object_type: string, query: any): any
    fake.lists[#fake.lists + 1] = { object_type = object_type, query = query }
    return { success = true }
end
fake.list_objects(nil, "contacts", { after = "objects-1" })
fake.list_objects(nil, "contacts", { after = "objects-2" })
local after: string = fake.lists[2].query.after

-- Reduced from app:gmail_test in kickside/spiralscout/outreach/test.
local seen = {}
local gmail = { _tool_call = nil }
gmail._tool_call = function(_id: string, args: any)
    seen.args = args
    return "Draft created: id d-9", nil
end
local function create_draft()
    gmail._tool_call("write", { action = "create_draft", to = "a@b.co" })
end
create_draft()
local action: string = seen.args.action

local declared: { [string]: { value: string }? } = {}
local declared_value: string = declared["missing"].value -- expect-error

local explicit: { [string]: { value: string }? } = {}
explicit["missing"] = nil
local explicit_value: string = explicit["missing"].value -- expect-error

local maybe_item: { value: string }? = nil
local inferred_with_nil = {}
inferred_with_nil["maybe"] = maybe_item
local inferred_with_nil_value: string = inferred_with_nil["maybe"].value -- expect-error

local function read_after_optional_write(item: { value: string }?)
    local inferred = {}
    local function write()
        inferred["maybe"] = item
    end
    write()
    local value: string = inferred["maybe"].value -- expect-error
end

-- Reduced from userspace.dataflow.node.parallel:iterator_test and the
-- source-based iterator module in wippy/dataflow.
local iterator = require("iterator")
local redirected = iterator.redirect_terminals_to_parent({
    data_targets = {{ data_type = "node_output" }},
}, "parallel-parent", 3, "source-node", "attempt-1")
local target_key: string = redirected.data_targets[1].key
