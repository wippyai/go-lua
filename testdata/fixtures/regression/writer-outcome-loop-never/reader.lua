-- Public estimation reader facade: the single cross-module read surface. Component-scoped
-- access enforcement binds here at the API layer later; for now it is the pure data facade
-- other modules import, validating the estimate_id and delegating to reader_internal, which
-- trusts nothing. Reads never touch the write path -- only the seq-fenced materialized tables.

local internal = require("reader_internal")

local M = {}

local function trim(v: any): string
    if type(v) ~= "string" then return "" end
    return (v:gsub("^%s*(.-)%s*$", "%1"))
end

local function require_estimate(estimate_id: string): string?
    if trim(estimate_id) == "" then return "estimate_id required" end
    return nil
end

-- tree_page: stable rank-DFS page over live nodes; cursor embeds head_seq and reports
-- structural drift as { restart_required = true }.
function M.tree_page(estimate_id: string, opts: any): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_tree_page(estimate_id, opts)
end

-- context_pack: the agent work order for one node (ancestry, annotations with acceptance never
-- truncated, comments, refs, resolved deps, metrics, attempt/claim state) under byte budgets.
function M.context_pack(estimate_id: string, node_id: string, budgets: any): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    if trim(node_id) == "" then return nil, "node_id required" end
    return internal.internal_context_pack(estimate_id, node_id, budgets)
end

-- frontier: ready nodes with all dependencies done, unclaimed, not recheck_required.
function M.frontier(estimate_id: string, opts: any): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_frontier(estimate_id, opts)
end

function M.rollups(estimate_id: string, node_id: string?): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_rollups(estimate_id, node_id)
end

function M.activity(estimate_id: string, opts: any): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_activity(estimate_id, opts)
end

function M.attempts(estimate_id: string, node_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    if trim(node_id) == "" then return nil, "node_id required" end
    return internal.internal_attempts(estimate_id, node_id)
end

-- schedule: the currently promoted run's per-node es/ef/ls/lf/slack/critical + lane; empty
-- until a run is published. Staged-but-unpublished runs are never returned.
function M.schedule(estimate_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_schedule(estimate_id, nil)
end

-- diff_baselines: structure (added/removed/moved/retitled/status-changed) + per-vertical
-- metric deltas between two baselines, computed over the hot diff tables.
function M.diff_baselines(estimate_id: string, base_a: string, base_b: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    if trim(base_a) == "" or trim(base_b) == "" then return nil, "two baseline ids are required" end
    return internal.internal_diff_baselines(estimate_id, base_a, base_b)
end

-- edges: every live edge of the estimate as flat { from_node, to_node, edge_type } rows
-- (depends_on/relates_to/duplicates), for the dependency map.
function M.edges(estimate_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.edges(estimate_id, nil)
end

-- history_flags: per-actor { can_undo, can_redo } for the undo/redo surface controls.
function M.history_flags(estimate_id: string, actor_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.history_flags(estimate_id, actor_id)
end

-- proposals: _proposal truth rows, optionally filtered by author_id / status.
function M.proposals(estimate_id: string, filter: any): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.internal_proposals(estimate_id, filter)
end

-- attention_split: counts of live nodes per attention value plus unset, for the dashboard.
function M.attention_split(estimate_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.attention_split(estimate_id)
end

-- baselines: discoverable baseline/checkpoint list (tombstoned excluded), oldest first.
function M.baselines(estimate_id: string): (any?, string?)
    local err = require_estimate(estimate_id)
    if err then return nil, err end
    return internal.baselines(estimate_id)
end

-- node_counts: live node count per estimate id, for the collection listing's node_count.
function M.node_counts(estimate_ids: { string }): (any, string?)
    return internal.node_counts(estimate_ids)
end

return M

