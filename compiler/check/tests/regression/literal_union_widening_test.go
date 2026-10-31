package regression

import "testing"

func TestMutableLiteralUnionWidening(t *testing.T) {
	checkBothModes(t, `
local function f(value: {db_id?: false | "UNKNOWN"})
 local broad: {db_id?: boolean | string} = value
 return broad
end
return f`, "")
}

func TestMutableLiteralUnionWideningControls(t *testing.T) {
	checkBothModes(t, `
local function f(value: {tag: "A" | "B"})
 local broad: {tag: "A" | "B" | "C"} = value
 return broad
end
return f`, "cannot assign")
	checkBothModes(t, `
local function f(value: {value: false | "UNKNOWN"})
 local broad: {value: number | string} = value
 return broad
end
return f`, "cannot assign")
}

func TestValueSourceMigrationTreeSort(t *testing.T) {
	checkBothModes(t, `
local function status(flag: boolean, applied: boolean)
    if flag then return "UNKNOWN" end
    return applied and "APPLIED" or "PENDING"
end
local function run(migrations: {{[string]: unknown}}, db_types: {[string]: unknown})
    local db_map = {}
    for _, migration in ipairs(migrations) do
        local target_db = migration.attributes and migration.attributes["meta.target_db"]
        if type(target_db) ~= "string" or target_db == "" then target_db = "UNKNOWN_DATABASE" end
        if not db_map[target_db] then
            db_map[target_db] = {db_id = target_db, db_type = db_types[target_db] or "unknown", migrations = {}}
        end
        local migration_id = type(migration.id) == "string" and migration.id or ""
        table.insert(db_map[target_db].migrations, {
            id = migration_id,
            description = migration.attributes and migration.attributes["meta.description"] or "",
            timestamp = migration.attributes and migration.attributes["meta.timestamp"] or "",
            status = status(false, true)
        })
    end
    for _, db_data in pairs(db_map) do
        table.sort(db_data.migrations, function(a, b)
            if a.timestamp ~= "" and b.timestamp ~= "" then return a.timestamp < b.timestamp end
            return a.id < b.id
        end)
    end
    local sorted_dbs = {}
    for _, db_data in pairs(db_map) do table.insert(sorted_dbs, db_data) end
    table.sort(sorted_dbs, function(a, b) return a.db_id < b.db_id end)
end
return run`, "")
}
