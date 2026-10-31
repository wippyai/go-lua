local M = {}

function M.trim(value: any): string
    return tostring(value or ""):match("^%s*(.-)%s*$") or ""
end

function M.journal_id(params: any): (string?, string?)
    local source = type(params) == "table" and params or {}
    local id = M.trim(source.journal_id or source.id)
    if id == "" then return nil, "journal_id is required" end
    return id, nil
end

function M.maintain_position(journal_id: string, options: any, out: any): any
    if type(out) ~= "table" then return out end
    local source = type(options) == "table" and options or {}
    local handle = M.trim(source.cursor_id)
    local agent_id = M.trim(source.agent_id)
    if handle == "" and agent_id == "" then return out end
    out.journal_id = journal_id
    return out
end

return M
