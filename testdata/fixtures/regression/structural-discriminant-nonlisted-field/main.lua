type Circle = {shape_kind: "circle", radius: number}
type Square = {shape_kind: "square", side: number}
type Shape = Circle | Square

-- shape_kind is NOT in the old magic list; structural detection recognizes it as
-- the discriminant, so narrowing reads the per-variant payload non-optional.
local function area(sh: Shape): number
    if sh.shape_kind == "circle" then
        return sh.radius * sh.radius
    end
    return sh.side * sh.side
end

local c: Shape = {shape_kind = "circle", radius = 2.0}
local a: number = area(c)
return a
