local sql = require("sql")

local function maybe_db(skip: boolean): (sql.DB?, string?)
    if skip then return nil, nil end
    return sql.get("app:db")
end

local function use(skip: boolean)
    local db, err = maybe_db(skip)
    if err then return end
    db:release() -- expect-error: cannot call method on optional value
end

use(false)
