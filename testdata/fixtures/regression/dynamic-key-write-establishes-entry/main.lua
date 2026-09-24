-- A write t[k] = v with v non-nil makes k a key of t on every path leaving the
-- write, until t or k changes; a join keeps it only when every incoming path
-- establishes it (keeper changeset observer: per-namespace counters).
local function after_write(k: string): integer
    local ns = {}
    ns[k] = { x = 1 }
    return ns[k].x + 1
end

local function both_branches(k: string, cond: boolean): integer
    local ns = {}
    if cond then
        ns[k] = { x = 1 }
    else
        ns[k] = { x = 2 }
    end
    return ns[k].x + 1
end

local function guarded_default(ks: {string})
    local ns = {}
    for _, k in ipairs(ks) do
        if not ns[k] then
            ns[k] = { x = 0 }
        end
        ns[k].x = ns[k].x + 1
    end
    return ns
end

local function read_before_write(k: string)
    local ns = {}
    ns[k] = { x = ns[k].x + 1 } -- expect-error
    return ns
end

local function one_branch(k: string, cond: boolean): integer
    local ns = {}
    if cond then
        ns[k] = { x = 1 }
    end
    return ns[k].x + 1 -- expect-error
end

local function key_reassigned(k: string, other: string): integer
    local ns = {}
    ns[k] = { x = 1 }
    k = other
    return ns[k].x + 1 -- expect-error
end

local function key_assigned_by_the_write(k: string, other: string): integer
    local ns = {}
    k, ns[k] = other, { x = 1 }
    return ns[k].x + 1 -- expect-error
end

local function optional_value(k: string, v: { x: integer }?): integer
    local ns = {}
    ns[k] = v
    return ns[k].x + 1 -- expect-error
end

local function other_key(k: string, other: string): integer
    local ns = {}
    ns[k] = { x = 1 }
    return ns[other].x + 1 -- expect-error
end

return {
    after_write = after_write,
    both_branches = both_branches,
    guarded_default = guarded_default,
    read_before_write = read_before_write,
    one_branch = one_branch,
    key_reassigned = key_reassigned,
    key_assigned_by_the_write = key_assigned_by_the_write,
    optional_value = optional_value,
    other_key = other_key,
}
