-- Rollup consolidation. The fold never computes rollups; it only enqueues dirty
-- ancestor chains. After the processor's projection catch-up reaches the contiguous
-- thread head, it calls consolidate: drain _dirty, recompute the affected ancestor
-- chains bottom-up ABSOLUTE from the committed, fenced _node + _metric state into
-- _rollup_metric (own/sub point+lo+hi) and _rollup_node (leaf/done/open counts,
-- metric-independent), stamping computed_through_seq. A dirty set wider than the
-- threshold falls back to a full-estimate recompute. Recompute is absolute, so a
-- replayed stale fold (fenced out of the base tables) never regresses a rollup, and
-- computed_through_seq only advances.

local sql = require("sql")
local types = require("types")

local M = {}

local T = types.T
local OPEN_STATUSES: any = types.OPEN_STATUSES

local function open_db(): (any?, string?)
    local db, err = sql.get(types.db_id())
    if not db then return nil, tostring(err or "estimation: db unavailable") end
    return db, nil
end

local function query(db: any, statement: string, params: any): (any, string?)
    local rows, err = db:query(statement, params or {})
    if err then return nil, tostring(err) end
    return rows or {}, nil
end

local function exec(db: any, statement: string, params: any): string?
    local _, err = db:execute(statement, params or {})
    if err then return tostring(err) end
    return nil
end

local function nz(v: any): number
    return tonumber(v) or 0
end

-- add_lane accumulates a source metric lane into a running sum, preserving nil until a
-- real value contributes (so a lane with no data stays null rather than 0).
local function add_lane(acc: any, key: string, value: any)
    if value == nil then return end
    acc[key] = (acc[key] or 0) + (tonumber(value) or 0)
end

