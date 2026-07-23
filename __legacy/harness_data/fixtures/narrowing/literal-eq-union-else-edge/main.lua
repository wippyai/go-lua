local function need_b(x: "b"): "b"
    return x
end

local function pick(): "a" | "b"
    return "a"
end

-- On the else edge of k == "a", the matched literal is removed from the union,
-- so k is narrowed to "b".
local k: "a" | "b" = pick()
if k == "a" then
    return "a"
else
    return need_b(k)
end
