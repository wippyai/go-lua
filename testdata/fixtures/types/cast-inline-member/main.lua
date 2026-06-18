local function g(result: any): string
    return (result :: { id: string }).id
end
return g({ id = "x1" })
