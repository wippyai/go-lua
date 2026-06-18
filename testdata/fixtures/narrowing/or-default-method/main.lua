local function f(name: string?): string
    return (name or "anon"):upper()
end
return f(nil)
