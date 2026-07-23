type Circle = { kind: "circle", radius: number }
type Square = { kind: "square", side: number }
type Shape = Circle | Square
local function area(s: Shape): number
    if s.kind ~= "circle" then
        return s.side * s.side
    end
    return s.radius * s.radius * 3
end
return area({ kind = "circle", radius = 1 })
