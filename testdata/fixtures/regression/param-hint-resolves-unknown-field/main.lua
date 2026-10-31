local catalog = {}

function catalog.get_by_name(name)
    if name == "" then
        return nil, "missing name"
    end
    return { name = name, max_tokens = 0, output_tokens = 0 }, nil
end

local compress = { _models = catalog }

local function get_model_info(model_name)
    local model_card, err = compress._models.get_by_name(model_name)
    if not model_card then
        return nil, "Model not found: " .. (err or "unknown error")
    end

    local max_context_tokens = model_card.max_tokens or 8000
    local max_output_tokens = model_card.output_tokens or 1000

    return {
        model_card = model_card,
        max_context_tokens = max_context_tokens,
        max_output_tokens = max_output_tokens,
    }, nil
end

local function validate_target_size(target_chars, model_info)
    if target_chars > (tonumber(model_info.max_output_tokens) or 0) then
        return nil, "too large"
    end
    return true, nil
end

function compress.to_size(model_name, target_chars)
    local model_info, err = get_model_info(model_name)
    if err then
        return nil, err
    end
    model_info = assert(model_info)

    local valid, verr = validate_target_size(target_chars, model_info)
    if verr then
        return nil, verr
    end
    return valid, nil
end

return compress
