local config = require("keeper_config")

local M = {}

function M.db_id(): string
    return config.app_db()
end

M.TOPIC = "keeper.knowledge"

M.DEFAULT_KB_ID = "00000000-0000-0000-0000-000000000001"
M.DEFAULT_KB_NAME = "General"

M.NODE_TYPE = {
    PATTERN = "pattern",
    CONVENTION = "convention",
    LEARNING = "learning",
    ANTI_PATTERN = "anti_pattern",
}

M.SOURCE = {
    HUMAN = "human",
    SCAN = "scan",
    SEED = "seed",
    AGENT = "agent",
    WORKSPACE = "workspace",
}

M.EVENTS = {
    KB_CREATED = "kb.created",
    KB_UPDATED = "kb.updated",
    KB_DELETED = "kb.deleted",
    NODE_CREATED = "node.created",
    NODE_UPDATED = "node.updated",
    NODE_DELETED = "node.deleted",
    NODE_EMBEDDED = "node.embedded",
    SCAN_PROGRESS = "scan.progress",
    SCAN_COMPLETE = "scan.complete",
}

M.EMBED = {
    MODEL = "class:embed",
    FALLBACK_MODELS = {},
    DIMENSIONS = 512,
}

function M.embedding_models(model: string?): {string}
    if type(model) == "string" and model ~= "" then
        return { model }
    end

    local models = { M.EMBED.MODEL }
    local seen = { [M.EMBED.MODEL] = true }
    for _, fallback in ipairs(M.EMBED.FALLBACK_MODELS) do
        if type(fallback) == "string" and fallback ~= "" and not seen[fallback] then
            table.insert(models, fallback)
            seen[fallback] = true
        end
    end
    return models
end

return M

