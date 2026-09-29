-- From spiralscout.estimation:binding_test (registry source, read renders the seeded tree).
local sql = require("sql")

local function read_seeded_tree(estimate_id: string, root: string)
    local db = sql.get("spiralscout.estimation:db")
    local _, m_err = db:execute(
        "INSERT INTO estimate_metric (estimate_id, node_id) VALUES ($1, $2)",
        { estimate_id, root })
    db:release()
    return m_err
end

return read_seeded_tree
