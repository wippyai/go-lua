local sql = require("sql")

local function read(db: any): number
    db:query("SELECT 1", {})
    return 1
end

local function run(value: number, callback: () -> ())
    callback()
end

local db = sql.get("database")
run(read(db), function() db = nil end)
db:release() -- expect-error: cannot call method on optional value

local db2 = sql.get("database")
run(false and read(db2) or 1, function() end)
db2:release() -- expect-error: cannot call method on optional value

local function replace(_value: number): DB?
    return nil
end
local db3 = sql.get("database")
db3 = replace(read(db3))
db3:release() -- expect-error: cannot call method on optional value
