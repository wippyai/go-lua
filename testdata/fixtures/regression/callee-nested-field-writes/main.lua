-- Writes a function makes into a table below a captured variable or a
-- parameter reach that table in the caller.
local M = {}
M.state = { clears = {} }

function M.reset(values: {[string]: any})
    M.state = { clears = {} }
    for k, v in pairs(values) do
        M.state[k] = v
    end
end

function M.nest()
    M.state.inner = { n = 1 }
    M.state.inner.label = "inner"
end

local function mark(t, key: string)
    t[key] = true
end

local holder = { box = { a = 1 } }
mark(holder.box, "seen")

M.reset({ mode = "on" })
M.nest()

local mode = M.state.mode
local inner = M.state.inner
local label: string? = inner and inner.label
local seen: boolean? = holder.box.seen

assert(mode == "on")
assert(label == "inner")
assert(seen == true)
