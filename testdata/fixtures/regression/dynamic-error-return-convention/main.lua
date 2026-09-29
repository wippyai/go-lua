-- A local function whose returns follow (value, nil) / (nil, err) keeps the
-- convention when the error comes from a dynamic call: the guard that
-- returns it proves it present (kb10 query_service: embeddings from an llm
-- module whose error result is untyped).
type EmbedResponse = { result: { number } | { { number } } }
local llm = {} :: { embed: (queries: { string }, options: any) -> (EmbedResponse?, any) }
local M = { _llm = llm }

local function embed_search_queries(queries, embedding_model)
    if #queries == 0 then
        return {}, nil
    end
    local response, err = M._llm.embed(queries, { model = embedding_model })
    if err then
        return nil, err
    end
    if not response.result or #response.result ~= #queries then
        return nil, "embedding batch size mismatch"
    end
    return response.result, nil
end

local function search(unique_queries: { string }, model: string)
    local query_embeddings, qembed_err = embed_search_queries(unique_queries, model)
    if qembed_err then
        return {}
    end
    local out = {}
    for i, _ in ipairs(unique_queries) do
        table.insert(out, query_embeddings[i])
    end
    return out
end

return search
