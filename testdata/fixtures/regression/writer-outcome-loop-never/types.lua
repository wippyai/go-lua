-- Shared constants for the estimation module: table names, event types, enums,
-- and the requirement-injected DB resource lookup.

local M = {}

M.DB_CONFIG = "spiralscout.estimation:db_config"

-- Injectable registry seam so tests can stub resolution.
M._registry = nil

function M.db_id(): string
    local registry = M._registry or require("registry")
    local entry, err = registry.get(M.DB_CONFIG)
    if err or not entry then
        error("estimation db_config entry is missing: " .. tostring(err))
    end
    local meta = (entry :: any).meta or ((entry :: any).data and (entry :: any).data.meta) or {}
    local id = tostring((meta :: any).db_id or "")
    if id == "" then
        error("estimation db_config has no injected db_id")
    end
    return id
end

M.T = {
    NODE = "spiralscout_estimation_node",
    METRIC = "spiralscout_estimation_metric",
    ANNOTATION = "spiralscout_estimation_annotation",
    COMMENT = "spiralscout_estimation_comment",
    COMMENT_REV = "spiralscout_estimation_comment_rev",
    REF = "spiralscout_estimation_ref",
    EDGE = "spiralscout_estimation_edge",
    ROLLUP_METRIC = "spiralscout_estimation_rollup_metric",
    ROLLUP_NODE = "spiralscout_estimation_rollup_node",
    DIRTY = "spiralscout_estimation_dirty",
    ASSIGNMENT = "spiralscout_estimation_assignment",
    CAPACITY = "spiralscout_estimation_capacity",
    GRANT = "spiralscout_estimation_grant",
    PROPOSAL = "spiralscout_estimation_proposal",
    ATTEMPT = "spiralscout_estimation_attempt",
    CLAIM = "spiralscout_estimation_claim",
    ACTIVITY = "spiralscout_estimation_activity",
    ADMISSION = "spiralscout_estimation_admission",
    OUTCOME = "spiralscout_estimation_outcome",
    SCHEDULE_STAGING = "spiralscout_estimation_schedule_staging",
    SCHEDULE_RUN = "spiralscout_estimation_schedule_run",
    SCHEDULE_CURRENT = "spiralscout_estimation_schedule_current",
    BASELINE = "spiralscout_estimation_baseline",
    BASELINE_NODE = "spiralscout_estimation_baseline_node",
    BASELINE_METRIC = "spiralscout_estimation_baseline_metric",
    BASELINE_EDGE = "spiralscout_estimation_baseline_edge",
    BLOB = "spiralscout_estimation_blob",
    VOCAB = "spiralscout_estimation_vocab",
    HISTORY = "spiralscout_estimation_history",
}

M.EVENTS_NS = "spiralscout.estimation.events"

M.EVENTS = {
    NODE_CREATED = M.EVENTS_NS .. ":node.created",
    NODE_UPDATED = M.EVENTS_NS .. ":node.updated",
    NODE_MOVED = M.EVENTS_NS .. ":node.moved",
    NODE_REORDERED = M.EVENTS_NS .. ":node.reordered",
    NODE_REMOVED = M.EVENTS_NS .. ":node.removed",
    METRIC_SET = M.EVENTS_NS .. ":metric.set",
    METRIC_CLEARED = M.EVENTS_NS .. ":metric.cleared",
    ANNOTATION_SET = M.EVENTS_NS .. ":annotation.set",
    ANNOTATION_CLEARED = M.EVENTS_NS .. ":annotation.cleared",
    COMMENT_ADDED = M.EVENTS_NS .. ":comment.added",
    COMMENT_EDITED = M.EVENTS_NS .. ":comment.edited",
    REF_ATTACHED = M.EVENTS_NS .. ":ref.attached",
    REF_UPDATED = M.EVENTS_NS .. ":ref.updated",
    REF_REMOVED = M.EVENTS_NS .. ":ref.removed",
    EDGE_LINKED = M.EVENTS_NS .. ":edge.linked",
    EDGE_UNLINKED = M.EVENTS_NS .. ":edge.unlinked",
    ASSIGNMENT_SET = M.EVENTS_NS .. ":assignment.set",
    ASSIGNMENT_REMOVED = M.EVENTS_NS .. ":assignment.removed",
    CAPACITY_SET = M.EVENTS_NS .. ":capacity.set",
    CAPACITY_REMOVED = M.EVENTS_NS .. ":capacity.removed",
    STATUS_CHANGED = M.EVENTS_NS .. ":status.changed",
    CLAIM_ACQUIRED = M.EVENTS_NS .. ":claim.acquired",
    CLAIM_RENEWED = M.EVENTS_NS .. ":claim.renewed",
    CLAIM_RELEASED = M.EVENTS_NS .. ":claim.released",
    ATTEMPT_CANCELLED = M.EVENTS_NS .. ":attempt.cancelled",
    PROPOSAL_SUBMITTED = M.EVENTS_NS .. ":proposal.submitted",
    PROPOSAL_DECIDED = M.EVENTS_NS .. ":proposal.decided",
    GRANT_CREATED = M.EVENTS_NS .. ":grant.created",
    GRANT_REVOKED = M.EVENTS_NS .. ":grant.revoked",
    GRANT_EXPIRED = M.EVENTS_NS .. ":grant.expired",
    BASELINE_CREATED = M.EVENTS_NS .. ":baseline.created",
    BASELINE_REMOVED = M.EVENTS_NS .. ":baseline.removed",
    VOCAB_REVISED = M.EVENTS_NS .. ":vocab.revised",
    SCHEDULE_PUBLISHED = M.EVENTS_NS .. ":schedule.published",
    COMMAND_COMPLETED = M.EVENTS_NS .. ":command.completed",
}

