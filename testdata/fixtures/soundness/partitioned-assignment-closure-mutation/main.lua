-- A closure call can clear x after the ok-keyed assignment. The later ok guard
-- must not resurrect the stale assignment fact and trust x as string.
local function f(ok: boolean): string
    local x: string?
    if ok then x = "ready" end
    local function clear() x = nil end
    clear()
    if ok then
        local s: string = x -- expect-error
        return s
    end
    return ""
end

return f
