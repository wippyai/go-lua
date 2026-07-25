local names: {string} = {"alpha", "beta"}
local total = 0
for _, name in ipairs(names) do
    total = total + #name
end

local function classify(count: number): {kind: "a", value: string} | {kind: "b", count: number}
    if count > 3 then
        return {kind = "a", value = "wide"}
    end
    return {kind = "b", count = count}
end

local flagged = classify(total)
if flagged.kind == "a" then
    local value: string = flagged.value
    local bad_value: number = flagged.value -- expect-error
else
    local count: number = flagged.count
    local bad_count: string = flagged.count -- expect-error
end

return total
