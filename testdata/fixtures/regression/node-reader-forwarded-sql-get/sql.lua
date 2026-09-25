-- The external sql.get function is declared without a Lua body.
type DB = { release: (self: DB) -> () }
type SQL = { get: (string) -> (DB?, string?) }
return {} :: SQL
