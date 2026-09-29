local sql = require("sql")
local cron_types = require("cron_types")
local schedule_store = require("schedule_store")
local get_db: () -> (sql.DB?, string?) = schedule_store.get_db
local row_to_schedule_data = function(row: any): cron_types.ScheduleData return row :: cron_types.ScheduleData end
local schedule_repo = {}

-- Source: kickside.cron.persist:schedule_repo.
function schedule_repo.get(task_id: any): (cron_types.ScheduleData?, string?)
    if not task_id or task_id == "" then
        return nil, "task_id is required"
    end

    local db, err = get_db()
    if err then
        return nil, err
    end

    local query = sql.builder.select("*"):from("kickside_cron_schedules"):where("id = ?", task_id)
    local executor = query:run_with(db)
    local results, query_err = executor:query()
    db:release()

    if query_err then
        return nil, "Failed to get schedule: " .. query_err
    end

    if not results or #results == 0 then
        return nil, "Schedule not found"
    end

    return row_to_schedule_data(results[1]), nil
end

return schedule_repo
