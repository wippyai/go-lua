local function first_byte(s: string): number
    local b = s:byte(1)
    if b == nil then
        return 0
    end
    return b
end
