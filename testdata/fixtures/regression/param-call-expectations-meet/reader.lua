local store = require("store")

type Executor = store.DB | store.Transaction

local reader = {}

function reader.rows(db_or_tx: Executor, id: string): {string}
    return {}
end

return reader
