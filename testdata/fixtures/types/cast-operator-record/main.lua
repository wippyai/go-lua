local function f(raw: any): string
    local obj = raw :: { title: string, body: string }
    return obj.title
end
return f({ title = "t", body = "b" })
