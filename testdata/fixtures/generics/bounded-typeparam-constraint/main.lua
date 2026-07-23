type HasId = { id: string }
local function id_of<T: HasId>(x: T): string
    return x.id
end
local user = { id = "u1", name = "n" }
return id_of(user)
