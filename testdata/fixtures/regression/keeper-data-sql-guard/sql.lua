-- Declaration of the external sql API; no Lua body supplies its return paths.
type DB = { release: (self: DB) -> (), query: (self: DB, string, any?) -> ({any}?, string?) }
type SQL = { get: (string) -> (DB?, string?) }
return {} :: SQL
