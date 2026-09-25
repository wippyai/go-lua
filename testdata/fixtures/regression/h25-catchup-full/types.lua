-- Projection-slice constant vocabulary. The entity shapes (Projection) and the
-- valid/invalid/pending PROJECTION_STATE the schema CHECK enforces are shared
-- across slices and live in kickside.core:types; this module holds only the
-- constants the projection engine owns: the cursor status set, key conventions,
-- worker runtime/input vocabularies, window modes, trigger engines, and the
-- numeric bounds that clamp register inputs.
--
-- Every value here is constant-invariant: it mirrors a schema CHECK, a URI
-- scheme, or a hard safety bound, so it is NOT operator-tunable. The handful of
-- knobs an operator may want to retune (batch-size default, window defaults,
-- trigger retry delay) live in the env accessor (env.lua), read through
-- env.variable entries, not here.

local M = {}

-- ─── Cursor status ─────────────────────────────────────────────────────
-- Mirrors kickside_projection_cursor.status CHECK. The scheduler dispatches on
-- pending; catch-up moves a cursor through running and parks it on caught_up
-- (bounded backfill done) or live (tailing). paused/failed/invalid are terminal
-- until an operator or a config change reactivates them.
M.CURSOR_STATUS = {
    PENDING = "pending",
    RUNNING = "running",
    LIVE = "live",
    CAUGHT_UP = "caught_up",
    FAILED = "failed",
    PAUSED = "paused",
    INVALID = "invalid",
}

-- last_error values with behavior attached. Keep these stable: operators and
-- recovery code use them to distinguish transient engine races from permanent
-- projection/config invalidations.
M.CURSOR_ERROR = {
    DISPATCH_ORPHAN = "dispatch: thread or projection no longer exists",
}

-- ─── Key conventions ───────────────────────────────────────────────────
-- A projection is addressed by (thread_id, kind, projection_key); a cursor by
-- (projection_id, cursor_key). KEY_DEFAULT is the implicit value for both when
-- a caller omits them. Mirrors the schema DEFAULT 'default'.
M.KEY_DEFAULT = "default"
-- Length caps guarding kind / *_key columns. Invariant: a defensive bound, not
-- a tuning dial.
M.KIND_MAX_LEN = 120
M.KEY_MAX_LEN = 120

-- ─── Worker runtime + input ────────────────────────────────────────────
-- A worker_ref is a URI: func://<id> runs a function.lua synchronously,
-- proc://<id> execs a process.lua on a host. These schemes are wire format.
M.WORKER_RUNTIME = {
    FUNC = "func",
    PROC = "proc",
}
M.WORKER_SCHEME = {
    FUNC = "func://",
    PROC = "proc://",
}
-- How the runner hands events to a worker. prefetch_events selects the batch
-- and passes the rows; cursor_only passes only the range and lets the worker
-- read its own. CURSOR_ONLY is the default when a projection declares neither.
M.WORKER_INPUT_MODE = {
    PREFETCH_EVENTS = "prefetch_events",
    CURSOR_ONLY = "cursor_only",
}
M.WORKER_INPUT_MODE_DEFAULT = "cursor_only"
-- worker_ref length caps. Invariant bounds.
M.WORKER_ID_MAX_LEN = 240
M.WORKER_REF_MAX_LEN = 260

-- ─── Window ────────────────────────────────────────────────────────────
-- The single supported coalescing window mode. Its numeric DEFAULTS are tunable
-- (see env.lua); these are the invariant safety ceilings the register path
-- clamps to.
M.WINDOW_MODE_LIVE_WINDOW = "live_window"
M.WINDOW_MAX_EVENTS = 20000
M.WINDOW_MAX_TIMEOUT_MS = 604800000 -- 7 days, the longest a window may stay open

-- ─── Trigger engines ───────────────────────────────────────────────────
-- The engine a projection's trigger policy runs under. Enumerated set, not a
-- dial. immediate dispatches every batch; live_window coalesces; expr/func defer
-- the decision to an expression or a func:// evaluator.
M.TRIGGER_ENGINE = {
    IMMEDIATE = "immediate",
    LIVE_WINDOW = "live_window",
    EXPR = "expr",
    FUNC = "func",
}

-- ─── Numeric bounds for register inputs ────────────────────────────────
-- Hard min/max the register path clamps batch_size and start_seq to. The
-- DEFAULTs are tunable (env.lua); the MIN/MAX here are invariant safety rails.
M.BATCH_SIZE = {
    MIN = 1,
    MAX = 2000,
}
M.START_SEQ = {
    MIN = 0,
    MAX = 2147483647, -- INT32 max, the widest a seq column accepts
}

return M
