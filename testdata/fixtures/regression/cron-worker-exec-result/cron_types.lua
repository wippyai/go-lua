-- Shared type surface for kickside.cron. Schedule args/context remain open maps
-- because task implementations own those payloads; schedule rows, worker
-- counters, and execution envelopes are stable.

local M = {}

type Map = { [string]: any }

local STATUS = {
    SCHEDULED = "scheduled",
    EXECUTING = "executing",
    COMPLETED = "completed", -- terminal: a once-schedule that ran successfully
    FAILED = "failed",       -- terminal: failed non-retriably or exhausted its retries
    DISABLED = "disabled",   -- terminal: paused by the user / API, not by execution
}

local SCHEDULE_TYPES = {
    ONCE = "once",
    INTERVAL = "interval",
    TICKER = "ticker",
    CRON = "cron",
}

local ORDER_FIELDS = { "created_at", "updated_at", "next_run_at" }
local ORDER_DIRECTIONS = { "ASC", "DESC" }
local DEFAULT_ORDER_FIELD = "created_at"
local DEFAULT_ORDER_DIRECTION = "DESC"

type ScheduleData = {
    id: string,
    description: string?,
    class: string,
    task_implementation_id: string,
    task_context: Map,
    task_args: Map,
    schedule_type: string,
    schedule_expression: string,
    next_run_at: string?,
    last_run_at: string?,
    status: string,
    enabled: boolean,
    picked: boolean,
    picked_by: string?,
    picked_at: string?,
    timeout_seconds: integer,
    retry_count: integer,
    max_retries: integer,
    consecutive_failures: integer,
    -- Deferrals asked for by the action ("wait, then call me again"), counted
    -- apart from retries, which are the action failing.
    defer_count: integer,
    last_error: string?,
    -- The structured failure the last run reported, decoded off last_failure_json.
    last_failure: Map?,
    -- The component a lowered schedule belongs to. NULL for schedules nobody owns.
    owner_component_id: string?,
    actor_id: string?,
    actor_context: string?,
    run_actor_id: string?,
    resolve_live: boolean,
    app_scope: string?,
    created_at: string?,
    updated_at: string?,
    -- Set when a stored JSON payload column fails to decode. Carries the decode
    -- error so the worker fails the task instead of executing it with empty args.
    hydration_error: string?,
}

type ScheduleCreateData = {
    description: string?,
    class: string?,
    task_implementation_id: string,
    task_context: Map?,
    task_args: Map?,
    schedule_type: string,
    schedule_expression: string,
    next_run_at: any?,
    timeout_seconds: integer?,
    max_retries: integer?,
    enabled: boolean?,
    owner_component_id: string?,
    actor_id: string?,
    actor_context: string?,
    run_actor_id: string?,
    resolve_live: boolean?,
    app_scope: string?,
}

type ScheduleUpdates = {
    description: string?,
    schedule_expression: string?,
    task_context: Map?,
    task_args: Map?,
    timeout_seconds: integer?,
    max_retries: integer?,
    enabled: boolean?,
    next_run_at: any?,
    status: string?,
    actor_id: string?,
    actor_context: string?,
    resolve_live: boolean?,
    run_actor_id: string?,
}

type ScheduleFilters = {
    status: string?,
    enabled: boolean?,
    class: string?,
    task_implementation_id: string?,
    schedule_type: string?,
    actor_id: string?,
}

type ListOptions = {
    limit: integer?,
    offset: integer?,
    order_by: string?,
    order_direction: string?,
}

type ScheduleExecutionResult = {
    duration_ms: integer?,
    error: string?,
}

type WorkerStats = {
    start_time: any,
    tasks_processed: number,
    tasks_succeeded: number,
    tasks_failed: number,
    tasks_rescheduled: number,
    tasks_completed: number,
    tasks_retried: number,
    tasks_deferred: number,
    batches_processed: number,
    avg_batch_size: number,
    current_concurrency: number,
    max_concurrency_reached: number,
}

type TaskExecutionResult = {
    task_id: string,
    result: any?,
    error: any?,
    retriable: boolean?,
    -- The task implementation's structured failure { code, message, retriable,
    -- scope, retry_after_ms? }. Present when the implementation reported one; the
    -- string error beside it stays for display.
    failure: table?,
    retry_after_ms: number?,
    duration_ms: number,
}

type WorkerConfig = {
    batch_size: number?,
    max_concurrent: number?,
    poll_interval: string?,
}

type WorkerState = {
    currently_running: number,
    batch_size: number,
    max_concurrent: number,
    worker_pid: string,
}

-- The calculator is reached as an injected module: every branch answers
-- (next_run_at, error) for (expression, last_run_at, created_at).
type ScheduleCalculator = {
    next_once_run: (string?, string?, string?) -> (string?, string?),
    next_interval_run: (string?, string?, string?) -> (string?, string?),
    next_ticker_run: (string?, string?, string?) -> (string?, string?),
    next_cron_run: (string?, string?, string?) -> (string?, string?),
}

type Dependencies = {
    schedule_repo: any,
    schedule_calculator: ScheduleCalculator,
    schedulable_contract: any,
    scope_resolver: any?,
    logger: any,
    execute_task: ((ScheduleData, any) -> TaskExecutionResult)?,
}

M.Map = Map
M.ScheduleData = ScheduleData
M.ScheduleCreateData = ScheduleCreateData
M.ScheduleUpdates = ScheduleUpdates
M.ScheduleFilters = ScheduleFilters
M.ListOptions = ListOptions
M.ScheduleExecutionResult = ScheduleExecutionResult
M.WorkerStats = WorkerStats
M.TaskExecutionResult = TaskExecutionResult
M.WorkerConfig = WorkerConfig
M.WorkerState = WorkerState
M.ScheduleCalculator = ScheduleCalculator
M.Dependencies = Dependencies
M.STATUS = STATUS
M.SCHEDULE_TYPES = SCHEDULE_TYPES
M.ORDER_FIELDS = ORDER_FIELDS
M.ORDER_DIRECTIONS = ORDER_DIRECTIONS
M.DEFAULT_ORDER_FIELD = DEFAULT_ORDER_FIELD
M.DEFAULT_ORDER_DIRECTION = DEFAULT_ORDER_DIRECTION

return M
