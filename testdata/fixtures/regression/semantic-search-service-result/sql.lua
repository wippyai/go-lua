type Row = {[string]: any}
type DB = {
    query: (string, any?) -> ({Row}, string?),
    type: () -> (string, string?),
}
type QueryExecutor = {
    query: () -> ({Row}, string?),
    exec: () -> ({rows_affected: integer, last_insert_id: integer}, string?),
}
type SelectBuilder = {
    from: (string) -> SelectBuilder,
    join: (string) -> SelectBuilder,
    where: (any, any?, any?) -> SelectBuilder,
    order_by: (string) -> SelectBuilder,
    group_by: (string) -> SelectBuilder,
    limit: (number) -> SelectBuilder,
    run_with: (DB) -> QueryExecutor,
}
local db: DB = {
    query = function(query: string, args: any?): ({Row}, string?) return {}, nil end,
    type = function(): (string, string?) return "sqlite", nil end,
}
return {
    get = function(id: string): (DB, string?) return db, nil end,
    builder = { select = function(...: any): SelectBuilder return {} :: SelectBuilder end,
                insert = function(...: any): any return {} end,
                update = function(...: any): any return {} end,
                delete = function(...: any): any return {} end },
    type = { POSTGRES = "postgres" },
    NULL = {} :: any,
}
