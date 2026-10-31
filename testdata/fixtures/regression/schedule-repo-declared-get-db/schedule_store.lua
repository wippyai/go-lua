local sql = require("sql")
type Store = {get_db: () -> (sql.DB?, string?)}
return {} :: Store
