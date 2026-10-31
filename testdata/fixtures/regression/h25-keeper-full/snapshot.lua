local consts = require("git_consts")
local push_policy = require("push_policy")

-- Snapshot is the in-memory shape held by the service.
-- {
--   run_id, built_at, journal_size_at_build, ai_model,
--   clusters = { [cluster_id] = cluster, ... },
--   cluster_order = { cluster_id, ... },     -- sorted by importance/decision
--   orphans = { change_id, ... },             -- suspect change ids
-- }
--
-- Cluster shape:
-- {
--   id, title, plain_summary, importance, verdict, verdict_text,
--   decision, change_ids = {...},
--   recommendations = { { id, severity, text, fix_hint, change_id, state }, ... },
--   stats = { added, removed, namespaces = {...}, files = N },
--   pushable, push_blockers = {...}
-- }

local M = {}

function M.empty()
    return {
        run_id = nil,
        built_at = nil,
        journal_size_at_build = 0,
        ai_model = nil,
        clusters = {},
        cluster_order = {},
        orphans = {},
    }
end

function M.is_empty(snap)
    return not snap or not snap.run_id
end

-- Resort cluster_order applying decision/importance ranks.
function M.reorder(snap)
    if not snap or not snap.clusters then return end
    local rank_dec = {
        [consts.DECISIONS.PENDING]  = 0,
        [consts.DECISIONS.APPROVED] = 1,
        [consts.DECISIONS.SKIPPED]  = 2,
        [consts.DECISIONS.PUSHED]   = 3,
    }
    local rank_imp = {
        [consts.IMPORTANCE.CRITICAL] = 0,
        [consts.IMPORTANCE.HIGH]     = 1,
        [consts.IMPORTANCE.NORMAL]   = 2,
        [consts.IMPORTANCE.CLEANUP]  = 3,
        [consts.IMPORTANCE.SUSPECT]  = 4,
    }
    local order = {}
    for id, _ in pairs(snap.clusters) do table.insert(order, id) end
    table.sort(order, function(a, b)
        local ca, cb = snap.clusters[a], snap.clusters[b]
        local da, db = rank_dec[ca.decision] or 9, rank_dec[cb.decision] or 9
        if da ~= db then return da < db end
        local ia, ib = rank_imp[ca.importance] or 9, rank_imp[cb.importance] or 9
        if ia ~= ib then return ia < ib end
        return (ca.title or "") < (cb.title or "")
    end)
    snap.cluster_order = order
end

function M.get_cluster(snap, id)
    if not snap or not snap.clusters then return nil end
    return snap.clusters[id]
end

function M.set_decision(snap, cluster_id, decision)
    if not consts.is_decision(decision) then
        return false, "invalid decision: " .. tostring(decision)
    end
    local c = M.get_cluster(snap, cluster_id)
    if not c then return false, consts.ERRORS.UNKNOWN_CLUSTER end
    c.decision = decision
    M.reorder(snap)
    return true, nil
end

function M.update_recommendation(snap, cluster_id, rec_id, state)
    if not consts.is_rec_state(state) then
        return false, "invalid recommendation state: " .. tostring(state)
    end
    local c = M.get_cluster(snap, cluster_id)
    if not c then return false, consts.ERRORS.UNKNOWN_CLUSTER end
    for _, r in ipairs(c.recommendations or {}) do
        if r.id == rec_id then
            r.state = state
            return true, nil
        end
    end
    return false, "unknown recommendation_id"
end

function M.cluster_pushable(c)
    if not c then return false end
    return #push_policy.blockers(c) == 0
end

-- Public DTO shape for API/list responses (omit verbose fields).
function M.to_summary(snap)
    if M.is_empty(snap) then
        return {
            run_id = nil,
            built_at = nil,
            ai_model = nil,
            clusters = {},
            counts = { all = 0, pending = 0, ready = 0, hidden = 0, suspect = 0,
                       pushable_ready = 0, blocked_ready = 0 },
        }
    end
    local counts = { all = 0, pending = 0, ready = 0, hidden = 0, suspect = 0,
                     pushable_ready = 0, blocked_ready = 0 }
    local list = {}
    for _, id in ipairs(snap.cluster_order or {}) do
            local c = snap.clusters[id]
            if c then
                local pushable = M.cluster_pushable(c)
                local push_blockers = push_policy.blockers(c)
            counts.all = counts.all + 1
            if c.decision == consts.DECISIONS.PENDING then counts.pending = counts.pending + 1
            elseif c.decision == consts.DECISIONS.APPROVED then
                counts.ready = counts.ready + 1
                if pushable then counts.pushable_ready = counts.pushable_ready + 1
                else counts.blocked_ready = counts.blocked_ready + 1 end
            elseif c.decision == consts.DECISIONS.SKIPPED then counts.hidden = counts.hidden + 1
            end
            if c.is_suspect or c.importance == consts.IMPORTANCE.SUSPECT then
                counts.suspect = counts.suspect + 1
            end
            table.insert(list, {
                id = c.id,
                title = c.title,
                plain_summary = c.plain_summary,
                importance = c.importance,
                verdict = c.verdict,
                verdict_text = c.verdict_text,
                decision = c.decision,
                change_count = #(c.change_ids or {}),
                stats = c.stats,
                source = c.source,
                pushable = pushable,
                push_blockers = push_blockers,
                rec_open = (function()
                    local n = 0
                    for _, r in ipairs(c.recommendations or {}) do
                        if r.state == consts.REC_STATES.OPEN then n = n + 1 end
                    end
                    return n
                end)(),
            })
        end
    end
    return {
        run_id = snap.run_id,
        built_at = snap.built_at,
        journal_size_at_build = snap.journal_size_at_build,
        ai_model = snap.ai_model,
        clusters = list,
        counts = counts,
    }
end

return M
