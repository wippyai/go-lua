type HasId = { id: string }
local function need_id<T: HasId>(x: T): string return x.id end
return need_id({ name = "no-id-here" })
