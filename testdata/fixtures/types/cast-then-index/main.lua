local function f(v: any): number
    return (v :: {number})[1]
end
return f({10, 20})
