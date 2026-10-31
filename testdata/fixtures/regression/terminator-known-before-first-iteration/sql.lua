-- Stand-in for the runtime sql module: sql.get's DB is present when its error
-- is absent.
type Row = {[string]: any}
type ExecResult = {rows_affected: integer, last_insert_id: integer}
type DB = {
    type: (self: DB) -> (string, error?),
    query: (self: DB, sql: string, ...any) -> ({Row}, error?),
    execute: (self: DB, sql: string, ...any) -> (ExecResult, error?),
    release: (self: DB) -> (boolean, error?),
}
local sql = {}
sql.type = { POSTGRES = "postgres", SQLITE = "sqlite" }
function sql.get(name: string): (DB?, error?) return nil, nil end
return sql
