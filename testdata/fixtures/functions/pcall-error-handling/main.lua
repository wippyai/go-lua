local function risky(n: number): number
    if n < 0 then
        error("negative")
    end
    return n * 2
end
local ok: boolean, result = pcall(risky, 5)
if ok then
    return result
end
return 0
