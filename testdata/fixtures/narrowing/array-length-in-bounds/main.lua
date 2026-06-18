local function first(xs: {number}): number
    if #xs >= 1 then
        return xs[1]
    end
    return 0
end
return first({ 10, 20 })
