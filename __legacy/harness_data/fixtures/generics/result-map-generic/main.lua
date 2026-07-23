type Result<T, E> = { ok: true, value: T } | { ok: false, error: E }
local function map<T, U, E>(r: Result<T, E>, f: (T) -> U): Result<U, E>
    if r.ok then
        return { ok = true, value = f(r.value) }
    end
    return { ok = false, error = r.error }
end
local r: Result<number, string> = { ok = true, value = 2 }
local doubled = map(r, function(x: number): number return x * 2 end)
return doubled
