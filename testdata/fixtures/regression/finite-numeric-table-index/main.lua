local registry = require("registry")

local M = {}

M.STATUS = {
    RUNNING = "running",
    COMPLETE = "complete",
    FAILED = "failed",
}

-- Thread event types appended to an experiment's immutable log (prefix law:
-- <ns>.events:<name>). query opens an exchange; run_r/progress are its work
-- steps; result/failed close it. The projection folds these into the read model.
M.EVENT = {
    QUERY = "spiralscout.stat_analysis.events:query",
    PROGRESS = "spiralscout.stat_analysis.events:progress",
    -- One RunR tool call: agent, R code, stdout/result excerpt, error, and any
    -- rendered plot artifacts. Drives the execution timeline live and in history.
    RUN_R = "spiralscout.stat_analysis.events:run_r",
    RESULT = "spiralscout.stat_analysis.events:result",
    FAILED = "spiralscout.stat_analysis.events:failed",
}

-- The single stdout excerpt bound, shared by the agent's tool result and the
-- persisted timeline. Sized to hold a STRUCTURE-class print whole — a str() /
-- summary() / head() of a wide frame (comfortably ~50 variables, a few KB) never
-- truncates. Only a genuinely long dump exceeds it, and when it does the FULL text
-- is persisted as a text artifact: the timeline offers "Show full output" and the
-- agent's result carries truncated=true + the artifact ref (bounded, never blind).
M.STEP = {
    CODE_BYTES = 4000,
    STDOUT_BYTES = 12000,
    RESULT_BYTES = 800,
    -- Upper bound on the full-output text artifact so a runaway print never stores
    -- megabytes; the agent still reads it back (bounded) through ReadArtifact.
    OUTPUT_MAX_BYTES = 200000,
}

-- One artifact model. An R plot is an image artifact backed by an uploaded PNG; an
-- overflow text artifact holds a long R stdout dump; a chart spec is the degraded
-- fallback recorded only when RunR is unavailable.
M.ARTIFACT = {
    IMAGE = "image",
    TEXT = "text",
    CHART = "chart",
}

M.MIME = {
    PNG = "image/png",
    TEXT = "text/plain",
}

-- A Stat Analysis space is a first-class workspace component. Session files and
-- experiments are scoped to its component_id; the "+" create flow registers a row
-- of this impl under the chosen parent, and deletion cascades the read model.
M.KIND_IMPL = "spiralscout.stat_analysis:stat_analysis_kind"
M.CLASS = "stat_analysis"
M.COMPONENT_SERVICE = "kickside.component:component_service"

-- The shared space the Analyze data Block keeps its results in when no space is
-- configured, created once per owner on first use.
M.BLOCK_SPACE_TITLE = "Automated analyses"

-- An experiment is a child component under the space (parent_id linkage) carrying
-- ONE canonical kickside.core thread (thread_id == experiment component_id). Every
-- query is an exchange appended to that thread; the binding declares the projection
-- that folds the thread into the read model, and drain-aware retention so events
-- are never pruned ahead of the projection cursor.
M.EXPERIMENT_IMPL = "spiralscout.stat_analysis:experiment_kind"
M.EXPERIMENT_CLASS = "stat_analysis_experiment"
M.THREAD_CLASS = "spiralscout.stat_analysis"

-- kickside.core seams the module reads and writes threads through.
M.THREADS_UPSERT = "kickside.core.threads:upsert_method"
M.THREADS_APPEND_EVENT = "kickside.core.threads:append_event_method"
M.THREADS_LIST_EVENTS = "kickside.core.threads:list_events_method"
M.THREADS_REGISTER_PROJECTION = "kickside.core.threads:register_projection_method"

-- Correlation status for an Analyze data Block parked on its terminal event.
M.BLOCK_SIGNAL_STATUS = {
    WAITING = "waiting",
    RESUMED = "resumed",
    FAILED = "failed",
}

