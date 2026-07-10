type RunResult = { status: string, error: string? }

local function run_migration(fail: boolean): RunResult
    local result: RunResult = {
        status = "success",
    }

    if fail then
        result.status = "error"
        result.error = "failed"
    end

    return result
end

local function boot(fail: boolean): string?
    local result = run_migration(fail)
    if result.status == "error" then
        return "Migration failed: " .. result.error
    end
    return nil
end

return boot
