local function f(raw: any): string
    local obj = raw :: { id: string? }
    return obj.id!
end
return f({ id = "x" })
