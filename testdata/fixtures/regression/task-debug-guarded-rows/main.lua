-- keeper.agents.tools.task:debug
--
-- One-shot task flight recorder. This intentionally lives under
-- keeper.agents.tools.task:* so task-specific observability does not crowd the
-- root agent tool namespace.

local json = require("json")

local changesets = require("changesets")
local flow_repo = require("flow_repo")
local nodes = require("nodes")
local render = require("render")
local tasks = require("tasks")

local M = {}
local ACTIONS = {}

local function mcp_text(text)
    return { _mcp_content = { { type = "text", text = text } } }
end

local function parse_json(v)
    if type(v) == "table" then return v end
    if type(v) ~= "string" or v == "" then return {} end
    local ok, out = pcall(json.decode, v)
    if ok and type(out) == "table" then return out end
    return {}
end

local function clip(v, n)
    return render.clip(v == nil and "" or tostring(v), n or 120)
end

local function parse_types_csv(s)
    if type(s) ~= "string" or s == "" then return nil end
    local out = {}
    for part in s:gmatch("[^,]+") do
        local v = part:gsub("^%s+", ""):gsub("%s+$", "")
        if v ~= "" then table.insert(out, v) end
    end
    if #out == 0 then return nil end
    return out
end
M.parse_types_csv = parse_types_csv

function M.unique_dataflow_ids(rows)
    local seen, ids = {}, {}
    for _, row in ipairs(rows or {}) do
        local id = row.dataflow_id
        if type(id) == "string" and id ~= "" and not seen[id] then
            seen[id] = true
            table.insert(ids, id)
        end
    end
    return ids
end

function M.latest_dataflow_id(rows)
    for i = #(rows or {}), 1, -1 do
        local id = rows[i].dataflow_id
        if type(id) == "string" and id ~= "" then return id end
    end
    return nil
end

function M.status_counts(rows)
    local counts = {}
    for _, row in ipairs(rows or {}) do
        local s = row.status or "?"
        counts[s] = (counts[s] or 0) + 1
    end
    return counts
end

function M.latest_failed_node(rows)
    for i = #(rows or {}), 1, -1 do
        local row = rows[i]
        if row.status == "failed" or (row.error_message and row.error_message ~= "") then
            return row
        end
    end
    return nil
end

local PROGRESS_TYPES = {
    phase_exited = true,
    phase_transition = true,
    integrate_stage = true,
}

local function is_progress_after_failure(row, failure)
    if not row or not failure then return false end
    if tonumber(row.seq or 0) <= tonumber(failure.seq or 0) then return false end
    if not PROGRESS_TYPES[row.type] then return false end
    if row.type == "integrate_stage" then
        return row.status == "passed" and row.discriminator == "push_success"
    end
    return row.status == "passed" or row.status == "active"
end

function M.failure_is_current(task, rows, failure)
    if not failure then return false, "none" end
    if task and (task.status == "completed" or task.phase == "finish") then
        return false, "task completed after this failure"
    end
    if task and task.status == "abandoned" then
        return false, "task abandoned after this failure"
    end
    for _, row in ipairs(rows or {}) do
        if is_progress_after_failure(row, failure) then
            return false, "later phase progress superseded this failure"
        end
    end
    return true, "latest failure has not been superseded"
end

function M.current_blocker(task, rows)
    if not task then return nil, "task not found" end
    if task.status == "completed" then return nil, "task completed" end
    if task.status == "abandoned" then return nil, "task abandoned" end

    for i = #(rows or {}), 1, -1 do
        local row = rows[i]
        if row.type == "ask_user" and row.status == "active" then
            return row, "waiting for user response"
        end
    end

    local failed = M.latest_failed_node(rows)
    local current = M.failure_is_current(task, rows, failed)
    if current then return failed, "latest unsuperseded failure" end
    if task.status == "waiting_for_user" then return nil, "waiting_for_user without active ask_user row" end
    if task.status == "error" then return failed, "task status is error" end
    return nil, "no current blocker"