function M.consolidate(estimate_id: string, computed_through_seq: number): string?
    local db, db_err = open_db()
    if not db then return db_err end

    local dirty, derr = query(db, "SELECT node_id FROM " .. T.DIRTY .. " WHERE estimate_id = $1", { estimate_id })
    if derr then db:release(); return derr end
    if #(dirty :: { any }) == 0 then db:release(); return nil end

    local nodes, nerr = query(db, "SELECT node_id, parent_id, depth, status FROM " .. T.NODE .. " WHERE estimate_id = $1 AND deleted_seq = 0", { estimate_id })
    if nerr then db:release(); return nerr end
    local metrics, merr = query(db, "SELECT node_id, vertical, point, lo, hi FROM " .. T.METRIC .. " WHERE estimate_id = $1 AND deleted_seq = 0", { estimate_id })
    if merr then db:release(); return merr end
    -- Active claims + depends_on edges drive recheck_required derivation from live state.
    local claim_rows, cl_err = query(db, "SELECT node_id, attempt_no FROM " .. T.CLAIM .. " WHERE estimate_id = $1", { estimate_id })
    if cl_err then db:release(); return cl_err end
    local dep_rows, dep_err = query(db, "SELECT from_node, to_node FROM " .. T.EDGE .. " WHERE estimate_id = $1 AND edge_type = 'depends_on' AND deleted_seq = 0", { estimate_id })
    if dep_err then db:release(); return dep_err end

    -- Index live structure.
    local live: { [string]: any } = {}
    local children: { [string]: { string } } = {}
    local ordered: { any } = {}
    for _, r in ipairs(nodes :: { any }) do
        local id = tostring((r :: any).node_id)
        live[id] = { parent_id = tostring((r :: any).parent_id or ""), depth = nz((r :: any).depth), status = tostring((r :: any).status or "planned") }
        ordered[#ordered + 1] = id
    end
    for id, info in pairs(live) do
        local pid = (info :: any).parent_id
        if pid ~= "" and live[pid] then
            children[pid] = children[pid] or {}
            children[pid][#children[pid] + 1] = id
        end
    end

    -- own metric lanes per node.
    local own: { [string]: { [string]: any } } = {}
    for _, r in ipairs(metrics :: { any }) do
        local id = tostring((r :: any).node_id)
        if live[id] then
            own[id] = own[id] or {}
            own[id][tostring((r :: any).vertical)] = { point = (r :: any).point, lo = (r :: any).lo, hi = (r :: any).hi }
        end
    end

    -- Live claim + dependency state for recheck derivation.
    local claimed: { [string]: boolean } = {}
    for _, r in ipairs(claim_rows :: { any }) do
        if nz((r :: any).attempt_no) > 0 then claimed[tostring((r :: any).node_id)] = true end
    end
    local deps: { [string]: { string } } = {}
    for _, r in ipairs(dep_rows :: { any }) do
        local f = tostring((r :: any).from_node)
        deps[f] = deps[f] or {}
        deps[f][#deps[f] + 1] = tostring((r :: any).to_node)
    end
    -- recheck_required is derived only for actively-claimed nodes: a claimed node with any
    -- non-done dependency is frozen, one whose dependencies are all done is cleared. Nodes
    -- with no active claim keep their recheck flag untouched (the frontier's sticky freeze).
    local function derive_recheck(node_id: string): boolean
        for _, tid in ipairs(deps[node_id] or {}) do
            local t = live[tid]
            if not t or (t :: any).status ~= types.STATUS.DONE then return true end
        end
        return false
    end

    -- Bottom-up: process deepest nodes first so a parent reads settled child totals.
    table.sort(ordered, function(a: string, b: string): boolean
        return nz(live[a].depth) > nz(live[b].depth)
    end)

    local agg: { [string]: { [string]: any } } = {} -- node -> vertical -> {own_point/lo/hi, sub_point/lo/hi}
    local counts: { [string]: any } = {}            -- node -> {leaf, done, open}

    for _, id in ipairs(ordered) do
        local lane: { [string]: any } = {}
        for v, m in pairs(own[id] or {}) do
            lane[v] = { own_point = (m :: any).point, own_lo = (m :: any).lo, own_hi = (m :: any).hi }
        end
        local kids = children[id] or {}
        if #kids == 0 then
            counts[id] = {
                leaf = 1,
                done = live[id].status == types.STATUS.DONE and 1 or 0,
                open = OPEN_STATUSES[tostring(live[id].status)] and 1 or 0,
            }
        else
            local c = { leaf = 0, done = 0, open = 0 }
            for _, cid in ipairs(kids) do
                local cc = counts[cid] or { leaf = 0, done = 0, open = 0 }
                c.leaf = c.leaf + cc.leaf; c.done = c.done + cc.done; c.open = c.open + cc.open
                for v, cl in pairs(agg[cid] or {}) do
                    local entry = lane[v] or {}
                    local sub = { point = entry.sub_point, lo = entry.sub_lo, hi = entry.sub_hi }
                    add_lane(sub, "point", (cl :: any).own_point); add_lane(sub, "point", (cl :: any).sub_point)
                    add_lane(sub, "lo", (cl :: any).own_lo); add_lane(sub, "lo", (cl :: any).sub_lo)
                    add_lane(sub, "hi", (cl :: any).own_hi); add_lane(sub, "hi", (cl :: any).sub_hi)
                    entry.sub_point = sub.point; entry.sub_lo = sub.lo; entry.sub_hi = sub.hi
                    lane[v] = entry
                end
            end
            counts[id] = c
        end
        agg[id] = lane
    end

    -- Target set: dirty nodes plus every ancestor of a still-live dirty node. Removed
    -- dirty nodes stay in the set so their stale rollup rows are deleted.
    local target: { [string]: boolean } = {}
    for _, r in ipairs(dirty :: { any }) do
        local id = tostring((r :: any).node_id)
        target[id] = true
        local cursor = live[id] and live[id].parent_id or ""
        local guard = 0
        while cursor ~= "" and live[cursor] and guard <= types.MAX_DEPTH + 2 do
            target[cursor] = true
            cursor = live[cursor].parent_id
            guard = guard + 1
        end
    end
    local target_count = 0
    for _ in pairs(target) do target_count = target_count + 1 end
    if target_count > types.DIRTY_FULL_THRESHOLD then
        for id in pairs(live) do target[id] = true end
    end

    local tx, tx_err = db:begin()
    if tx_err then db:release(); return "failed to begin consolidation transaction: " .. tostring(tx_err) end

    for id in pairs(target) do
        local e1 = exec(tx, "DELETE FROM " .. T.ROLLUP_METRIC .. " WHERE estimate_id=$1 AND node_id=$2", { estimate_id, id })
        if e1 then tx:rollback(); db:release(); return e1 end
        local e2 = exec(tx, "DELETE FROM " .. T.ROLLUP_NODE .. " WHERE estimate_id=$1 AND node_id=$2", { estimate_id, id })
        if e2 then tx:rollback(); db:release(); return e2 end
    end

    for id in pairs(target) do
        if live[id] then
            local c = counts[id] or { leaf = 0, done = 0, open = 0 }
            local ne = exec(tx,
                "INSERT INTO " .. T.ROLLUP_NODE .. " (estimate_id, node_id, leaf_count, done_count, open_count, computed_through_seq)" ..
                " VALUES ($1,$2,$3,$4,$5,$6)",
                { estimate_id, id, c.leaf, c.done, c.open, computed_through_seq })
            if ne then tx:rollback(); db:release(); return ne end
            for v, l in pairs(agg[id] or {}) do
                local me = exec(tx,
                    "INSERT INTO " .. T.ROLLUP_METRIC ..
                    " (estimate_id, node_id, vertical, own_point, own_lo, own_hi, sub_point, sub_lo, sub_hi, computed_through_seq)" ..
                    " VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
                    { estimate_id, id, v, (l :: any).own_point, (l :: any).own_lo, (l :: any).own_hi,
                      (l :: any).sub_point, (l :: any).sub_lo, (l :: any).sub_hi, computed_through_seq })
                if me then tx:rollback(); db:release(); return me end
            end
        end
    end

    -- Derive recheck_required for any actively-claimed target node from live dependency state.
    for id in pairs(target) do
        if live[id] and claimed[id] then
            local flag = derive_recheck(id) and 1 or 0
            local re = exec(tx, "UPDATE " .. T.NODE .. " SET recheck_required=$1 WHERE estimate_id=$2 AND node_id=$3 AND deleted_seq=0",
                { flag, estimate_id, id })
            if re then tx:rollback(); db:release(); return re end
        end
    end

    local delerr = exec(tx, "DELETE FROM " .. T.DIRTY .. " WHERE estimate_id = $1", { estimate_id })
    if delerr then tx:rollback(); db:release(); return delerr end

    local _, commit_err = tx:commit()
    db:release()
    if commit_err then return "failed to commit consolidation: " .. tostring(commit_err) end
    return nil
end

return M

