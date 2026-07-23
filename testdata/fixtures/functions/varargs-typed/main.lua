local function sum(...: number): number
    local total = 0
    for _, v in ipairs({...}) do
        total = total + v
    end
    return total
end
return sum(1, 2, 3, 4)
