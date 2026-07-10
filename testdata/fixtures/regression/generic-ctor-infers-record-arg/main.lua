type Validation<T> = {ok: true, value: T} | {ok: false}
type Config = {id: string, n: number}

local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end

-- raw.id / raw.n are proven concrete by the type guards, so the record literal
-- passed to the generic constructor lets T be inferred as Config and the
-- instantiated return Validation<Config> matches the declared return.
local function decode(raw: any): Validation<Config>
    if type(raw.id) ~= "string" then
        return {ok = false}
    end
    if type(raw.n) ~= "number" then
        return {ok = false}
    end
    return ok({id = raw.id, n = raw.n})
end

return decode
