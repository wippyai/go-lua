-- Reduced from wippy.llm.bedrock:embed. Both model helpers return a result
-- paired with an error, then the handler assigns either pair to the same locals.
local function embed_with_titan(fail: boolean)
    if fail then return nil, "titan failed" end
    return { embeddings = { 1 }, tokens = { total_tokens = 1 } }, nil
end

local function embed_with_cohere(fail: boolean)
    if fail then return nil, "cohere failed" end
    return { embeddings = { 2 }, tokens = { total_tokens = 2 } }, nil
end

local function detect_model_family(model_id: string)
    if model_id:match("titan") then
        return "titan"
    elseif model_id:match("cohere") then
        return "cohere"
    end
    return nil
end

local function handler(model_id: string, fail: boolean)
    local family = detect_model_family(model_id)
    if not family then return nil, "unsupported" end
    local result, err
    if family == "titan" then
        result, err = embed_with_titan(fail)
    elseif family == "cohere" then
        result, err = embed_with_cohere(fail)
    end
    if err then return nil, err end
    return { embeddings = result.embeddings, tokens = result.tokens }
end

local function direct(fail: boolean)
    local result, err = embed_with_titan(fail)
    if err then return nil, err end
    return result.embeddings
end

local function no_error(model_id: string)
    local family = detect_model_family(model_id)
    if not family then return nil end
    local result
    if family == "titan" then
        result = { embeddings = { 1 } }
    elseif family == "cohere" then
        result = { embeddings = { 2 } }
    end
    return result.embeddings
end

return handler