M.STATUS = { PLANNED = "planned", READY = "ready", IN_PROGRESS = "in_progress", DONE = "done", DROPPED = "dropped" }
M.ORIGIN = { HUMAN = "human", AGENT = "agent", IMPORT = "import", PROPOSAL = "proposal" }
M.GRANT_ROLE = { READ = "read", PROPOSE = "propose" }
M.ATTEMPT_STATE = { ACTIVE = "active", FAILED = "failed", COMPLETED = "completed", CANCELLED = "cancelled" }
M.PROPOSAL_STATUS = { PENDING = "pending", ACCEPTED = "accepted", REJECTED = "rejected", EXPIRED = "expired" }

M.MAX_DEPTH = 64
M.MAX_COMMAND_OPS = 500
M.MAX_COMMAND_BYTES = 262144
M.MAX_PROPOSAL_OPS = 200
M.MAX_PROPOSAL_BYTES = 524288

-- Per-estimate in-flight admission cap; over cap yields a `busy` refusal.
M.MAX_INFLIGHT = 64
-- Dirty-set size beyond which the consolidator recomputes the whole estimate
-- instead of the affected ancestor chains.
M.DIRTY_FULL_THRESHOLD = 256

-- Per-actor undo/redo history depth; older entries are trimmed on each record.
M.MAX_HISTORY_DEPTH = 50

M.THREAD_CLASS = "workspace.estimate"
M.CLASS = "estimate"

-- Core seams and this kind's declared projection.
M.THREADS = "kickside.core:threads"
M.IMPL_ID = "spiralscout.estimation:estimate_kind"
M.PROJECTION_KIND = "spiralscout.estimation:state"

-- Command-processor process entries + registry naming.
M.PROCESSOR = "spiralscout.estimation:processor"
M.SUPERVISOR = "spiralscout.estimation:supervisor"
M.REGISTRY_PREFIX = "estimation:"
M.DISPATCHER_NAME = "estimation:dispatcher"
M.PROCESS_HOST_CONFIG = "spiralscout.estimation:process_host_config"

-- Statuses the rollup treats as open (leaf work still outstanding).
M.OPEN_STATUSES = { planned = true, ready = true, in_progress = true }

-- Reserved _activity seq base for operator alarms (family='alarm'). Real thread event
-- seqs are event counts and never approach this, so an alarm row (written by repair, not
-- the fold) never collides with a folded activity row and sorts after the live feed. Kept
-- below 2^31 so it fits a 32-bit INTEGER seq column on Postgres.
M.ALARM_SEQ_BASE = 2000000000

-- Resolve the process host the command processor spawns onto. Requirement-injected
-- (default app:processes) so the module never hardcodes an app-owned resource.
function M.process_host(): string
    local registry = M._registry or require("registry")
    local entry = registry.get(M.PROCESS_HOST_CONFIG)
    if type(entry) == "table" then
        local meta = (entry :: any).meta or ((entry :: any).data and (entry :: any).data.meta) or {}
        local host = tostring((meta :: any).host_id or "")
        if host ~= "" then return host end
    end
    return "app:processes"
end

-- RFC3339 UTC TEXT timestamp (canonical timestamp handling: TEXT columns, UTC).
function M.now(): string
    return os.date("!%Y-%m-%dT%H:%M:%SZ") :: string
end

return M

