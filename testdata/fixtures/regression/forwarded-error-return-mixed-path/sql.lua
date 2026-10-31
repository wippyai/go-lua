type DB = { release: (self: DB) -> () }
type SQL = { get: (string) -> (DB?, string?) }
return {} :: SQL
