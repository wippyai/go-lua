-- Encapsulates the central service's mutable state so handlers don't reach
-- into file-locals. One instance lives per service process; methods mutate
-- that single instance.
--
--   state.snapshot       : current snapshot (snapshot_lib shape)
--   state.stale          : true when the snapshot is older than the journal
--   state.rebuilding_now : guards concurrent AI rebuilds

local snapshot_lib = require("snapshot")
local run_repo = require("run_repo")
local consts = require("git_consts")

local State = {}
State.__index = State

function State.new()
    return setmetatable({
        snapshot       = snapshot_lib.empty(),
        stale          = false,
        rebuilding_now = false,
    }, State)
end

-- Replace snapshot with a freshly built one (rebuild path).
function State:replace_snapshot(snap)
    self.snapshot = snap
    self.stale = false
    self.rebuilding_now = false
end

-- Persist the in-memory snapshot to keeper_git_runs (best effort; logged on err).
function State:persist(log)
    if not self.snapshot or not self.snapshot.run_id then return end
    local _, err = run_repo.update(self.snapshot.run_id, { payload = self.snapshot })
    if err and log then log:debug("snapshot persist failed", { error = err }) end
end

function State:set_decision(cluster_id, decision)
    return snapshot_lib.set_decision(self.snapshot, cluster_id, decision)
end

function State:update_recommendation(cluster_id, rec_id, rec_state)
    return snapshot_lib.update_recommendation(self.snapshot, cluster_id, rec_id, rec_state)
end

function State:get_cluster(cluster_id)
    return snapshot_lib.get_cluster(self.snapshot, cluster_id)
end

function State:reorder()
    snapshot_lib.reorder(self.snapshot)
end

function State:summary()
    local s = snapshot_lib.to_summary(self.snapshot)
    s.stale = self.stale
    if self.rebuilding_now then s.in_progress = true end
    return s
end

-- Recompute stale flag from journal growth. Returns true if the flag flipped.
function State:recompute_stale()
    if snapshot_lib.is_empty(self.snapshot) then
        self.stale = false
        return false
    end
    local size, err = run_repo.current_journal_size()
    if err then return false end
    local diff = size - (self.snapshot.journal_size_at_build or 0)
    if diff >= consts.STALE_AFTER_CHANGES and not self.stale then
        self.stale = true
        return true
    end
    return false
end

-- Drop pushed clusters from the snapshot and mark stale (the journal moved).
function State:mark_pushed(cluster_id)
    snapshot_lib.set_decision(self.snapshot, cluster_id, consts.DECISIONS.PUSHED)
    self.stale = true
end

-- Restore from latest persisted run on boot. Returns true if anything loaded.
function State:restore_from_db(log)
    local row, err = run_repo.latest_finished()
    if err then
        if log then log:warn("restore failed", { error = tostring(err) }) end
        return false
    end
    if not row or not row.payload then return false end
    self.snapshot = row.payload
    self.stale = true
    return true
end

return State
