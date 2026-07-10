-- Derived from framework/src/migration/runner.lua.

local runner = {}
local time = {}
local funcs = {} :: any

function time.now(): any
    return {
        sub = function(_self: any, _start: any): any
            return {
                milliseconds = function(): number
                    return 0
                end
            }
        end
    }
end

type RunnerOptions = {
    tags: {string}?,
    allowed_ids: {string}?,
    count: number?,
}

local function create_error(message: string): any
    return {
        status = "error",
        error = tostring(message)
    }
end

local function get_description(migration: any): any
    if migration.meta and migration.meta.description and migration.meta.description ~= "" then
        return migration.meta.description
    end
    if migration.comment and migration.comment ~= "" then
        return migration.comment
    end
    return ""
end

local Runner = {}
Runner.__index = Runner

function runner.setup(database_id: string): any
    if not database_id then
        error("Database ID is required for migration runner setup")
    end

    local self = setmetatable({}, Runner)
    self.database_id = database_id

    return self
end

function Runner:find_migrations(_options: RunnerOptions?): ({any}?, string?)
    return {
        {
            id = "migration.001",
            meta = { description = "create tables" },
            applied = false,
        }
    }, nil
end

local function execute_migration(_migration_id: string, _options: any): any
    local executor = funcs.new() :: any
    local result, exec_err = executor:call(_migration_id, _options)
    if exec_err then
        return {
            status = "error",
            error = "Failed to execute migration: " .. tostring(exec_err)
        }
    end

    if result.migrations and #result.migrations > 0 then
        return result.migrations[1]
    end

    return result
end

function Runner:run(options: RunnerOptions?): any
    options = options or {}

    local migrations, find_err = self:find_migrations(options)
    if find_err then
        return create_error(find_err)
    end

    if not migrations or #migrations == 0 then
        return {
            status = "complete",
            message = "No migrations found",
            migrations_found = 0,
            migrations_applied = 0,
            migrations_skipped = 0,
            migrations_failed = 0
        }
    end

    local results = {
        status = "running",
        migrations_found = #migrations,
        migrations_applied = 0,
        migrations_skipped = 0,
        migrations_failed = 0,
        migrations = {},
        skipped_details = {}
    }

    local start_time = time.now()

    for _, migration in ipairs(migrations) do
        if migration.applied then
            results.migrations_skipped = results.migrations_skipped + 1
            local skip_details = {
                id = migration.id,
                name = get_description(migration),
                reason = "Already applied",
                skip_type = "already_applied"
            }
            table.insert(results.skipped_details, skip_details)
            table.insert(results.migrations, {
                id = migration.id,
                status = "skipped",
                skip_type = "already_applied",
                reason = "Already applied",
                applied_at = migration.applied_at,
                description = get_description(migration)
            })
            goto continue
        end

        local migration_options = {
            database_id = self.database_id,
            direction = "up",
            id = migration.id
        }

        local result = execute_migration(tostring(migration.id), migration_options)

        if result and result.status == "error" then
            results.migrations_failed = results.migrations_failed + 1
            table.insert(results.migrations, {
                id = migration.id,
                status = "error",
                error = result.error,
                description = get_description(migration)
            })

            results.status = "error"
            results.error = result.error
            break
        elseif result and result.status == "applied" then
            results.migrations_applied = results.migrations_applied + 1
            table.insert(results.migrations, {
                id = migration.id,
                status = "applied",
                description = get_description(migration),
                duration = result.duration
            })
        else
            results.migrations_skipped = results.migrations_skipped + 1

            local reason = result and result.reason or "Unknown"

            if result and result.skipped_reasons and #result.skipped_reasons > 0 then
                reason = result.skipped_reasons[1].reason
            end

            local skip_details = {
                id = migration.id,
                name = get_description(migration),
                reason = reason,
                skip_type = "other"
            }
            table.insert(results.skipped_details, skip_details)
            table.insert(results.migrations, {
                id = migration.id,
                status = "skipped",
                skip_type = "other",
                reason = reason,
                description = get_description(migration)
            })
        end

        ::continue::
    end

    local end_time = time.now()
    results.duration = end_time:sub(start_time):milliseconds() / 1000

    if results.status ~= "error" then
        results.status = "complete"
    end

    return results
end

return runner
