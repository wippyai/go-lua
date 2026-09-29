-- An unannotated parameter passed where a DB is expected and where a
-- DB | Transaction is expected must be a DB: both expectations hold.
local store = require("store")
local reader = require("reader")

local function with_db(fn: (store.DB) -> ())
    local db = store.get("app:db")
    if db then
        fn(db)
    end
end

local function make_thread(db: store.DB): string
    return db:type()
end

local function run_once(external_db: store.DB?): boolean
    return external_db ~= nil
end

local function run()
    with_db(function(db)
        local id = make_thread(db)
        local rows = reader.rows(db, id)
        make_thread(db)
        return rows
    end)
    with_db(function(db)
        local rows = reader.rows(db, "x")
        return make_thread(db), rows
    end)
    -- An optional expectation admits the DB the other call requires.
    with_db(function(db)
        local id = make_thread(db)
        local ran = run_once(db)
        return make_thread(db), id, ran
    end)
end

return run
