type ContractArgs = {
    model: string,
    options: table,
}

local function parse_text(text: string?)
    return text
end

local function merge(args: ContractArgs)
    return args
end

-- A scalar `type(x.f) == "string"` guard on a gradual `any` field proves the
-- field is a string on the true edge, so the narrowed field flows into a typed
-- parameter without error.
local function extract(block: any)
    if type(block.text) == "string" then
        return parse_text(block.text)
    end
    return nil
end

-- The narrowed field also satisfies a typed record field built from it.
local function build(info: any)
    if type(info.model) == "string" then
        return merge({model = info.model, options = {}})
    end
    return nil
end

local a = extract({text = "hi"})
local b = build({model = "x"})
return a, b
