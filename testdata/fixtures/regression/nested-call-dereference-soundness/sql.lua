type DB = {
    query: (self: DB, statement: string, params: {string}) -> ({any}, string?),
    release: (self: DB) -> boolean,
}

local sql = {}
function sql.get(name: string): DB?
    if name == "missing" then return nil end
    return {query = function() return {}, nil end, release = function() return true end} :: DB
end
return sql
