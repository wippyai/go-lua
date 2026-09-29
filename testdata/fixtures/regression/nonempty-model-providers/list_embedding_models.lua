local http = require("http")
local security = require("security")
local models = require("models")

local function handler()
    local res = http.response()
    local req = http.request()
    if not res or not req then
        return nil, "failed to get http context"
    end

    res:set_content_type(http.CONTENT.JSON)

    local actor = security.actor()
    if not actor then
        res:set_status(http.STATUS.UNAUTHORIZED)
        res:write_json({ success = false, error = "authentication required" })
        return
    end

    local all_models = models.get_all()

    local embedding_models = {}
    for _, model in ipairs(all_models) do
        local is_embedding = false
        if model.capabilities then
            for _, cap in ipairs(model.capabilities) do
                if cap == "embed" then
                    is_embedding = true
                    break
                end
            end
        end

        if is_embedding then
            local provider = "unknown"
            if model.providers and #model.providers > 0 then
                local first = model.providers[1]
                provider = first.title or first.name or first.id or provider
            end

            table.insert(embedding_models, {
                name = model.name,
                title = model.title or model.name,
                description = model.description or "",
                dimensions = model.dimensions,
                provider = provider,
            })
        end
    end

    res:set_status(http.STATUS.OK)
    res:write_json({
        success = true,
        count = #embedding_models,
        models = embedding_models
    })
end

return { handler = handler }

