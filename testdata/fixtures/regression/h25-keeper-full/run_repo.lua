local sql = require("sql")
local json = require("json")
local consts = require("git_consts")
local schema = require("schema")

local M = {}

local function db()
    local db, err = sql.get("keeper.state:db")
    if err then error("git: failed to open keeper.state:db: " .. err) end
    local _, schema_err = schema.ensure(db)
    if schema_err then
        db:release()
        error("git: failed to ensure keeper_git_runs schema: " .. schema_err)
    end
    return db
end

local function row_from_record(row)
    local payload = nil
    if type(row.payload_json) == "string" and row.payload_json ~= "" then
        local decoded, derr = json.decode(row.payload_json)
        if not derr then payload = decoded end
    end
    return {
        run_id        = row.run_id,
        started_at    = row.started_at,
        finished_at   = row.finished_at,
        status        = row.status,
        journal_size  = row.journal_size,
        cluster_count = row.cluster_count,
        ai_model      = row.ai_model,
        error         = row.error,
        payload       = payload,
    }
end

function M.insert(run)
    local payload_str = json.encode(run.payload or {}) or "{}"
    local d = db()
    local _, err = d:execute([[
        INSERT INTO keeper_git_runs
            (run_id, started_at, finished_at, status, journal_size, cluster_count, ai_model, error, payload_json)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    ]], {
        run.run_id,
        run.started_at,
        run.finished_at,
        run.status,
        run.journal_size or 0,
        run.cluster_count or 0,
        run.ai_model,
        run.error,
        payload_str,
    })
    if err then return nil, err end
    return run.run_id, nil
end

function M.update(run_id, fields)
    if not run_id or run_id == "" then return nil, "run_id required" end
    local sets, params = {}, {}
    for k, v in pairs(fields or {}) do
        if k == "payload" then
            table.insert(sets, "payload_json = ?")
            local encoded = json.encode(v or {})
            table.insert(params, encoded)
        elseif k == "finished_at" or k == "status" or k == "journal_size"
            or k == "cluster_count" or k == "ai_model" or k == "error" then
            table.insert(sets, k .. " = ?")
            table.insert(params, v)
        end
    end
    if #sets == 0 then return run_id, nil end
    table.insert(params, run_id)
    local d = db()
    local _, err = d:execute("UPDATE keeper_git_runs SET " .. table.concat(sets, ", ") ..
        " WHERE run_id = ?", params)
    if err then return nil, err end
    return run_id, nil
end

function M.latest_finished()
    local d = db()
    local rows, err = d:query([[
        SELECT * FROM keeper_git_runs
        WHERE status = ?
        ORDER BY finished_at DESC
        LIMIT 1
    ]], { consts.RUN_STATUS.FINISHED })
    if err then return nil, err end
    if not rows or #rows == 0 then return nil, nil end
    return row_from_record(rows[1]), nil
end

-- Drop runs beyond the most recent KEEP rows (by started_at).
function M.cleanup_old(keep)
    keep = keep or consts.RUN_HISTORY_KEEP
    local d = db()
    local _, err = d:execute([[
        DELETE FROM keeper_git_runs
        WHERE run_id NOT IN (
            SELECT run_id FROM keeper_git_runs
            ORDER BY started_at DESC
            LIMIT ?
        )
    ]], { keep })
    return err
end

-- Total journal rows currently in keeper_changeset_changes (for staleness check).
function M.current_journal_size()
    local d = db()
    local rows, err = d:query("SELECT COUNT(*) AS n FROM keeper_changeset_changes", {})
    if err then return 0, err end
    if not rows or #rows == 0 then return 0, nil end
    return rows[1].n or 0, nil
end

return M