end

function M.next_action(task, blocker, latest_flow)
    if not task then return "inspect task id; task row was not found" end
    if task.status == "completed" or task.phase == "finish" then
        return "none — task completed"
    end
    if task.status == "abandoned" then
        return "none — task abandoned"
    end
    if blocker then
        if blocker.type == "ask_user" then
            return "respond to the active ask_user node"
        end
        return "inspect the current blocker; latest failure has not been superseded"
    end
    if latest_flow and (latest_flow.status == "running" or tonumber(latest_flow.running or 0) > 0) then
        return "wait or poll again; latest phase dataflow is still running"
    end
    if task.status == "waiting_for_user" then
        return "respond to task; no active ask_user row was found in the debug window"
    end
    if task.status == "error" then
        return "inspect task error state; no current failing node was found in the debug window"
    end
    return "no immediate action — task is active with no current blocker"
end

local function table_or_empty(v)
    if type(v) == "table" then return v end
    return {}
end

function M.integrate_runs(rows)
    local by_parent = {}
    local roots = {}
    for _, row in ipairs(rows or {}) do
        if row.type == "integrate_stage" then
            if row.discriminator == "run" then
                table.insert(roots, { root = row, stages = {} })
                by_parent[row.node_id] = roots[#roots]
            end
        end
    end
    for _, row in ipairs(rows or {}) do
        if row.type == "integrate_stage" and row.discriminator ~= "run" then
            local group = by_parent[row.parent_node_id or ""]
            if group then table.insert(group.stages, row) end
        end
    end
    return roots
end

local function phase_from_flow_title(title)
    if type(title) ~= "string" then return nil end
    local phase = title:match("^([%w_%-]+):")
    if phase == "task" then return nil end
    return phase
end
M.phase_from_flow_title = phase_from_flow_title

local flow_rollup

function M.phase_attempt_state(row)
    if type(row) ~= "table" then return "unknown" end

    local running = tonumber(row.running or 0) or 0
    local failed = tonumber(row.failed or 0) or 0
    local completed = tonumber(row.completed or 0) or 0
    local accepted = tonumber(row.accepted_exits or 0) or 0

    if running > 0 then return "running" end
    if accepted > 0 then
        if failed > 0 then return "recovered" end
        return "ok"
    end
    if failed > 0 then return "failed" end
    if completed > 0 then return "no_exit" end
    if (tonumber(row.flows or 0) or 0) > 0 then return "started" end
    return "unknown"
end

function M.phase_attempt_rows(rows, rollup_fn)
    rollup_fn = rollup_fn or flow_rollup
    local by_phase = {}
    local order = {}

    local function ensure(phase)
        if not phase or phase == "" then return nil end
        if not by_phase[phase] then
            by_phase[phase] = {
                phase = phase,
                flows = 0,
                completed = 0,
                failed = 0,
                running = 0,
                other = 0,
                accepted_exits = 0,
                last_flow_id = "",
            }
            table.insert(order, phase)
        end
        return by_phase[phase]
    end

    for _, flow_id in ipairs(M.unique_dataflow_ids(rows)) do
        local ok, r = pcall(rollup_fn, flow_id)
        if ok and type(r) == "table" then
            local phase = phase_from_flow_title(r.title)
            local bucket = ensure(phase)
            if bucket then
                bucket.flows = bucket.flows + 1
                local status = r.status or "other"
                if status == "completed" then bucket.completed = bucket.completed + 1
                elseif status == "failed" then bucket.failed = bucket.failed + 1
                elseif status == "running" then bucket.running = bucket.running + 1
                else bucket.other = bucket.other + 1 end
                bucket.last_flow_id = flow_id
            end
        end
    end

    for _, row in ipairs(rows or {}) do
        if row.type == "phase_transition" then
            local from_phase = tostring(row.discriminator or ""):match("^([^%-]+)%-%>")
            local bucket = ensure(from_phase)
            if bucket then bucket.accepted_exits = bucket.accepted_exits + 1 end
        end
    end

    local out = {}
    for _, phase in ipairs(order) do
        local row = by_phase[phase]
        row.state = M.phase_attempt_state(row)
        table.insert(out, row)
    end
    return out
end

function flow_rollup(flow_id)
    local ok_flow, flow = pcall(flow_repo.get_flow, flow_id)
    if not ok_flow or not flow then
        return { flow_id = flow_id, error = tostring(flow or "not found") }
    end
    local ok_nodes, flow_nodes = pcall(flow_repo.all_nodes, flow_id)
    if not ok_nodes then flow_nodes = {} end
    local counts = M.status_counts(flow_nodes or {})
    local tool_calls = 0
    for _, node in ipairs(flow_nodes or {}) do
        if node.type == "tool.call" then tool_calls = tool_calls + 1 end
    end
    local meta = parse_json(flow.metadata)
    return {
        flow_id = flow_id,
        status = flow.status,
        type = flow.type,
        title = meta.title or meta.name or "",
        nodes = #(flow_nodes or {}),
        failed = counts.failed or 0,
        running = counts.running or 0,
        tool_calls = tool_calls,
        created_at = flow.created_at,
    }
end

local function task_nodes(task_id, opts)
    return nodes.list(task_id, opts or { visibility = "all" })
end

local function task_required(params)
    local task_id = params.task_id
    if type(task_id) ~= "string" or task_id == "" then
        return nil, "task_id is required"
    end
    return task_id, nil
end

local function render_task_row(out, task)
    table.insert(out, "## Task")
    table.insert(out, render.table_header({ "task_id", "status", "phase", "title", "updated_at" }))
    table.insert(out, render.table_row({
        task.task_id,
        task.status,
        task.phase,
        clip(task.title, 60),
        task.updated_at or "",
    }))
end

local function render_changesets(out, task_id)
    local active = changesets.active_for_task(task_id)
    local all = changesets.changesets_for_task(task_id) or {}

    table.insert(out, "")
    table.insert(out, "## Changeset / Lock")
    table.insert(out, render.table_header({
        "changeset_id", "state", "branch", "locked_by", "locked_at", "active",
    }))
    if #all == 0 then
        table.insert(out, render.table_row({ "(none)", "", "", "", "", "" }))
        return
    end
    for _, cs in ipairs(all) do
        table.insert(out, render.table_row({
            cs.changeset_id or "",
            cs.state or "",
            cs.state_branch or "",
            cs.locked_by or "",
            cs.locked_at or "",
            active and cs.changeset_id == active.changeset_id and "yes" or "",
        }))
    end
end

local function render_queue(out)
    local active = table_or_empty(select(1, tasks.tasks():with_status("active"):with_archived(false):order_by_created("asc"):limit(5):all()))
    local waiting = table_or_empty(select(1, tasks.tasks():with_status("waiting_for_user"):with_archived(false):order_by_created("asc"):limit(5):all()))
    local errored = table_or_empty(select(1, tasks.tasks():with_status("error"):with_archived(false):order_by_created("asc"):limit(5):all()))

    table.insert(out, "")
    table.insert(out, "## Queue Blockers")
    table.insert(out, render.table_header({ "status", "task_id", "phase", "title" }))
    local any = false
    for _, group in ipairs({
        { "active", active },
        { "waiting_for_user", waiting },
        { "error", errored },
    }) do
        for _, task in ipairs(group[2]) do
            any = true
            table.insert(out, render.table_row({
                group[1],
                task.task_id,
                task.phase,
                clip(task.title, 60),
            }))
        end
    end
    if not any then table.insert(out, render.table_row({ "(none)", "", "", "" })) end
end

local function render_transitions(out, rows)
    local transitions = {}
    for _, row in ipairs(rows or {}) do
        if row.type == "phase_transition" then table.insert(transitions, row) end
    end
    table.insert(out, "")
    table.insert(out, "## Phase Transitions")
    if #transitions == 0 then
        table.insert(out, "(none)")
        return
    end
    table.insert(out, render.table_header({ "seq", "from->to", "status", "summary" }))
    for _, row in ipairs(transitions) do
        table.insert(out, render.table_row({
            row.seq or "",
            row.discriminator or "",
            row.status or "",
            clip(row.result_summary or row.content or "", 120),
        }))
    end
end

local function render_integrations(out, rows)
    local runs = M.integrate_runs(rows)
    table.insert(out, "")
    table.insert(out, "## Integrate Attempts")
    if #runs == 0 then
        table.insert(out, "(none)")
        return
    end
    table.insert(out, render.table_header({ "run_seq", "status", "summary", "stages" }))
    for _, run in ipairs(runs) do
        local parts = {}
        for _, st in ipairs(run.stages) do
            table.insert(parts, string.format("%s:%s", st.discriminator or "?", st.status or "?"))
        end
        table.insert(out, render.table_row({
            run.root.seq or "",
            run.root.status or "",
            clip(run.root.result_summary or run.root.error_message or "", 120),
            table.concat(parts, " "),
        }))
    end
end

local function render_phase_attempts(out, rows)
    local attempts = M.phase_attempt_rows(rows)
    table.insert(out, "")
    table.insert(out, "## Phase Attempt Summary")
    if #attempts == 0 then
        table.insert(out, "(none)")
        return
    end
    table.insert(out, render.table_header({
        "phase", "state", "flows", "completed", "failed", "running", "accepted exits", "last flow",
    }))
    for _, row in ipairs(attempts) do
        table.insert(out, render.table_row({
            row.phase,
            row.state,
            row.flows,
            row.completed,
            row.failed,
            row.running,
            row.accepted_exits,
            row.last_flow_id ~= "" and render.shorten(row.last_flow_id) or "",
        }))
    end
end

local function render_current_blocker(out, task, rows)
    local blocker, reason = M.current_blocker(task, rows)
    table.insert(out, "")
    table.insert(out, "## Current Blocker")
    if not blocker then
        table.insert(out, "(none — " .. (reason or "clear") .. ")")
        return
    end
    table.insert(out, render.table_header({ "seq", "type", "disc", "status", "reason", "summary" }))
    table.insert(out, render.table_row({
        blocker.seq or "",
        blocker.type or "",
        blocker.discriminator or "",
        blocker.status or "",
        reason or "",
        clip(blocker.error_message or blocker.result_summary or blocker.content or blocker.title or "", 180),
    }))
end

local function render_latest_flow(out, rows)
    table.insert(out, "")
    table.insert(out, "## Latest Flow")
    local flow_id = M.latest_dataflow_id(rows)
    if not flow_id then
        table.insert(out, "(none)")
        return nil
    end
    local r = flow_rollup(flow_id)
    table.insert(out, render.table_header({ "flow_id", "status", "nodes", "failed", "running", "title" }))
    table.insert(out, render.table_row({
        r.flow_id,
        r.status or "ERR",
        r.nodes or "",
        r.failed or "",
        r.running or "",
        clip(r.error or r.title or "", 90),
    }))
    return r
end

local function render_latest_historical_failure(out, task, rows)
    local failed = M.latest_failed_node(rows)
    table.insert(out, "")
    table.insert(out, "## Latest Historical Failure")
    if not failed then
        table.insert(out, "(none)")
        return
    end

    local current, reason = M.failure_is_current(task, rows, failed)
    local label = current and "current" or "historical"
    table.insert(out, render.table_header({ "seq", "type", "disc", "status", "state", "reason", "error/result" }))
    table.insert(out, render.table_row({
        failed.seq or "",
        failed.type or "",
        failed.discriminator or "",
        failed.status or "",
        label,
        reason or "",
        clip(failed.error_message or failed.result_summary or failed.content or "", 180),
    }))
end

local function render_recent_nodes(out, rows, limit)
    limit = math.min(math.max(tonumber(limit) or 12, 1), 50)
    table.insert(out, "")
    table.insert(out, "## Recent Task Nodes")
    table.insert(out, render.table_header({ "seq", "type", "disc", "status", "title/result" }))
    local start_i = math.max(1, #rows - limit + 1)
    for i = start_i, #rows do
        local row = rows[i]
        table.insert(out, render.table_row({
            row.seq or "",
            row.type or "",
            row.discriminator or "",
            row.status or "",
            clip(row.result_summary or row.title or row.content or row.error_message or "", 120),
        }))
    end
end

local function render_flows(out, rows, limit)
    local ids = M.unique_dataflow_ids(rows)
    limit = math.min(math.max(tonumber(limit) or 12, 1), 50)
    table.insert(out, "")
    table.insert(out, "## Related Dataflows")
    if #ids == 0 then
        table.insert(out, "(none)")
        return
    end
    table.insert(out, render.table_header({ "flow_id", "status", "nodes", "failed", "running", "title" }))
    for i = 1, math.min(#ids, limit) do
        local r = flow_rollup(ids[i])
        table.insert(out, render.table_row({
            r.flow_id,
            r.status or "ERR",
            r.nodes or "",
            r.failed or "",
            r.running or "",
            clip(r.error or r.title or "", 70),
        }))
    end
end

function ACTIONS.summary(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    local task, err = tasks.get_task(task_id)
    if err then return nil, err end
    local rows, nerr = task_nodes(task_id, { visibility = "all" })
    if nerr then return nil, nerr end

    local out = { "# Task Debug `" .. task_id .. "`" }
    render_task_row(out, task)
    render_changesets(out, task_id)
    render_queue(out)

    render_current_blocker(out, task, rows)
    render_latest_historical_failure(out, task, rows)
    render_phase_attempts(out, rows)
    render_transitions(out, rows)
    render_integrations(out, rows)
    render_flows(out, rows, tonumber(params.limit) or 12)
    render_recent_nodes(out, rows, tonumber(params.limit) or 12)

    table.insert(out, "")
    table.insert(out, "Next: `action=integrations` for integrate detail, `action=flows` for flow rollups, `action=search query=<text>` for scoped evidence.")
    return mcp_text(table.concat(out, "\n"))
end

function ACTIONS.status(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    local task, err = tasks.get_task(task_id)
    if err then return nil, err end
    local rows, nerr = task_nodes(task_id, { visibility = "all" })
    if nerr then return nil, nerr end

    local out = { "# Task Status `" .. task_id .. "`" }
    render_task_row(out, task)
    render_current_blocker(out, task, rows)
    local latest_flow = render_latest_flow(out, rows)
    render_latest_historical_failure(out, task, rows)
    render_phase_attempts(out, rows)

    local blocker = M.current_blocker(task, rows)
    table.insert(out, "")
    table.insert(out, "## Next Action")
    table.insert(out, M.next_action(task, blocker, latest_flow))

    return mcp_text(table.concat(out, "\n"))
end

function ACTIONS.nodes(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    local opts = {
        visibility = "all",
        limit = math.min(math.max(tonumber(params.limit) or 80, 1), 200),
        types = parse_types_csv(params.types),
    }
    local rows, err = task_nodes(task_id, opts)
    if err then return nil, err end
    local out = { "# Task Nodes `" .. task_id .. "`", "" }
    table.insert(out, render.table_header({ "seq", "type", "disc", "status", "dataflow", "title/result" }))
    for _, row in ipairs(rows or {}) do
        table.insert(out, render.table_row({
            row.seq or "",
            row.type or "",
            row.discriminator or "",
            row.status or "",
            row.dataflow_id and render.shorten(row.dataflow_id) or "",
            clip(row.result_summary or row.title or row.error_message or row.content or "", 140),
        }))
    end
    return mcp_text(table.concat(out, "\n"))
end

function ACTIONS.flows(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    local rows, err = task_nodes(task_id, { visibility = "all" })
    if err then return nil, err end
    local out = { "# Task Dataflows `" .. task_id .. "`" }
    render_flows(out, rows, tonumber(params.limit) or 50)
    return mcp_text(table.concat(out, "\n"))
end

function ACTIONS.integrations(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    local rows, err = task_nodes(task_id, { visibility = "all", types = { "integrate_stage" } })
    if err then return nil, err end
    local out = { "# Integrations `" .. task_id .. "`" }
    render_integrations(out, rows)
    table.insert(out, "")
    table.insert(out, "## Stage Rows")
    table.insert(out, render.table_header({ "seq", "parent", "stage", "status", "summary/error" }))
    for _, row in ipairs(rows or {}) do
        table.insert(out, render.table_row({
            row.seq or "",
            row.parent_node_id and render.shorten(row.parent_node_id) or "",
            row.discriminator or "",
            row.status or "",
            clip(row.result_summary or row.error_message or "", 180),
        }))
    end
    return mcp_text(table.concat(out, "\n"))
end

local function dataflow_matches(flow_id, query, limit)
    local matches = {}
    local ok, rows = pcall(flow_repo.flow_data, flow_id, { content = true })
    if not ok then return matches end
    local needle = query:lower()
    for _, row in ipairs(rows or {}) do
        local content = row.content
        if type(content) == "string" and content:lower():find(needle, 1, true) then
            table.insert(matches, {
                dataflow_id = flow_id,
                node_id = row.node_id,
                type = row.type,
                created_at = row.created_at,
                snippet = clip(content, 220),
            })
            if #matches >= limit then break end
        end
    end
    return matches
end

function ACTIONS.search(params)
    local task_id, terr = task_required(params)
    if terr then return nil, terr end
    if type(params.query) ~= "string" or params.query == "" then
        return nil, "query is required for search"
    end
    local limit = math.min(math.max(tonumber(params.limit) or 30, 1), 100)
    local out = { "# Task Search `" .. task_id .. "`", "" }

    local task_matches, terr2 = nodes.search(task_id, params.query, { limit = limit })
    if terr2 then task_matches = {} end
    table.insert(out, "## Task Nodes")
    if #task_matches == 0 then
        table.insert(out, "(none)")
    else
        table.insert(out, render.table_header({ "seq", "type", "disc", "title" }))
        for _, row in ipairs(task_matches) do
            table.insert(out, render.table_row({
                row.seq or "",
                row.type or "",
                row.discriminator or "",
                clip(row.title or "", 120),
            }))
        end
    end

    local all_nodes, nerr = task_nodes(task_id, { visibility = "all" })
    if nerr then all_nodes = {} end
    local ids = M.unique_dataflow_ids(all_nodes)
    table.insert(out, "")
    table.insert(out, "## Related Dataflow Payloads")
    local shown = 0
    table.insert(out, render.table_header({ "flow_id", "node_id", "type", "snippet" }))
    for _, flow_id in ipairs(ids) do
        if shown >= limit then break end
        local matches = dataflow_matches(flow_id, params.query, limit - shown)
        for _, match in ipairs(matches) do
            table.insert(out, render.table_row({
                render.shorten(match.dataflow_id),
                render.shorten(match.node_id),
                match.type or "",
                match.snippet,
            }))
            shown = shown + 1
        end
    end
    if shown == 0 then table.insert(out, render.table_row({ "(none)", "", "", "" })) end

    return mcp_text(table.concat(out, "\n"))
end

local function handler(params)
    params = params or {}
    local action = params.action or "summary"
    local fn = ACTIONS[action]
    if not fn then
        return nil, "Invalid action. Use: summary, status, nodes, flows, integrations, search"
    end
    return fn(params)
end

M.handler = handler
return M

