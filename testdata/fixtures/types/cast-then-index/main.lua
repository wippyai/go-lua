local function f(v: any): number
    return (v :: {number})[1] -- expect-error: may be nil
end
return f({10, 20})
