local function build(flag: boolean): number
    local h
    if flag then
        h = 5
    end
    local x: number = h
    return x
end

print(build(true))
