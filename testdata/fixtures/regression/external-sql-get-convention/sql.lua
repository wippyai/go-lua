-- Declaration of the external sql API; no Lua body supplies its return paths.
type DB = { release: (self: DB) -> () }
type SQL = { get: (string) -> (DB?, string?) }
return {} :: SQL
