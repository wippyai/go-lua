type Result<T> = { ok: true, value: T } | { ok: false, error: string }
local function bind<T, U>(r: Result<T>, f: (T) -> Result<U>): Result<U>
    if r.ok then
        return f(r.value)
    end
    return { ok = false, error = r.error }
end
local function parse(n: number): Result<number>
    if n > 0 then return { ok = true, value = n } end
    return { ok = false, error = "non-positive" }
end
local r: Result<number> = { ok = true, value = 5 }
local out = bind(r, parse)
return out
