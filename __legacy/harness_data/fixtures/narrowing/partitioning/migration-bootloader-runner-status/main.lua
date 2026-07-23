-- Derived from framework/src/migration/migration_bootloader.lua.

local runner = require("runner")

local log = {}
function log:info(_message: string, _fields: any?): ()
end
function log:error(_message: string, _fields: any?): ()
end

local migration_registry = {}
function migration_registry.get_target_dbs(): ({string}?, string?)
    return {"db.main"}, nil
end

type BootloaderResult = { status: string, message: string, details: any? }

local function wait_for_database(_db_id: string, _max_attempts: number, _sleep_ms: number): (boolean, string?)
    return true, nil
end

local function run(options: any?): BootloaderResult
    log:info("Starting migration bootloader")

    -- Find target databases
    local target_dbs, err = migration_registry.get_target_dbs()
    if err then
        return {
            status = "error",
            message = "Failed to discover target databases: " .. tostring(err)
        } :: BootloaderResult
    end

    if not target_dbs or #target_dbs == 0 then
        log:info("No target databases found")
        return {
            status = "skipped",
            message = "No migrations to apply"
        } :: BootloaderResult
    end

    log:info("Discovered target databases", {
        count = #target_dbs,
        databases = target_dbs
    })

    local total_applied = 0
    local total_failed = 0
    local total_skipped = 0
    local databases_processed = {}

    -- Execute migrations for each target database
    for _, db_resource in ipairs(target_dbs) do
        log:info("Processing migrations for database", { database = db_resource })

        local db_ready, db_err = wait_for_database(db_resource, 20, 500)
        if not db_ready then
            log:error("Database unavailable, skipping migrations", {
                database = db_resource,
                error = db_err
            })

            return {
                status = "error",
                message = "Database unavailable: " .. db_err,
                details = {
                    database = db_resource,
                    databases_processed = databases_processed
                }
            }
        end

        local db_runner = runner.setup(db_resource)
        local result = db_runner:run()

        table.insert(databases_processed, {
            database = db_resource,
            applied = result.migrations_applied or 0,
            failed = result.migrations_failed or 0,
            skipped = result.migrations_skipped or 0,
            status = result.status
        })

        total_applied = total_applied + (result.migrations_applied or 0)
        total_failed = total_failed + (result.migrations_failed or 0)
        total_skipped = total_skipped + (result.migrations_skipped or 0)

        if result.status == "error" then
            log:error("Migration failed for database", {
                database = db_resource,
                error = result.error
            })

            return {
                status = "error",
                message = "Migration failed: " .. result.error,
                details = {
                    databases_processed = databases_processed,
                    total_applied = total_applied,
                    total_failed = total_failed,
                    total_skipped = total_skipped
                }
            }
        end

        log:info("Completed migrations for database", {
            database = db_resource,
            applied = result.migrations_applied,
            skipped = result.migrations_skipped
        })
    end

    return {
        status = "success",
        message = string.format(
            "Processed %d database(s): %d applied, %d skipped",
            #target_dbs,
            total_applied,
            total_skipped
        ),
        details = {
            databases_processed = databases_processed,
            total_applied = total_applied,
            total_failed = total_failed,
            total_skipped = total_skipped
        }
    }
end

return { run = run }
