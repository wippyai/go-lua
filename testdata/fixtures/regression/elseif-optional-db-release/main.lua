-- Source: app:value_store_bench lines 159-176; identical in app:value_store_bench_load.
type DB = {query: (string, any) -> (any?, string?), release: () -> ()}
local sql = {} :: {get: (string) -> (DB?, string?)}
local time = {} :: any
local APP_DB = "app:db"
local function fail(message: string) error(message) end
local function wait_for_tables()
    for _ = 1, 300 do
        local db, err = sql.get(APP_DB)
        if not err and db then
            -- Probe the table itself: engine-agnostic; a query error means the
            -- migration has not created it yet.
            local _, qerr = db:query("SELECT 1 FROM spiralscout_crm_record_value_index LIMIT 1", {})
            db:release()
            if not qerr then
                return
            end
        elseif db then
            db:release()
        end
        time.sleep("100ms")
    end
    fail("bootloader did not create CRM tables before benchmark start")
end

wait_for_tables()
