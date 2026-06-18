type Named = { name: string }
type Aged = { age: number }
type Person = Named & Aged
local function describe(p: Person): string
    return p.name
end
return describe({ name = "bob", age = 30 })
