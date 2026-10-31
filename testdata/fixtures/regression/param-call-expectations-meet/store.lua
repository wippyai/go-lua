type DB = {
    type: (self: DB) -> string,
    release: (self: DB) -> boolean,
}
type Transaction = {
    commit: (self: Transaction) -> boolean,
}

local store = {}

function store.get(name: string): DB?
    return nil
end

return store