-- Verified live against wippy-langs/r >= 0.2.12 (readers_live_test): base R reads
-- .csv/.tsv/.rds/.rda, readxl reads Excel workbooks, and the bundled pure-R haven
-- reader reads supported SPSS .sav files. The remaining formats stage fine but R
-- cannot open them; the surface shows a note without rejecting the upload.
local UNREADABLE_EXT: { [string]: string } = {
    zsav = "Compressed .zsav files are not supported — export as an uncompressed or classic-compressed .sav.",
    dta = "R cannot read .dta in this build — convert to .csv or .rda.",
    por = "R cannot read .por in this build — convert to .csv.",
    json = "R has no JSON reader in this build (jsonlite is absent) — convert to .csv.",
}

-- A gentle, non-blocking note for a staged file R cannot open, or "" when readable.
function M.stage_note(filename: any): string
    local name = type(filename) == "string" and filename or ""
    local ext = name:match("%.([%w]+)$")
    if type(ext) ~= "string" then return "" end
    return UNREADABLE_EXT[ext:lower()] or ""
end

-- Depth -> iteration budget. Depth is the one user control besides the prompt:
-- how many analyze/render/inspect R calls one exchange may spend. The arena loop
-- bound scales with it, and run_r enforces r_calls as a hard per-exchange cap.
M.DEPTH = {
    MIN = 1,
    MAX = 5,
    DEFAULT = 3,
}

local DEPTH_BUDGET: { [number]: any } = {
    [1] = { r_calls = 3, arena_iterations = 12 },
    [2] = { r_calls = 6, arena_iterations = 18 },
    [3] = { r_calls = 10, arena_iterations = 26 },
    [4] = { r_calls = 16, arena_iterations = 38 },
    [5] = { r_calls = 24, arena_iterations = 54 },
}

function M.clamp_depth(depth: any): number
    local d = math.floor(tonumber(depth) or M.DEPTH.DEFAULT)
    if d < M.DEPTH.MIN then d = M.DEPTH.MIN end
    if d > M.DEPTH.MAX then d = M.DEPTH.MAX end
    return d
end

function M.depth_budget(depth: any): any
    return DEPTH_BUDGET[M.clamp_depth(depth)]
end

-- RunR execution bounds. The wippy-langs/r reactor runs synchronously on a warm
-- inline pool and exposes no per-call timeout, so the tool bounds work by code
-- size and clamps plot geometry; long computations are the engine's concern.
-- Plot defaults render a readable, retina-crisp canvas (a 1200x800 PNG at 144 dpi
-- stays well under the ~1MB inline-image bound); geometry is clamped to MIN/MAX.
M.R = {
    MAX_CODE_BYTES = 100000,
    DATA_FS = "r:data",
    DATA_MOUNT = "/data",
    DEFAULT_WIDTH = 1200,
    DEFAULT_HEIGHT = 800,
    MIN_DIM = 160,
    MAX_DIM = 2400,
    DEFAULT_DPI = 144,
    MIN_DPI = 48,
    MAX_DPI = 300,
}

-- Registry entry whose meta.db_id holds the configured database resource. The
-- target_db ns.requirement rewrites that field per deployment.
M.DB_CONFIG = "spiralscout.stat_analysis:db_config"

function M.db_id(): string
    local entry: any = registry.get(M.DB_CONFIG)
    return entry.meta.db_id :: string
end

-- Servable, owner-scoped URL for an uploaded artifact's raw bytes.
function M.raw_url(upload_id: string): string
    if type(upload_id) ~= "string" or upload_id == "" then return "" end
    return "/api/v1/uploads/" .. upload_id .. "/raw"
end

-- Deterministic R workspace continuity. Every RunR call inside an experiment
-- restores this image first and saves it after, so a follow-up exchange starts
-- exactly where the previous one left the R session — independent of warm-pool
-- instance recycling. The file lives on the shared R /data mount.
function M.state_path(experiment_id: string): string
    if type(experiment_id) ~= "string" or experiment_id == "" then return "" end
    return M.R.DATA_MOUNT .. "/.state/" .. experiment_id .. ".RData"
end

return M

