local sql = require("sql")
local helper = require("helper")
local db = sql.get("spiralscout.estimation:db")
local function clear() db = nil end
helper.rows(db, clear)
db:release()
