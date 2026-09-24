type DB = {
    execute: (self: DB, statement: string, params: {string}) -> (boolean, string?),
    release: (self: DB) -> boolean,
}

local sql = {}
function sql.get(name: string): DB?
    if name == "missing" then return nil end
    return {execute = function() return true, nil end, release = function() return true end} :: DB
end
return sql
