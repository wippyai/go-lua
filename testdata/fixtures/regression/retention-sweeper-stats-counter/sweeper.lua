local time = require("time")

local M = {}

-- Diagnostics only. Never prune flow/node summaries, public inputs/outputs,
-- results, observations/evidence, pending commits, or the last applied commit.
local DIAGNOSTICS = "'cycle.state','cycle.function_result','node.input','node.yield','node.yield.result','parallel.progress'"

function M.validate(options)
    options = options or {}
    local days = tonumber(options.days or 0)
    local batch = tonumber(options.batch_size or 500)
    if not days or days < 0 or days > 36500 or days % 1 ~= 0 then
        return nil, "retention days must be an integer from 0 (disabled) to 36500"
    end
    if not batch or batch < 1 or batch > 1000 or batch % 1 ~= 0 then
        return nil, "retention batch_size must be an integer from 1 to 1000"
    end
    return { days = days, batch_size = batch, dry_run = options.dry_run == true }
end

local function rebind(query, postgres)
    if not postgres then return query end
    local i = 0
    return (query:gsub("%?", function() i = i + 1; return "$" .. i end))
end

-- One bounded atomic batch. Callers own/release the pooled database handle.
function M.run(db, options)
    local config, err = M.validate(options)
    if err then return nil, err end
    local stats = { enabled = config.days > 0, days = config.days, data = 0, commits = 0,
        data_candidates = 0, commit_candidates = 0, dry_run = config.dry_run }
    if not stats.enabled then return stats end
    local dialect, type_err = db:type()
    if type_err then return nil, tostring(type_err) end
    local postgres = tostring(dialect) == "postgres"
    if not postgres and tostring(dialect) ~= "sqlite" then return nil, "unsupported retention database" end
    local tx, begin_err = db:begin()
    if begin_err then return nil, tostring(begin_err) end
    local function execute(query, params)
        local result, execute_err = tx:execute(rebind(query, postgres), params or {})
        if execute_err then error(tostring(execute_err)) end
        return result
    end
    local function query(query_text, params)
        local rows, query_err = tx:query(rebind(query_text, postgres), params or {})
        if query_err then error(tostring(query_err)) end
        return rows
    end
    local ok, failure = pcall(function()
        if postgres then
            execute("SET LOCAL lock_timeout = '2s'")
            execute("SET LOCAL statement_timeout = '30s'")
            -- Serialize family eligibility with status changes and new children.
            execute("LOCK TABLE dataflows IN SHARE ROW EXCLUSIVE MODE")
        else
            -- Acquire SQLite's writer reservation before reading eligibility.
            execute("UPDATE dataflows SET status = status WHERE 1 = 0")
        end
        local cutoff = time.now():add(-config.days * 24 * time.HOUR):format(time.RFC3339)
        local old = postgres and "updated_at < ?" or "julianday(updated_at) < julianday(?)"
        local prefix = [[WITH RECURSIVE family AS (
            SELECT dataflow_id, dataflow_id AS root_id, status, updated_at
            FROM dataflows WHERE parent_dataflow_id IS NULL
            UNION
            SELECT f.dataflow_id, p.root_id, f.status, f.updated_at
            FROM dataflows f JOIN family p ON f.parent_dataflow_id = p.dataflow_id
        ), safe_roots AS (
            SELECT root_id FROM family GROUP BY root_id
            HAVING SUM(CASE WHEN status IN ('completed','failed','cancelled','terminated')
                AND ]] .. old .. [[ THEN 0 ELSE 1 END) = 0
        ), eligible AS (
            SELECT f.dataflow_id FROM family f JOIN safe_roots s ON s.root_id = f.root_id
        ) ]]
        local age = postgres and "d.created_at < ?" or "julianday(d.created_at) < julianday(?)"
        local data_rows = query(prefix .. [[SELECT d.data_id AS id FROM dataflow_data d
            JOIN eligible e ON e.dataflow_id = d.dataflow_id
            WHERE d.type IN (]] .. DIAGNOSTICS .. ") AND " .. age ..
            " ORDER BY d.created_at, d.data_id LIMIT ?", { cutoff, cutoff, config.batch_size })
        local commits = query(prefix .. [[SELECT d.commit_id AS id FROM dataflow_commits d
            JOIN eligible e ON e.dataflow_id = d.dataflow_id
            JOIN dataflows f ON f.dataflow_id = d.dataflow_id
            WHERE d.op_id IS NOT NULL AND (f.last_commit_id IS NULL OR d.commit_id <> f.last_commit_id)
            AND ]] .. age .. " ORDER BY d.created_at, d.commit_id LIMIT ?",
            { cutoff, cutoff, config.batch_size })
        stats.data_candidates = #data_rows
        stats.commit_candidates = #commits
        if not config.dry_run then
            local function remove(table_name, key, rows)
                if #rows == 0 then return 0 end
                local placeholders, ids = {}, {}
                for _, row in ipairs(rows) do
                    placeholders[#placeholders + 1] = "?"
                    ids[#ids + 1] = row.id
                end
                local result = execute("DELETE FROM " .. table_name .. " WHERE " .. key ..
                    " IN (" .. table.concat(placeholders, ",") .. ")", ids)
                return tonumber(result.rows_affected) or 0
            end
            stats.data = remove("dataflow_data", "data_id", data_rows)
            stats.commits = remove("dataflow_commits", "commit_id", commits)
        end
    end)
    if not ok then tx:rollback(); return nil, tostring(failure) end
    local _, commit_err = tx:commit()
    if commit_err then tx:rollback(); return nil, tostring(commit_err) end
    return stats
end

return M
