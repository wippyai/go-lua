local run_repo = require("run_repo")
local git_scan = require("git_scan")
local changeset_scan = require("changeset_scan")
local suspect = require("suspect")
local function normalize_change_source(value: unknown): string
    local source = type(value) == "string" and value or ""
    if source == "changeset" or source == "changesets" then return "changeset" end
    if source == "mixed" then return "mixed" end
    return "git_scan"
end

local function load_pending(opts)
    local source = normalize_change_source(opts.change_source or opts.source)
    if source == "changeset" then
        return changeset_scan.list_changes(opts)
    end
    if source == "mixed" then
        local git_pending, git_cfg = git_scan.list_changes(opts)
        if not git_pending then return nil, git_cfg end
        local cs_pending, cs_cfg = changeset_scan.list_changes(opts)
        if not cs_pending then return nil, cs_cfg end
        for _, ch in ipairs(cs_pending) do table.insert(git_pending, ch) end
        return git_pending, { git = git_cfg, changeset = cs_cfg, source = "mixed" }
    end
    return git_scan.list_changes(opts)
end

return load_pending
