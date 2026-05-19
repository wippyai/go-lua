type Config = {
    id: string,
    retries: number,
    labels: {string},
    metadata: {[string]: string},
}

type Validation<T> = {ok: true, value: T} | {ok: false, error: string}

local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end

local function invalid<T>(message: string): Validation<T>
    return {ok = false, error = message}
end

local function read_labels(value): Validation<{string}>
    if value == nil then
        return ok({} :: {string})
    end
    if type(value) ~= "table" then
        return invalid("labels")
    end
    local labels: {string} = {}
    for _, item in ipairs(value) do
        if type(item) ~= "string" then
            return invalid("label")
        end
        table.insert(labels, item)
    end
    return ok(labels)
end

local function read_metadata(value): Validation<{[string]: string}>
    if value == nil then
        return ok({} :: {[string]: string})
    end
    if type(value) ~= "table" then
        return invalid("metadata")
    end
    local metadata: {[string]: string} = {}
    for key, item in pairs(value) do
        if type(key) == "string" and type(item) == "string" then
            metadata[key] = item
        end
    end
    return ok(metadata)
end

local function decode_config(raw: any): Validation<Config>
    if type(raw) ~= "table" then
        return invalid("root")
    end
    if type(raw.id) ~= "string" then
        return invalid("id")
    end
    if type(raw.retries) ~= "number" then
        return invalid("retries")
    end

    local labels = read_labels(raw.labels)
    if not labels.ok then
        return invalid(labels.error)
    end

    local metadata = read_metadata(raw.metadata)
    if not metadata.ok then
        return invalid(metadata.error)
    end

    return ok({
        id = raw.id,
        retries = raw.retries,
        labels = labels.value,
        metadata = metadata.value,
    })
end

local decoded = decode_config({
    id = "worker",
    retries = 3,
    labels = {"critical", "api"},
    metadata = {owner = "ops"},
})

if decoded.ok then
    local config: Config = decoded.value
    local first = config.labels[1]
    if first then
        local label: string = first
    end
    local owner = config.metadata.owner
    if owner then
        local owner_name: string = owner
    end
end

local raw_config: any = {id = "worker", retries = 3}
local unchecked_config: Config = raw_config -- expect-error

if raw_config.id then
    local id: string = raw_config.id -- expect-error
end

local raw_items: any = {items = {"ok", 99}}
if type(raw_items.items) == "table" and type(raw_items.items[1]) == "string" then
    local labels: {string} = raw_items.items -- expect-error
end

local callback: any = function(config)
    return 1
end

local typed_callback: (Config) -> string = callback -- expect-error

return "ok"
