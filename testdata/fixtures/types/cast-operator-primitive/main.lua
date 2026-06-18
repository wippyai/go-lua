local function h(v: any): number
    local s = v :: string
    return #s
end
return h("hello")
